// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package dockerinfo

import (
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for docker info HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new docker info handler.
func NewHandler(app *router.AppDeps) *Handler {
	svc := NewService(app.DockerPool)
	return &Handler{app: app, service: svc}
}

// HandleSystemInfo returns extended Docker daemon info.
func (h *Handler) HandleSystemInfo(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	info, err := h.service.SystemInfo(r.Context(), envID)
	if err != nil {
		h.app.Logger.Error("failed to get Docker system info", "error", err, "env", envID)
		response.InternalErrorCode(w, r, i18n.ErrDockerInfoFailed)
		return
	}

	response.OK(w, info)
}
