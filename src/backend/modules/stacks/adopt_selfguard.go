// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package stacks

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	cerrdefs "github.com/containerd/errdefs"
	sdkclient "github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/selfguard"
)

// adoptTargetsMcHarbor reports whether adopting name (a discovered compose project) or
// containerID (a standalone container) would put McHarbor's own containers under a stack
// every stacks.manage holder could then stop, redeploy or delete.
func (s *Service) adoptTargetsMcHarbor(ctx context.Context, envID, name, containerID string) (bool, error) {
	if s.dockerPool == nil {
		return false, nil
	}
	if containerID != "" {
		cli, err := s.dockerPool.GetContext(ctx, envID)
		if err != nil {
			return false, fmt.Errorf("docker connection failed: %w", err)
		}
		inspectCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
		res, err := cli.ContainerInspect(inspectCtx, containerID, sdkclient.ContainerInspectOptions{})
		cancel()
		switch {
		case err == nil:
			if docker.ClassifyContainer(res.Container) != docker.SelfTargetNone {
				return true, nil
			}
		case !cerrdefs.IsNotFound(err):
			return false, fmt.Errorf("inspecting container %s: %w", containerID, err)
		}
	}
	if name == "" {
		return false, nil
	}
	targets, err := s.TargetsMcHarbor(ctx, envID, name)
	if err != nil {
		// Same policy as guardSelfStack: a daemon whose containers cannot be listed has no
		// project to match, and the container inspect above already decided a standalone.
		slog.Warn("stacks: self-target check before adopt failed", "name", name, "env", envID, "error", err)
		return false, nil
	}
	return targets, nil
}

// guardSelfAdopt refuses taking over a stack or container that runs McHarbor to a caller
// without host_self.manage, matching the Take over button, and reports whether the handler
// may continue. A container that cannot be inspected fails closed; an unlistable project
// passes, as it does for guardSelfStack.
func (h *Handler) guardSelfAdopt(w http.ResponseWriter, r *http.Request, envID string, req AdoptRequest) bool {
	targets, err := h.service.adoptTargetsMcHarbor(r.Context(), envID, req.Name, req.ContainerID)
	if err != nil {
		h.app.Logger.Error("stacks: self-target check before adopt failed", "name", req.Name, "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrStackAdoptFailed, err)
		return false
	}
	if !targets {
		return true
	}
	allowed, err := selfguard.Allowed(r.Context())
	if err != nil {
		h.app.Logger.Error("stacks: host_self.manage check failed", "name", req.Name, "env", envID, "op", "adopt", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return false
	}
	if allowed {
		return true
	}
	selfguard.Refuse(w, r, h.app.AuditLog, &selfguard.Refusal{Code: i18n.ErrHostSelfStackRequired, Target: stackSelfTarget},
		audit.Entry{EntityType: "stack", EntityName: req.Name, EnvironmentID: envID, Details: "op=adopt"})
	return false
}
