// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package versions

import (
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for version handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a version handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{
		app:     app,
		service: NewService(app.DB, app.AgentPool),
	}
}

// HandleInfo returns McHarbor and agent version information.
func (h *Handler) HandleInfo(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	info, err := h.service.Info()
	if err != nil {
		h.app.Logger.Error("failed to get version info", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	response.OK(w, info)
}
