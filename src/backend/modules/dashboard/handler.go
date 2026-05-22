// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package dashboard

import (
	"net/http"
	"strconv"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for dashboard HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new dashboard handler.
func NewHandler(app *router.AppDeps) *Handler {
	svc := NewService(app.DB, app.DockerPool)
	return &Handler{app: app, service: svc}
}

// HandleStats returns overall Docker resource counts.
func (h *Handler) HandleStats(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	stats, err := h.service.Stats(r.Context(), envID)
	if err != nil {
		h.app.Logger.Error("failed to get dashboard stats", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	response.OK(w, stats)
}

// HandleMetrics returns historical host metrics time series.
func (h *Handler) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	metrics, err := h.service.Metrics(envID, limit)
	if err != nil {
		h.app.Logger.Error("failed to get dashboard metrics", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	response.OK(w, metrics)
}
