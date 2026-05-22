// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package deployments

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for deployment HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new deployments handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{app: app, service: NewService(app.KubernetesPool)}
}

// HandleList returns all deployments.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := r.URL.Query().Get("env")
	namespace := r.URL.Query().Get("namespace")

	deps, err := h.service.List(r.Context(), envID, namespace)
	if err != nil {
		h.app.Logger.Error("failed to list deployments", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrDeploymentListFailed)
		return
	}

	response.OK(w, deps)
}

// HandleGet returns a single deployment.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := r.URL.Query().Get("env")
	namespace := chi.URLParam(r, "namespace")
	name := chi.URLParam(r, "name")

	dep, err := h.service.Get(r.Context(), envID, namespace, name)
	if err != nil {
		h.app.Logger.Error("failed to get deployment", "namespace", namespace, "name", name, "error", err)
		response.NotFoundCode(w, r, i18n.ErrDeploymentNotFound)
		return
	}

	response.OK(w, dep)
}

// HandleDelete deletes a deployment.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := r.URL.Query().Get("env")
	namespace := chi.URLParam(r, "namespace")
	name := chi.URLParam(r, "name")

	if err := h.service.Delete(r.Context(), envID, namespace, name); err != nil {
		h.app.Logger.Error("failed to delete deployment", "namespace", namespace, "name", name, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrDeploymentDeleteFailed)
		return
	}

	h.app.Logger.Info("deployment deleted", "namespace", namespace, "name", name, "user", user.Username)
	response.NoContent(w)
}

// HandleScale scales a deployment.
func (h *Handler) HandleScale(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := r.URL.Query().Get("env")
	namespace := chi.URLParam(r, "namespace")
	name := chi.URLParam(r, "name")

	var req ScaleRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if req.Replicas < 0 {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if err := h.service.Scale(r.Context(), envID, namespace, name, req.Replicas); err != nil {
		h.app.Logger.Error("failed to scale deployment", "namespace", namespace, "name", name, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrDeploymentScaleFailed)
		return
	}

	h.app.Logger.Info("deployment scaled", "namespace", namespace, "name", name, "replicas", req.Replicas, "user", user.Username)
	response.OK(w, map[string]any{"replicas": req.Replicas})
}

// HandleRestart restarts a deployment via rollout restart.
func (h *Handler) HandleRestart(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := r.URL.Query().Get("env")
	namespace := chi.URLParam(r, "namespace")
	name := chi.URLParam(r, "name")

	if err := h.service.Restart(r.Context(), envID, namespace, name); err != nil {
		h.app.Logger.Error("failed to restart deployment", "namespace", namespace, "name", name, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrDeploymentRestartFailed)
		return
	}

	h.app.Logger.Info("deployment restarted", "namespace", namespace, "name", name, "user", user.Username)
	response.OK(w, map[string]string{"status": "restarting"})
}
