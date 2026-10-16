// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"context"
	"errors"
	"fmt"
	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"
	"log/slog"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/secrets"
)

// errRefreshUnavailable means this container cannot take a rotated value
// without being rebuilt — it is not in file mode, it is not running, or some
// of its references are delivered through the environment, which Docker cannot
// change on a live container. The applier falls back to a recreate on it and
// on nothing else.
var errRefreshUnavailable = errors.New("secret files cannot be refreshed in place")

// secretReferenceBinder is the half of the vault that reference bookkeeping
// needs, kept separate from SecretResolver so the resolver interface stays the
// narrow thing every other call site uses — and so an existing test stub keeps
// compiling.
type secretReferenceBinder interface {
	RebindReference(ctx context.Context, resourceType, fromResourceID, toResourceID, resourceName string) error
	ClearReferences(ctx context.Context, resourceType, resourceID string) error
}

// bindSecretReferences moves the references just written under a container's
// name onto the id Docker has now assigned, and drops the ones the container
// it replaced left behind.
//
// Resolution happens before the container exists, so the name is the only
// identifier available at that moment. Rebinding afterwards is what makes a
// reference survive a rename, which a name-keyed row would not.
func (s *Service) bindSecretReferences(ctx context.Context, name, newID, replacedID string) {
	binder, ok := s.resolver.(secretReferenceBinder)
	if !ok || name == "" || newID == "" {
		return
	}
	if replacedID != "" && replacedID != newID {
		if err := binder.ClearReferences(ctx, "container", replacedID); err != nil {
			slog.Warn("containers: clearing secret references of replaced container failed",
				"container", replacedID, "error", err)
		}
	}
	if err := binder.RebindReference(ctx, "container", name, newID, name); err != nil {
		slog.Warn("containers: rebinding secret references failed",
			"container", name, "error", err)
	}
}

// RefreshContainerSecrets rewrites a running container's secret files so it
// picks up a rotated value without being recreated.
//
// Only file mode can do this: an environment variable is fixed for the life of
// a container, whereas a file under /run/secrets is just a file. Whether the
// process notices is the image's business — SecretSignalLabel is how an
// operator says which signal makes it re-read.
func (s *Service) RefreshContainerSecrets(ctx context.Context, envID, containerID string) error {
	planner, ok := s.resolver.(secretFilePlanner)
	if !ok {
		return errRefreshUnavailable
	}

	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return err
	}

	inspectCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	infoRes, err := cli.ContainerInspect(inspectCtx, containerID, client.ContainerInspectOptions{})
	info := infoRes.Container
	cancel()
	if err != nil {
		return fmt.Errorf("inspecting container for secret refresh: %w", err)
	}
	if info.Config == nil || info.State == nil || !info.State.Running {
		return errRefreshUnavailable
	}
	if info.Config.Labels[secretFileModeLabel] != "true" {
		return errRefreshUnavailable
	}

	refs := decodeSecretRefs(info.Config.Labels[secretRefsLabel])
	if len(refs) == 0 {
		return errRefreshUnavailable
	}

	owner := strings.TrimPrefix(info.Name, "/")
	plan, _, err := planner.PlanFileModeSlice(ctx, restoreSecretRefs(info.Config.Env, refs), secrets.ResolveScope{
		EnvironmentID: envID,
		ResourceType:  "container",
		ResourceID:    info.ID,
		ResourceName:  owner,
	})
	if err != nil {
		return err
	}
	// A reference interpolated into a larger string — a DSN, say — has no file
	// form and stays in the environment, so refreshing the files would leave it
	// on the old value while reporting success. Recreate is the honest answer.
	if len(plan.Replaced) != len(refs) {
		return errRefreshUnavailable
	}

	if err := docker.RefreshSecretFiles(ctx, cli, owner, info.Config.Image, plan.Files); err != nil {
		return err
	}
	slog.Info("container secret files refreshed in place",
		"container", owner, "variables", plan.Replaced)

	return docker.SignalContainer(ctx, cli, info.ID, info.Config.Labels[docker.SecretSignalLabel])
}

// secretApplier re-materialises one container so it runs with a secret's
// current value. Registered into the vault's applier registry at mount time;
// core/secrets cannot import this package, so the wiring goes this way round.
type secretApplier struct {
	service *Service
}

// Apply refreshes the container's secret files in place when it can, and
// recreates it when it cannot. Recreate already re-resolves every
// `${secret:NAME}` from the `com.mcharbor.secret-refs` label, so the zero
// request is the whole of "rebuild this exactly as it was, with today's
// values".
func (a secretApplier) Apply(ctx context.Context, target secrets.ApplyTarget) error {
	id, err := a.resolveContainer(ctx, target)
	if err != nil {
		return err
	}

	switch err := a.service.RefreshContainerSecrets(ctx, target.EnvironmentID, id); {
	case err == nil:
		return nil
	case !errors.Is(err, errRefreshUnavailable):
		// A refresh that started and failed may have written some files and
		// not others. Recreating on top of that would hide it; the operator
		// should see which container stopped the run.
		return err
	}

	if _, err := a.service.Recreate(ctx, target.EnvironmentID, id, RecreateRequest{}); err != nil {
		return err
	}
	return nil
}

// Resolve returns the id of the container a reference points at, and whether
// it is still there.
//
// The vault accumulated a reference per identifier a container ever had — its
// name, and the id of every incarnation a recreate replaced — because nothing
// used to delete them. Returning the resolved id is what lets the impact list
// fold the survivors back into one entry and drop the corpses, instead of
// showing one container four times.
func (a secretApplier) Resolve(ctx context.Context, target secrets.ApplyTarget) (string, bool, error) {
	id, err := a.resolveContainer(ctx, target)
	if err == nil {
		return id, true, nil
	}
	if errors.Is(err, errContainerGone) {
		return "", false, nil
	}
	return "", false, err
}

// errContainerGone separates "Docker says no such container" from "the daemon
// could not be reached". Only the first means the reference is prunable; the
// second must leave it alone.
var errContainerGone = errors.New("container no longer exists")

// resolveContainer finds the container a reference points at, by id first and
// by name second. References written before ids were rebound hold a name, and
// so does any reference whose container was recreated while McHarbor was down.
func (a secretApplier) resolveContainer(ctx context.Context, target secrets.ApplyTarget) (string, error) {
	cli, err := a.service.getClient(ctx, target.EnvironmentID)
	if err != nil {
		return "", err
	}

	for _, candidate := range []string{target.ResourceID, target.ResourceName} {
		if candidate == "" {
			continue
		}
		inspectCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
		infoRes, err := cli.ContainerInspect(inspectCtx, candidate, client.ContainerInspectOptions{})
		info := infoRes.Container
		cancel()
		if err == nil {
			return info.ID, nil
		}
		if !cerrdefs.IsNotFound(err) {
			return "", fmt.Errorf("inspecting container for secret apply: %w", err)
		}
	}
	return "", errContainerGone
}
