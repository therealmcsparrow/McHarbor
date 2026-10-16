// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package instancebackup

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// HandleListSchedules serves GET /api/instance-backups/schedules.
func (h *Handler) HandleListSchedules(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListSchedules(r.Context())
	if err != nil {
		h.app.Logger.Error("instance backup: listing schedules", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, items)
}

func (h *Handler) decodeSchedule(w http.ResponseWriter, r *http.Request, creating bool) (ScheduleInput, bool) {
	var in ScheduleInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return in, false
	}
	if reason := in.Check(creating); reason != "" {
		response.BadRequestCodeWithData(w, r, i18n.ErrInvalidBody, InvalidReason{Reason: reason})
		return in, false
	}
	return in, true
}

// HandleCreateSchedule serves POST /api/instance-backups/schedules.
func (h *Handler) HandleCreateSchedule(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	in, ok := h.decodeSchedule(w, r, true)
	if !ok {
		return
	}
	sc, err := h.svc.CreateSchedule(r.Context(), user.Username, in)
	if err != nil {
		h.app.Logger.Error("instance backup: creating a schedule", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{Action: "create", EntityType: "instance_backup_schedule", EntityID: sc.ID, EntityName: sc.Name,
		Details: fmt.Sprintf("cron=%q key=%s retention=%d/%dd verify=%t", sc.Cron, sc.KeyKind, sc.RetentionCount, sc.RetentionDays, sc.VerifyAfter)})
	response.Created(w, sc)
}

// HandleUpdateSchedule serves PUT /api/instance-backups/schedules/{id}.
func (h *Handler) HandleUpdateSchedule(w http.ResponseWriter, r *http.Request) {
	in, ok := h.decodeSchedule(w, r, false)
	if !ok {
		return
	}
	sc, err := h.svc.UpdateSchedule(r.Context(), chi.URLParam(r, "id"), in)
	if h.writeScheduleError(w, r, err) {
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{Action: "update", EntityType: "instance_backup_schedule", EntityID: sc.ID, EntityName: sc.Name,
		Details: fmt.Sprintf("cron=%q enabled=%t key_replaced=%t", sc.Cron, sc.Enabled, in.Passphrase != "" || in.KeyFile != "")})
	response.OK(w, sc)
}

// HandleDeleteSchedule serves DELETE /api/instance-backups/schedules/{id}.
func (h *Handler) HandleDeleteSchedule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if h.writeScheduleError(w, r, h.svc.DeleteSchedule(r.Context(), id)) {
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{Action: "delete", EntityType: "instance_backup_schedule", EntityID: id})
	response.OK(w, map[string]string{"status": "deleted"})
}

// HandleRunSchedule serves POST /api/instance-backups/schedules/{id}/run.
func (h *Handler) HandleRunSchedule(w http.ResponseWriter, r *http.Request) {
	sc, err := h.svc.Schedule(r.Context(), chi.URLParam(r, "id"))
	if h.writeScheduleError(w, r, err) {
		return
	}
	h.svc.RunScheduleInBackground(*sc, h.uploader)
	h.app.AuditLog.Log(r, audit.Entry{Action: "run", EntityType: "instance_backup_schedule", EntityID: sc.ID, EntityName: sc.Name})
	response.OK(w, sc)
}

func (h *Handler) writeScheduleError(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrNotFound):
		response.NotFoundCode(w, r, i18n.ErrNotFound)
	default:
		h.app.Logger.Error("instance backup: schedule", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
	}
	return true
}
