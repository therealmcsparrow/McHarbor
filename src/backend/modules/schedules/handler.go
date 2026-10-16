// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package schedules exposes the schedule listing and management endpoints.
package schedules

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for schedule HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new schedules handler.
func NewHandler(app *router.AppDeps) *Handler {
	svc := NewService(app.DB)
	return &Handler{app: app, service: svc}
}

// HandleList returns a paginated list of schedules.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	page, perPage := response.ParsePagination(r)

	items, total, err := h.service.List(r.Context(), page, perPage)
	if err != nil {
		h.app.Logger.Error("schedules: list error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrScheduleListFailed, err)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}

// HandleCreate creates a new schedule.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var input CreateScheduleInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if input.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrScheduleNameRequired)
		return
	}
	if input.Cron == "" {
		response.BadRequestCode(w, r, i18n.ErrScheduleCronRequired)
		return
	}
	if input.Action == "" {
		response.BadRequestCode(w, r, i18n.ErrScheduleActionRequired)
		return
	}
	if input.Target == "" {
		response.BadRequestCode(w, r, i18n.ErrScheduleTargetRequired)
		return
	}

	sched, err := h.service.Create(r.Context(), input)
	if err != nil {
		h.app.Logger.Error("schedules: create error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrScheduleCreateFailed, err)
		return
	}

	response.Created(w, sched)
}

// HandleGet returns a single schedule.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	sched, err := h.service.ByID(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("schedules: get error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrScheduleListFailed, err)
		return
	}
	if sched == nil {
		response.NotFoundCode(w, r, i18n.ErrScheduleNotFound)
		return
	}

	response.OK(w, sched)
}

// HandleUpdate updates an existing schedule.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input UpdateScheduleInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	sched, err := h.service.Update(r.Context(), id, input)
	if err != nil {
		h.app.Logger.Error("schedules: update error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrScheduleUpdateFailed, err)
		return
	}
	if sched == nil {
		response.NotFoundCode(w, r, i18n.ErrScheduleNotFound)
		return
	}

	response.OK(w, sched)
}

// HandleDelete removes a schedule.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	deleted, err := h.service.Delete(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("schedules: delete error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrScheduleRemoveFailed, err)
		return
	}
	if !deleted {
		response.NotFoundCode(w, r, i18n.ErrScheduleNotFound)
		return
	}

	response.NoContent(w)
}

// HandleListExecutions returns execution history for a schedule.
func (h *Handler) HandleListExecutions(w http.ResponseWriter, r *http.Request) {
	scheduleID := chi.URLParam(r, "id")
	page, perPage := response.ParsePagination(r)

	items, total, err := h.service.ListExecutions(r.Context(), scheduleID, page, perPage)
	if err != nil {
		h.app.Logger.Error("schedules: executions list error", "error", err, "scheduleId", scheduleID)
		response.InternalErrorCause(w, r, i18n.ErrScheduleListFailed, err)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}
