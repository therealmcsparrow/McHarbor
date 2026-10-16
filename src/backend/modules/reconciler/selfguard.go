// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"errors"
	"net/http"

	"github.com/moby/moby/api/types/container"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/selfguard"
)

// reconcileActs reports whether HandleReconcile would start, stop or remove the inspected
// container to reach ds's desired status.
func reconcileActs(ds *DesiredState, exists bool, info container.InspectResponse) bool {
	if !exists {
		return false
	}
	running := info.State != nil && info.State.Running
	switch ds.DesiredStatus {
	case "running":
		return !running
	case "stopped":
		return running
	case "removed":
		return true
	}
	return false
}

// guardSelfReconcile refuses a reconcile that would act on one of McHarbor's own (or an
// operator-protected) containers unless the caller holds host_self.manage, and reports
// whether the handler may continue. The refusal is audited like the containers module's.
func (h *Handler) guardSelfReconcile(w http.ResponseWriter, r *http.Request, ds *DesiredState, info container.InspectResponse) bool {
	kind := docker.ClassifyContainer(info)
	if kind == docker.SelfTargetNone {
		return true
	}
	err := selfguard.RequireAttended(r.Context(), i18n.ErrHostSelfContainerRequired, string(kind), nil)
	if err == nil {
		return true
	}
	var ref *selfguard.Refusal
	if !errors.As(err, &ref) {
		h.app.Logger.Error("reconciler: deciding host_self.manage failed", "id", ds.ID, "env", ds.EnvID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return false
	}
	h.app.Logger.Warn("reconciler: refused to act on a McHarbor container", "id", ds.ID, "env", ds.EnvID, "container", ds.ContainerName, "target", ref.Target)
	selfguard.Refuse(w, r, h.app.AuditLog, ref, audit.Entry{
		EntityType:    "container",
		EntityID:      info.ID,
		EntityName:    ds.ContainerName,
		EnvironmentID: ds.EnvID,
		Details:       "op=reconcile desired=" + ds.DesiredStatus,
	})
	return false
}
