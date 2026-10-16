// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package networks

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/selfguard"
)

// guardContainer is the containers routes' classify-and-refuse step for a network connect or
// disconnect: moving McHarbor's own container between networks needs host_self.manage. It
// returns the inspected container ID, so the operation acts on what was checked rather than
// on a name that could be re-pointed in between; an unknown container is left to fail itself.
func (s *Service) guardContainer(ctx context.Context, envID, ref string) (string, error) {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return "", err
	}
	inspectCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()
	res, err := cli.ContainerInspect(inspectCtx, ref, client.ContainerInspectOptions{})
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return ref, nil
		}
		return "", fmt.Errorf("inspecting container %s: %w", ref, err)
	}
	kind := docker.ClassifyContainer(res.Container)
	if kind != docker.SelfTargetNone {
		if err := selfguard.RequireAttended(ctx, i18n.ErrHostSelfContainerRequired, string(kind), nil); err != nil {
			return "", err
		}
	}
	return res.Container.ID, nil
}

// writeSelfRefusal answers a host_self Refusal from Connect or Disconnect with an audited 403
// and reports whether err was one.
func (h *Handler) writeSelfRefusal(w http.ResponseWriter, r *http.Request, envID, networkID, container, op string, err error) bool {
	var ref *selfguard.Refusal
	if !errors.As(err, &ref) {
		return false
	}
	selfguard.Refuse(w, r, h.app.AuditLog, ref, audit.Entry{
		EntityType: "container", EntityName: container, EnvironmentID: envID,
		Details: "op=network_" + op + " network=" + networkID,
	})
	return true
}
