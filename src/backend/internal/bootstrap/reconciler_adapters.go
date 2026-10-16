// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	cerrdefs "github.com/containerd/errdefs"

	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/selfguard"
	"github.com/therealmcsparrow/mcharbor/modules/containers"
	"github.com/therealmcsparrow/mcharbor/modules/reconciler"
	"github.com/therealmcsparrow/mcharbor/modules/stacks"
)

// NewReconcilerActuator returns the reconciler's Actuator over the containers
// and stacks services, so a reconciler action takes exactly the path a person's
// click would, protected containers and secret resolution included.
func NewReconcilerActuator(c *containers.Service, s *stacks.Service) reconciler.Actuator {
	return reconcilerActuator{Containers: c, Stacks: s}
}

type reconcilerActuator struct {
	Containers *containers.Service
	Stacks     *stacks.Service
}

// Start starts a container by name.
func (a reconcilerActuator) Start(ctx context.Context, envID, name string) error {
	return a.Containers.Start(ctx, envID, name)
}

// Stop stops a container by name with Docker's default grace period.
func (a reconcilerActuator) Stop(ctx context.Context, envID, name string) error {
	return a.Containers.Stop(ctx, envID, name, 10)
}

// Remove force-removes a container and keeps its anonymous volumes, which
// are data a desired state never declared and so never gets to delete.
func (a reconcilerActuator) Remove(ctx context.Context, envID, name string) error {
	return a.Containers.Remove(ctx, envID, name, true, false)
}

// Recreate recreates a container with the plan's overrides. On one of McHarbor's own
// containers it needs an operator holding host_self.manage; an enforcement pass has none.
func (a reconcilerActuator) Recreate(ctx context.Context, envID, name string, req docker.RecreateRequest) error {
	if err := a.guardSelfContainer(ctx, envID, name); err != nil {
		return err
	}
	_, err := a.Containers.Recreate(ctx, envID, name, req)
	return err
}

// StackUp runs compose up for a stack McHarbor manages. A stack that runs McHarbor is
// redeployed only for an operator holding host_self.manage.
func (a reconcilerActuator) StackUp(ctx context.Context, stack string) error {
	targets, err := a.Stacks.TargetsMcHarbor(ctx, "", stack)
	if err != nil {
		return fmt.Errorf("checking whether stack %s runs McHarbor: %w", stack, err)
	}
	if targets {
		if err := selfguard.RequireAttended(ctx, i18n.ErrHostSelfStackRequired, "stack", nil); err != nil {
			slog.Warn("reconciler: refused to redeploy a stack that runs McHarbor", "stack", stack, "error", err)
			return err
		}
	}
	res := a.Stacks.Up(ctx, stack)
	if res == nil || !res.Success {
		msg := "compose up failed"
		if res != nil && strings.TrimSpace(res.Error) != "" {
			msg = res.Error
		}
		return errors.New(msg)
	}
	return nil
}

// guardSelfContainer refuses acting on a McHarbor or operator-protected container unless
// ctx carries an operator's host_self.manage decision. The enforcement loop runs with no
// user, which selfguard.Allowed would read as McHarbor acting for itself.
func (a reconcilerActuator) guardSelfContainer(ctx context.Context, envID, name string) error {
	info, err := a.Containers.Inspect(ctx, envID, name)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("inspecting %s before a self-protection check: %w", name, err)
	}
	kind := docker.ClassifyContainer(info)
	if kind == docker.SelfTargetNone {
		return nil
	}
	if err := selfguard.RequireAttended(ctx, i18n.ErrHostSelfContainerRequired, string(kind), nil); err != nil {
		slog.Warn("reconciler: refused to act on a McHarbor container", "env", envID, "container", name, "error", err)
		return err
	}
	return nil
}
