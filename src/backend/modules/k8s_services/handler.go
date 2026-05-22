// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package k8s_services

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for K8s service HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new k8s services handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{app: app, service: NewService(app.KubernetesPool)}
}

// HandleList returns all Kubernetes services.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := r.URL.Query().Get("env")
	namespace := r.URL.Query().Get("namespace")

	svcs, err := h.service.List(r.Context(), envID, namespace)
	if err != nil {
		h.app.Logger.Error("failed to list k8s services", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrK8sServiceListFailed)
		return
	}

	response.OK(w, svcs)
}

// HandleGet returns a single Kubernetes service.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := r.URL.Query().Get("env")
	namespace := chi.URLParam(r, "namespace")
	name := chi.URLParam(r, "name")

	svc, err := h.service.Get(r.Context(), envID, namespace, name)
	if err != nil {
		h.app.Logger.Error("failed to get k8s service", "namespace", namespace, "name", name, "error", err)
		response.NotFoundCode(w, r, i18n.ErrK8sServiceNotFound)
		return
	}

	response.OK(w, svc)
}

// HandleDelete deletes a Kubernetes service.
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
		h.app.Logger.Error("failed to delete k8s service", "namespace", namespace, "name", name, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrK8sServiceDeleteFailed)
		return
	}

	h.app.Logger.Info("k8s service deleted", "namespace", namespace, "name", name, "user", user.Username)
	response.NoContent(w)
}
