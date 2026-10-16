// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package dashboard aggregates Docker resource counts and host metric history into the figures the dashboard page and its widgets render.
package dashboard

import (
	"context"
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
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
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	response.OK(w, stats)
}

// HandleSummary returns the estate-wide counts behind the dashboard page head.
//
// Each figure is computed only over the environments this caller may read, and
// a resource they cannot read is left out of the response entirely.
func (h *Handler) HandleSummary(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	ctx := r.Context()
	ids, err := h.service.EnvironmentIDs(ctx)
	if err != nil {
		h.app.Logger.Error("dashboard: listing environments for summary", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	visible := make([]string, 0, len(ids))
	for _, id := range ids {
		if h.allows(ctx, user.ID, id, rbac.PermEnvironmentsView) {
			visible = append(visible, id)
		}
	}

	wantContainers := false
	wantStacks := false
	for _, id := range visible {
		if !wantContainers && h.allows(ctx, user.ID, id, rbac.PermContainersView) {
			wantContainers = true
		}
		if !wantStacks && h.allows(ctx, user.ID, id, rbac.PermStacksView) {
			wantStacks = true
		}
	}

	response.OK(w, h.service.Summary(ctx, visible, wantContainers, wantStacks))
}

// allows reports whether the user holds perm on envID, treating a lookup
// failure as denial so a broken check can never widen what is counted.
func (h *Handler) allows(ctx context.Context, userID, envID string, perm rbac.Permission) bool {
	if h.app.RBACService == nil {
		return true
	}
	ok, err := h.app.RBACService.HasPermission(ctx, userID, envID, perm)
	if err != nil {
		h.app.Logger.Warn("dashboard: permission check failed", "env", envID, "permission", perm, "error", err)
		return false
	}
	return ok
}
