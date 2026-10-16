// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package docker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

const (
	// ProtectedLabel marks a resource McHarbor itself depends on so destructive routes refuse it.
	ProtectedLabel      = "com.mcharbor.protected"
	composeProjectLabel = "com.docker.compose.project"
	composeServiceLabel = "com.docker.compose.service"
	mcHarborProject     = "mcharbor"
	mcHarborService     = "mcharbor"
	mcHarborAgent       = "mcharbor-agent"
)

// ErrProtectedResource is returned when a destructive action targets McHarbor's own infrastructure.
var ErrProtectedResource = errors.New("protected mcharbor resource")

// shortHexContainerIDRe matches Docker's short (12+ hex) container
// id tokens used in cgroup paths and hostname.
var shortHexContainerIDRe = regexp.MustCompile(`^[0-9a-f]{12,64}$`)

// IsProtectedContainer returns true for containers McHarbor should not mutate
// through normal container actions: its app containers, its helpers, and anything an
// operator labelled com.mcharbor.protected. It cannot see the own-container id; use
// ClassifyContainer or ClassifySummary where the id is at hand.
func IsProtectedContainer(names []string, image string, labels map[string]string) bool {
	if isProtectedLabel(labels) || IsMcHarborHelper(labels) {
		return true
	}
	return IsMcHarborContainer(names, image, labels)
}

// IsMcHarborContainer returns true for a McHarbor application container, this node or a
// peer: the image's role label, a McHarbor compose service, the container name or the image.
func IsMcHarborContainer(names []string, image string, labels map[string]string) bool {
	if strings.EqualFold(strings.TrimSpace(labels[AppRoleLabel]), AppRoleValue) {
		return true
	}
	if labels[composeProjectLabel] == mcHarborProject && labels[composeServiceLabel] == mcHarborService {
		return true
	}
	if mcHarborServiceRe.MatchString(strings.ToLower(strings.TrimSpace(labels[composeServiceLabel]))) {
		return true
	}
	if imageRefIsMcHarbor(image) {
		return true
	}
	for _, name := range names {
		normalized := strings.ToLower(strings.Trim(strings.TrimSpace(name), "/"))
		if normalized == mcHarborService || strings.HasPrefix(normalized, "mcharbor-mcharbor-") || mcHarborNameRe.MatchString(normalized) {
			return true
		}
	}
	return false
}

// IsMcHarborInstance reports whether an inspected container is a McHarbor application
// container: a peer node by image, labels or name, or this process's own container by the
// ids CurrentMcHarborContainerCandidates reads, which survives a renamed container or image.
func IsMcHarborInstance(info container.InspectResponse) bool {
	image := ""
	var labels map[string]string
	if info.Config != nil {
		image = info.Config.Image
		labels = info.Config.Labels
	}
	if IsMcHarborContainer([]string{info.Name}, image, labels) {
		return true
	}
	if info.Config != nil && hasAppImageEnv(info.Config.Env) {
		return true
	}
	if info.ID == "" {
		return false
	}
	for _, candidate := range selfContainerIDs() {
		if shortHexContainerIDRe.MatchString(candidate) && strings.HasPrefix(info.ID, candidate) {
			return true
		}
	}
	return false
}

// IsAgentContainer returns true for the McHarbor remote agent container.
func IsAgentContainer(names []string, image string, labels map[string]string) bool {
	if labels != nil {
		value := strings.ToLower(strings.TrimSpace(labels["com.mcharbor.agent"]))
		if value == "true" || value == "1" || value == "yes" {
			return true
		}
	}
	if imageRefIsMcHarborAgent(image) {
		return true
	}
	for _, name := range names {
		normalized := strings.ToLower(strings.Trim(strings.TrimSpace(name), "/"))
		if normalized == mcHarborAgent || strings.HasPrefix(normalized, mcHarborAgent+"-") {
			return true
		}
	}
	return false
}

// EnsureContainerMutable returns ErrProtectedResource when an operation targets McHarbor itself.
func EnsureContainerMutable(ctx context.Context, cli *client.Client, id string) error {
	inspectCtx, cancel := context.WithTimeout(ctx, CallTimeout)
	defer cancel()

	inspected, err := cli.ContainerInspect(inspectCtx, id, client.ContainerInspectOptions{})
	if err != nil {
		return err
	}
	if ClassifyContainer(inspected.Container) != SelfTargetNone {
		return ErrProtectedResource
	}
	return nil
}

func isProtectedLabel(labels map[string]string) bool {
	if labels == nil {
		return false
	}
	value := strings.ToLower(strings.TrimSpace(labels[ProtectedLabel]))
	return value == "true" || value == "1" || value == "yes"
}

// imageRefIsMcHarbor matches the published image and any registry mirror of it, whose
// last path segment is still "mcharbor"; a retag to another name is caught by AppRoleLabel.
func imageRefIsMcHarbor(ref string) bool {
	repo := imageRepository(ref)
	return repo == "mcharbor" || strings.HasSuffix(repo, "/mcharbor")
}

func imageRefIsMcHarborAgent(ref string) bool {
	repo := imageRepository(ref)
	return repo == "ghcr.io/therealmcsparrow/mcharbor-agent" ||
		repo == "therealmcsparrow/mcharbor-agent" ||
		repo == "mcharbor-agent"
}

func imageRepository(ref string) string {
	ref = strings.ToLower(strings.TrimSpace(ref))
	if ref == "" || ref == "<none>:<none>" {
		return ""
	}
	if idx := strings.Index(ref, "@"); idx >= 0 {
		ref = ref[:idx]
	}
	lastSlash := strings.LastIndex(ref, "/")
	if idx := strings.LastIndex(ref, ":"); idx > lastSlash {
		ref = ref[:idx]
	}
	return ref
}

// CurrentMcHarborContainerCandidates returns the container ids/names
// that should be tried when looking up the McHarbor application
// container from inside itself. Order is: short id from hostname,
// any short hex tokens in mountinfo / cgroup, then the well-known
// Compose names ("mcharbor", "mcharbor-mcharbor-1").
//
// Used by self-management operations (restart, backup-key install)
// that need to address the currently-running McHarbor container.
func CurrentMcHarborContainerCandidates() []string {
	candidates := currentContainerIDCandidates()
	seen := make(map[string]struct{}, len(candidates)+3)
	for _, candidate := range candidates {
		seen[candidate] = struct{}{}
	}
	for _, candidate := range []string{"mcharbor", "/mcharbor", "mcharbor-mcharbor-1"} {
		if _, ok := seen[candidate]; ok {
			continue
		}
		candidates = append(candidates, candidate)
		seen[candidate] = struct{}{}
	}
	return candidates
}

// CurrentMcHarborContainer returns the inspect response for the
// running McHarbor application container. It walks the candidates
// returned by CurrentMcHarborContainerCandidates and returns the
// first one that resolves to a container that passes
// IsMcHarborContainer. If no candidates work, it falls back to
// listing all containers to find McHarbor.
func CurrentMcHarborContainer(ctx context.Context, cli *client.Client) (container.InspectResponse, error) {
	var lastErr error
	for _, candidate := range CurrentMcHarborContainerCandidates() {
		inspectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		inspected, err := cli.ContainerInspect(inspectCtx, candidate, client.ContainerInspectOptions{})
		cancel()
		if err != nil {
			lastErr = err
			continue
		}
		current := inspected.Container
		if current.Config == nil {
			lastErr = fmt.Errorf("container %s has no config", candidate)
			continue
		}
		if IsMcHarborContainer([]string{current.Name}, current.Config.Image, current.Config.Labels) {
			return current, nil
		}
		lastErr = fmt.Errorf("container %s is not mcharbor", candidate)
	}

	// Fallback: list all containers and find McHarbor by labels/image
	// This works on environments where /proc and /etc files don't exist or
	// container auto-discovery via hostname/cgroup fails
	listCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	containersRes, err := cli.ContainerList(listCtx, client.ContainerListOptions{All: true})
	if err != nil {
		if lastErr != nil {
			return container.InspectResponse{}, fmt.Errorf("%w; fallback container list: %w", lastErr, err)
		}
		return container.InspectResponse{}, fmt.Errorf("container list fallback: %w", err)
	}
	containers := containersRes.Items

	for _, c := range containers {
		inspectCtx, inspectCancel := context.WithTimeout(ctx, 10*time.Second)
		inspected, err := cli.ContainerInspect(inspectCtx, c.ID, client.ContainerInspectOptions{})
		inspectCancel()
		if err != nil {
			continue
		}
		current := inspected.Container
		if current.Config == nil {
			continue
		}
		if IsMcHarborContainer([]string{current.Name}, current.Config.Image, current.Config.Labels) {
			return current, nil
		}
	}

	if lastErr != nil {
		return container.InspectResponse{}, lastErr
	}
	return container.InspectResponse{}, fmt.Errorf("no current container candidates found")
}

func currentContainerIDCandidates() []string {
	seen := map[string]struct{}{}
	var candidates []string
	add := func(value string) {
		value = strings.TrimSpace(strings.TrimPrefix(value, "/"))
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		candidates = append(candidates, value)
	}

	if hostname, err := os.Hostname(); err == nil {
		add(hostname)
	}
	if data, err := os.ReadFile("/etc/hostname"); err == nil {
		add(string(data))
	}

	for _, path := range []string{"/proc/self/mountinfo", "/proc/self/cgroup"} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, token := range regexp.MustCompile(`[0-9a-f]{12,64}`).FindAllString(string(data), -1) {
			if shortHexContainerIDRe.MatchString(token) {
				add(token)
			}
		}
	}

	return candidates
}

// MountForDestination returns the mount entry whose container path
// matches destination, copying its source/name into a fresh
// mount.Mount so callers can reuse it in a new container's HostConfig.
// Returns ok=false when no matching mount is found.
func MountForDestination(mounts []container.MountPoint, destination string) (mount.Mount, bool) {
	cleaned := filepath.Clean(destination)
	for _, mp := range mounts {
		if filepath.Clean(mp.Destination) != cleaned {
			continue
		}
		source := mp.Source
		// Volume-backed mounts expose the volume name in mp.Name;
		// bind mounts expose the host path in mp.Source. Either way
		// we need the host-visible identifier for the new container.
		if mp.Type == "volume" {
			source = mp.Name
		}
		if source == "" {
			return mount.Mount{}, false
		}
		return mount.Mount{
			Type:     mount.Type(mp.Type),
			Source:   source,
			Target:   mp.Destination,
			ReadOnly: !mp.RW,
		}, true
	}
	return mount.Mount{}, false
}
