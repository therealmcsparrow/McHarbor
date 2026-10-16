// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package runbooks

import (
	"context"
	"errors"
	"fmt"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/selfguard"
)

// guardSelfTarget refuses a start, stop or restart aimed at a McHarbor container (this node, a peer,
// a helper or an operator-protected one) unless the running user holds host_self.manage.
// It uses selfguard.Attended, so a run with nobody behind it is refused rather than let through.
func (s *Service) guardSelfTarget(ctx context.Context, envID string, step Step) error {
	target := stepTarget(step)
	if target == "" || s.RuntimeFor == nil {
		return nil
	}
	rt, err := s.RuntimeFor(envID)
	if err != nil {
		return fmt.Errorf("reaching the environment: %w", err)
	}
	res, err := rt.ContainerInspect(ctx, target, client.ContainerInspectOptions{})
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return nil
		}
		// Unwrapped on purpose: the step fails with the runtime's own message, exactly as the action would have.
		return err
	}
	kind := coredocker.ClassifyContainer(res.Container)
	if kind == coredocker.SelfTargetNone {
		return nil
	}
	err = selfguard.RequireAttended(ctx, i18n.ErrHostSelfContainerRequired, string(kind), nil)
	if err == nil {
		return nil
	}
	var ref *selfguard.Refusal
	if !errors.As(err, &ref) {
		return err
	}
	selfguard.AuditRefusal(ctx, ref, audit.Entry{
		EntityType: "container", EntityID: res.Container.ID, EntityName: target, EnvironmentID: envID,
		Details: "op=runbook." + step.ActionKey,
	})
	return &ForbiddenError{Permission: rbac.PermHostSelfManage}
}
