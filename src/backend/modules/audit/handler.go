// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import (
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for audit HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new audit handler.
func NewHandler(app *router.AppDeps) *Handler {
	svc := NewService(app.DB)
	return &Handler{app: app, service: svc}
}

// HandleList returns paginated audit logs with optional filtering.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	page, perPage := response.ParsePagination(r)
	action := r.URL.Query().Get("action")
	entityType := r.URL.Query().Get("entity_type")

	logs, total, err := h.service.List(page, perPage, action, entityType)
	if err != nil {
		h.app.Logger.Error("failed to list audit logs", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	response.Paginated(w, logs, total, page, perPage)
}

// HandleCreate inserts a new audit log entry (internal use).
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

	if req.Action == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	// Auto-fill user info from auth context if not provided
	if req.UserID == nil {
		req.UserID = &user.ID
	}
	if req.Username == nil {
		req.Username = &user.Username
	}

	log, err := h.service.Create(req)
	if err != nil {
		h.app.Logger.Error("failed to create audit log", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	response.Created(w, log)
}
