// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package activity

import (
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for activity HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new activity handler.
func NewHandler(app *router.AppDeps) *Handler {
	svc := NewService(app.DB)
	return &Handler{app: app, service: svc}
}

// HandleList returns paginated container events.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	page, perPage := response.ParsePagination(r)
	envID := response.ParseEnvID(r)

	events, total, err := h.service.List(page, perPage, envID)
	if err != nil {
		h.app.Logger.Error("failed to list activity events", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	response.Paginated(w, events, total, page, perPage)
}

// HandleCreate records a new container event.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	var req CreateRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if req.ContainerID == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if req.EventType == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if req.Action == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	evt, err := h.service.Create(req)
	if err != nil {
		h.app.Logger.Error("failed to create activity event", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	response.Created(w, evt)
}
