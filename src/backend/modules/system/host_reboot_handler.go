// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package system

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// rebootRefusalData is the data a refused reboot request carries beside its
// i18n code: the machine reason the UI translates, and the detail.
type rebootRefusalData struct {
	Reason string `json:"reason"`
	Detail string `json:"detail,omitempty"`
}

// HandleHostRebootState returns the environment host's reboot row.
func (h *Handler) HandleHostRebootState(w http.ResponseWriter, r *http.Request) {
	h.serveHostRebootState(w, r, EnvTarget(response.ParseEnvID(r)))
}

// HandleSelfHostRebootState returns the reboot row of McHarbor's own host.
func (h *Handler) HandleSelfHostRebootState(w http.ResponseWriter, r *http.Request) {
	h.serveHostRebootState(w, r, SelfTarget())
}

func (h *Handler) serveHostRebootState(w http.ResponseWriter, r *http.Request, target HostTarget) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	state, err := h.reboots.State(r.Context(), target)
	if errors.Is(err, errRebootTargetMissing) {
		response.NotFoundCode(w, r, i18n.ErrEnvNotFound)
		return
	}
	if err != nil {
		h.app.Logger.Error("system: host reboot state failed", "target", target.rowID(), "error", err)
		response.InternalErrorCause(w, r, i18n.ErrHostUpdateStateFailed, err)
		return
	}
	response.OK(w, state)
}

// HandleHostRebootInventory reports what a reboot of the environment host stops.
func (h *Handler) HandleHostRebootInventory(w http.ResponseWriter, r *http.Request) {
	h.serveHostRebootInventory(w, r, EnvTarget(response.ParseEnvID(r)))
}

// HandleSelfHostRebootInventory reports what a reboot of McHarbor's own host stops.
func (h *Handler) HandleSelfHostRebootInventory(w http.ResponseWriter, r *http.Request) {
	h.serveHostRebootInventory(w, r, SelfTarget())
}

func (h *Handler) serveHostRebootInventory(w http.ResponseWriter, r *http.Request, target HostTarget) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	inventory, err := h.reboots.Inventory(r.Context(), target)
	if errors.Is(err, errRebootTargetMissing) {
		response.NotFoundCode(w, r, i18n.ErrEnvNotFound)
		return
	}
	if err != nil {
		h.app.Logger.Error("system: host reboot inventory failed", "target", target.rowID(), "error", err)
		response.InternalErrorCause(w, r, i18n.ErrHostOSInfoFailed, err)
		return
	}
	response.OK(w, inventory)
}

// HandleHostRebootSchedule schedules a reboot of the environment host.
func (h *Handler) HandleHostRebootSchedule(w http.ResponseWriter, r *http.Request) {
	h.serveHostRebootSchedule(w, r, EnvTarget(response.ParseEnvID(r)))
}

// HandleSelfHostRebootSchedule schedules a reboot of McHarbor's own host.
func (h *Handler) HandleSelfHostRebootSchedule(w http.ResponseWriter, r *http.Request) {
	h.serveHostRebootSchedule(w, r, SelfTarget())
}

func (h *Handler) serveHostRebootSchedule(w http.ResponseWriter, r *http.Request, target HostTarget) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	var req HostRebootRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	rec, err := h.reboots.Schedule(r.Context(), target, req, user.ID, user.Username)
	if err != nil {
		h.writeRebootError(w, r, target, err)
		return
	}

	entityType, entityID := auditEntity(target)
	entry := audit.Entry{
		Action:     "host_reboot_scheduled",
		EntityType: entityType,
		EntityID:   entityID,
		Details: fmt.Sprintf("host reboot scheduled for %s (drainedExternally=%t)",
			rec.ScheduledAt.Format(time.RFC3339), req.DrainedExternally),
	}
	if !target.Self {
		entry.EnvironmentID = entityID
	}
	h.app.AuditLog.Log(r, entry)
	response.OK(w, rec)
}

// HandleHostRebootCancel cancels a scheduled reboot of the environment host.
func (h *Handler) HandleHostRebootCancel(w http.ResponseWriter, r *http.Request) {
	h.serveHostRebootCancel(w, r, EnvTarget(response.ParseEnvID(r)))
}

// HandleSelfHostRebootCancel cancels a scheduled reboot of McHarbor's own host.
func (h *Handler) HandleSelfHostRebootCancel(w http.ResponseWriter, r *http.Request) {
	h.serveHostRebootCancel(w, r, SelfTarget())
}

func (h *Handler) serveHostRebootCancel(w http.ResponseWriter, r *http.Request, target HostTarget) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	if err := h.reboots.Cancel(r.Context(), target); err != nil {
		h.writeRebootError(w, r, target, err)
		return
	}
	entityType, entityID := auditEntity(target)
	entry := audit.Entry{
		Action:     "host_reboot_cancelled",
		EntityType: entityType,
		EntityID:   entityID,
		Details:    "scheduled host reboot cancelled",
	}
	if !target.Self {
		entry.EnvironmentID = entityID
	}
	h.app.AuditLog.Log(r, entry)
	response.OK(w, map[string]bool{"cancelled": true})
}

// writeRebootError maps a refusal onto a status and a translatable reason; any
// other error is logged and answered with a generic code.
func (h *Handler) writeRebootError(w http.ResponseWriter, r *http.Request, target HostTarget, err error) {
	if errors.Is(err, errRebootTargetMissing) {
		response.NotFoundCode(w, r, i18n.ErrEnvNotFound)
		return
	}
	refusal, ok := AsRebootRefusal(err)
	if !ok {
		h.app.Logger.Error("system: host reboot request failed", "target", target.rowID(), "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	data := rebootRefusalData{Reason: refusal.Reason, Detail: refusal.Detail}
	switch refusal.Reason {
	case RebootReasonDemoMode:
		response.ErrCodeWithData(w, r, http.StatusForbidden, i18n.ErrForbidden, data)
	case RebootReasonInProgress:
		response.ErrCodeWithData(w, r, http.StatusConflict, i18n.ErrForbidden, data)
	case RebootReasonNotScheduled:
		response.ErrCodeWithData(w, r, http.StatusNotFound, i18n.ErrNotFound, data)
	default:
		response.ErrCodeWithData(w, r, http.StatusBadRequest, i18n.ErrInvalidBody, data)
	}
}
