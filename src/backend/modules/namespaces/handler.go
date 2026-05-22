// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package namespaces

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for namespace HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new namespaces handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{app: app, service: NewService(app.KubernetesPool)}
}

// HandleList returns all namespaces.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := r.URL.Query().Get("env")

	nsList, err := h.service.List(r.Context(), envID)
	if err != nil {
		h.app.Logger.Error("failed to list namespaces", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrNamespaceListFailed)
		return
	}

	response.OK(w, nsList)
}

// HandleGet returns a single namespace.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := r.URL.Query().Get("env")
	name := chi.URLParam(r, "name")

	ns, err := h.service.Get(r.Context(), envID, name)
	if err != nil {
		h.app.Logger.Error("failed to get namespace", "name", name, "error", err)
		response.NotFoundCode(w, r, i18n.ErrNamespaceNotFound)
		return
	}

	response.OK(w, ns)
}
