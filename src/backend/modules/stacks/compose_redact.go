// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package stacks

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/moby/moby/api/types/container"

	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/envredact"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// stackPinned returns the root-of-trust values this process runs with, the backup key
// included, for envredact.Policy.Pinned.
func stackPinned(app *router.AppDeps) []string {
	var extra []string
	if app.BackupCrypto != nil {
		extra = append(extra, app.BackupCrypto.KeyMaterialBase64())
	}
	return envredact.PinnedFromConfig(app.Config, extra...)
}

// composeDetector answers envredact.McHarborService for one stack with core/docker's rule:
// a service whose running container is classified as McHarbor, or whose name, image or
// compose identity (project stack, service name) says so. A deployed service is named
// <stack>_<service> or <stack>-<service>-1, so both are offered as container names.
func composeDetector(stack string, live map[string]bool) envredact.McHarborService {
	return func(service, image string) bool {
		if live[service] {
			return true
		}
		labels := map[string]string{"com.docker.compose.project": stack, "com.docker.compose.service": service}
		names := []string{service, stack + "_" + service, stack + "-" + service + "-1"}
		return docker.IsMcHarborContainer(names, image, labels)
	}
}

// liveMcHarborServices lists the services of stack whose container core/docker classifies as
// a McHarbor application container — by its own id, role label or image — which catches a
// renamed service the compose text alone cannot. A daemon that cannot be read yields none.
func (s *Service) liveMcHarborServices(ctx context.Context, envID, stack string) map[string]bool {
	if s.dockerPool == nil {
		return nil
	}
	containers, err := s.StackContainers(ctx, envID, stack)
	if err != nil {
		slog.Debug("stacks: reading stack containers for McHarbor detection failed", "stack", stack, "error", err)
		return nil
	}
	out := map[string]bool{}
	for _, c := range containers {
		if docker.ClassifySummary(c.ID, c.Names, c.Image, c.Labels) == docker.SelfTargetApp {
			out[c.Labels["com.docker.compose.service"]] = true
		}
	}
	return out
}

// restoreMaskedCompose puts the stored value back wherever an edited compose still carries
// the marker, reading it from the file about to be replaced; it fails rather than write one,
// and refuses to move a value nobody may reveal (envredact.ErrComposeMaskedMoved).
func restoreMaskedCompose(policy envredact.Policy, detector envredact.McHarborService, content, composePath string) (string, error) {
	if !strings.Contains(content, envredact.Marker) {
		return content, nil
	}
	stored, err := os.ReadFile(composePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("reading compose file to restore masked values: %w", err)
	}
	return policy.RestoreCompose(content, string(stored), detector)
}

// maskAdoptEnv withholds, before an adopt preview is built, the values the caller may not
// see: credential-looking values unless reveal (containers.manage, audited by the handler),
// and a McHarbor container's own keys and any value carrying a pinned key always.
func maskAdoptEnv(inspected []container.InspectResponse, pinned []string, reveal bool) {
	for i := range inspected {
		envredact.Policy{Reveal: reveal, McHarbor: docker.IsMcHarborInstance(inspected[i]), Pinned: pinned}.Container(&inspected[i])
	}
}

// restoreAdoptCompose puts back the values a masked adopt preview withheld, reading them
// from the containers being adopted as a revealed preview would show them. A value no reveal
// shows stays the marker, so the restore refuses rather than write it.
func (s *Service) restoreAdoptCompose(ctx context.Context, envID string, req AdoptRequest) (string, error) {
	source := AdoptPreviewRequest{ContainerID: req.ContainerID}
	if source.ContainerID == "" {
		source.StackName = req.Name
	}
	if s.dockerPool == nil {
		return "", fmt.Errorf("%w: the adopt preview withheld a value that must be typed in", envredact.ErrComposeMaskedValue)
	}
	live, err := s.previewAdopt(ctx, envID, source, true)
	if err != nil {
		return "", fmt.Errorf("%w: the adopt preview withheld a value that must be typed in: %w", envredact.ErrComposeMaskedValue, err)
	}
	return envredact.Policy{Pinned: s.pinned}.RestoreCompose(req.Compose, live.Compose, nil)
}
