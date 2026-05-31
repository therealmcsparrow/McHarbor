// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package docker

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/docker/docker/client"
)

const (
	ProtectedLabel      = "com.mcharbor.protected"
	composeProjectLabel = "com.docker.compose.project"
	composeServiceLabel = "com.docker.compose.service"
	mcHarborProject     = "mcharbor"
	mcHarborService     = "mcharbor"
	mcHarborAgent       = "mcharbor-agent"
)

var ErrProtectedResource = errors.New("protected mcharbor resource")

// IsProtectedContainer returns true for containers McHarbor should not mutate
// through normal container actions.
func IsProtectedContainer(names []string, image string, labels map[string]string) bool {
	if isProtectedLabel(labels) {
		return true
	}
	return IsMcHarborContainer(names, image, labels)
}

// IsMcHarborContainer returns true for the McHarbor application container.
func IsMcHarborContainer(names []string, image string, labels map[string]string) bool {
	if labels[composeProjectLabel] == mcHarborProject && labels[composeServiceLabel] == mcHarborService {
		return true
	}
	if imageRefIsMcHarbor(image) {
		return true
	}
	for _, name := range names {
		normalized := strings.ToLower(strings.Trim(strings.TrimSpace(name), "/"))
		if normalized == mcHarborService || strings.HasPrefix(normalized, "mcharbor-mcharbor-") {
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
	inspectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	info, err := cli.ContainerInspect(inspectCtx, id)
	if err != nil {
		return err
	}

	image := ""
	labels := map[string]string{}
	if info.Config != nil {
		image = info.Config.Image
		labels = info.Config.Labels
	}
	if IsProtectedContainer([]string{info.Name}, image, labels) {
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

func imageRefIsMcHarbor(ref string) bool {
	repo := imageRepository(ref)
	return repo == "ghcr.io/therealmcsparrow/mcharbor" ||
		repo == "therealmcsparrow/mcharbor" ||
		repo == "mcharbor"
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
