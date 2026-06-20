// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package autoheal

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for the autoheal HTTP endpoints.
type Handler struct {
	app *router.AppDeps
	svc *Service
}

// NewHandler creates a new autoheal handler.
func NewHandler(app *router.AppDeps, svc *Service) *Handler {
	return &Handler{app: app, svc: svc}
}

// HandleGetPreference returns the in-memory preference state for a container.
func (h *Handler) HandleGetPreference(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	containerID := chi.URLParam(r, "id")
	if containerID == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	pref := h.svc.GetPreference(envID, containerID)
	if pref.ContainerName == "" {
		// Fall back to the id so the UI can still render the row.
		pref.ContainerName = containerID
	}
	response.OK(w, pref)
}

// HandleSetPreference enables or disables auto-heal for a container. The
// change is applied as a Docker label in place — no recreate required.
func (h *Handler) HandleSetPreference(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := response.ParseEnvID(r)
	containerID := chi.URLParam(r, "id")
	if containerID == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	var req PreferenceRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if err := h.svc.SetPreference(r.Context(), envID, containerID, req.Enabled); err != nil {
		h.app.Logger.Error("autoheal: set preference failed", "env", envID, "container", containerID, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInvalidBody)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:        "autoheal",
		EntityType:    "container",
		EntityID:      containerID,
		EnvironmentID: envID,
		Details:       "preference=" + boolStr(req.Enabled),
	})

	response.OK(w, h.svc.GetPreference(envID, containerID))
}

func boolStr(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
