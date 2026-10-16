// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/selfguard"
)

// guardSelfRestore refuses restoring into one of McHarbor's own containers, or into a
// container of McHarbor's environment that reaches its host, unless ctx carries an
// operator's host_self.manage decision. A restore writes files into the container
// (CopyToContainer) and may recreate it from archived config, so it is gated like a file
// write and a recreate are; a restore with nobody behind it is refused outright.
func (s *Service) guardSelfRestore(ctx context.Context, envID, containerID string) error {
	return s.guardSelfRestoreFor(ctx, envID, containerID, nil)
}

// guardSelfRestoreFor is guardSelfRestore with an optional workflow decision: a workflow acts
// for the system, so its saved host_self flag replaces the request user's, and anything that
// cannot be classified is refused as if it were McHarbor's.
func (s *Service) guardSelfRestoreFor(ctx context.Context, envID, containerID string, workflowAuthorized *bool) error {
	if workflowAuthorized != nil && *workflowAuthorized {
		return nil
	}
	cli, err := s.client(ctx, envID)
	if err != nil {
		return s.workflowClassifyFailed(ctx, workflowAuthorized, envID, containerID, err)
	}
	inspectCtx, cancel := context.WithTimeout(ctx, coredocker.CallTimeout)
	res, err := cli.ContainerInspect(inspectCtx, containerID, client.ContainerInspectOptions{})
	cancel()
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			// The restore itself reports the missing container.
			return nil
		}
		return s.workflowClassifyFailed(ctx, workflowAuthorized, envID, containerID, fmt.Errorf("inspecting container before a self-protection check: %w", err))
	}
	target, reasons := s.pool.SelfTargetFor(ctx, envID, res.Container, true)
	if target == coredocker.SelfTargetNone {
		return nil
	}
	if workflowAuthorized != nil {
		if s.logger != nil {
			s.logger.Warn("container restore refused: workflow not authorized for McHarbor containers", "env", envID, "container", containerID, "target", target)
		}
		return &selfguard.Refusal{Code: i18n.ErrHostSelfWorkflowUnauthorized, Target: string(target), Reasons: reasons}
	}
	code := i18n.ErrHostSelfContainerRequired
	if target == coredocker.SelfTargetHostAccess {
		code = i18n.ErrHostSelfHostAccessRequired
	}
	if err := selfguard.RequireAttended(ctx, code, string(target), reasons); err != nil {
		if s.logger != nil {
			s.logger.Warn("container restore refused on a McHarbor container", "env", envID, "container", containerID, "target", target, "error", err)
		}
		return err
	}
	return nil
}

// workflowClassifyFailed keeps a user-driven restore's error as it was and makes a workflow's
// unclassifiable target a refusal, since an unknown answer may be McHarbor's own container.
func (s *Service) workflowClassifyFailed(_ context.Context, workflowAuthorized *bool, envID, containerID string, err error) error {
	if workflowAuthorized == nil {
		return err
	}
	if s.logger != nil {
		s.logger.Error("container restore self-protection check failed; refusing for a workflow", "env", envID, "container", containerID, "error", err)
	}
	return &selfguard.Refusal{Code: i18n.ErrHostSelfWorkflowUnauthorized, Target: "unknown"}
}

// refuseSelfRestore answers a host_self Refusal from a restore with a 403 audited against
// entry, and reports whether err was one.
func (h *Handler) refuseSelfRestore(w http.ResponseWriter, r *http.Request, entry audit.Entry, err error) bool {
	var ref *selfguard.Refusal
	if !errors.As(err, &ref) {
		return false
	}
	selfguard.Refuse(w, r, h.app.AuditLog, ref, entry)
	return true
}
