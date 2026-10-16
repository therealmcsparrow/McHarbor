// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// HandleListSchedules serves GET /api/vms/{id}/snapshot-schedules.
func (h *Handler) HandleListSchedules(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	schedules, err := h.svc.ListSchedules(r.Context(), id)
	if err != nil {
		h.respondErr(w, r, "list snapshot schedules", err)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("runs"))
	runs, err := h.svc.ListScheduleRuns(r.Context(), id, limit)
	if err != nil {
		h.respondErr(w, r, "list snapshot runs", err)
		return
	}
	ok(w, map[string]any{"schedules": schedules, "runs": runs})
}

// HandleCreateSchedule serves POST /api/vms/{id}/snapshot-schedules.
func (h *Handler) HandleCreateSchedule(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")

	var req ScheduleRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	schedule, err := h.svc.CreateSchedule(r.Context(), id, req)
	if err != nil {
		h.respondErr(w, r, "create snapshot schedule", err)
		return
	}

	h.emit(userID, "plugin.kvm.snapshot_schedule.created", map[string]any{
		"vmId": id, "scheduleId": schedule.ID,
		"cron": schedule.Cron, "keepCount": schedule.KeepCount,
		"withMemory": schedule.WithMemory,
	})
	created(w, schedule, msgScheduleSaved)
}

// HandleUpdateSchedule serves PUT /api/vms/{id}/snapshot-schedules/{scheduleId}.
func (h *Handler) HandleUpdateSchedule(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	scheduleID := chi.URLParam(r, "scheduleId")

	var req ScheduleRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	schedule, err := h.svc.UpdateSchedule(r.Context(), id, scheduleID, req)
	if err != nil {
		h.respondErr(w, r, "update snapshot schedule", err)
		return
	}

	h.emit(userID, "plugin.kvm.snapshot_schedule.updated", map[string]any{
		"vmId": id, "scheduleId": scheduleID,
		"cron": schedule.Cron, "enabled": schedule.Enabled,
		"keepCount": schedule.KeepCount,
	})
	okData(w, schedule, msgScheduleSaved)
}

// HandleDeleteSchedule serves DELETE /api/vms/{id}/snapshot-schedules/{scheduleId}.
//
// The snapshots the schedule made are kept: they are the guest's restore
// points, and deleting a policy is not a request to throw away what it produced.
func (h *Handler) HandleDeleteSchedule(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	scheduleID := chi.URLParam(r, "scheduleId")

	if err := h.svc.DeleteSchedule(r.Context(), id, scheduleID); err != nil {
		h.respondErr(w, r, "delete snapshot schedule", err)
		return
	}

	h.emit(userID, "plugin.kvm.snapshot_schedule.deleted", map[string]any{
		"vmId": id, "scheduleId": scheduleID,
	})
	okMsg(w, msgScheduleDeleted)
}

// HandleRunSchedule serves POST /api/vms/{id}/snapshot-schedules/{scheduleId}/run.
//
// The "run now" button. It goes through exactly the same path the ticker does,
// retention included, so testing a schedule tells the operator what it will
// actually do rather than what a simplified version of it would.
func (h *Handler) HandleRunSchedule(w http.ResponseWriter, r *http.Request) {
	userID, authed := requireUser(r)
	if !authed {
		fail(w, r, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	scheduleID := chi.URLParam(r, "scheduleId")

	schedule, err := h.svc.schedule(r.Context(), id, scheduleID)
	if err != nil {
		h.respondErr(w, r, "read snapshot schedule", err)
		return
	}

	h.emit(userID, "plugin.kvm.snapshot_schedule.run", map[string]any{
		"vmId": id, "scheduleId": scheduleID,
	})

	if err := h.svc.RunSchedule(r.Context(), schedule); err != nil {
		h.respondErr(w, r, "run snapshot schedule", err)
		return
	}
	okMsg(w, msgScheduleRan)
}
