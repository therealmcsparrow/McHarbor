// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pods

import (
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for pod HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new pods handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{app: app, service: NewService(app.KubernetesPool)}
}

// HandleList returns all pods.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := r.URL.Query().Get("env")
	namespace := r.URL.Query().Get("namespace")

	pods, err := h.service.List(r.Context(), envID, namespace)
	if err != nil {
		h.app.Logger.Error("failed to list pods", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrPodListFailed)
		return
	}

	response.OK(w, pods)
}

// HandleGet returns a single pod.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := r.URL.Query().Get("env")
	namespace := chi.URLParam(r, "namespace")
	name := chi.URLParam(r, "name")

	pod, err := h.service.Get(r.Context(), envID, namespace, name)
	if err != nil {
		h.app.Logger.Error("failed to get pod", "namespace", namespace, "name", name, "error", err)
		response.NotFoundCode(w, r, i18n.ErrPodNotFound)
		return
	}

	response.OK(w, pod)
}

// HandleDelete deletes a pod.
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
		h.app.Logger.Error("failed to delete pod", "namespace", namespace, "name", name, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrPodDeleteFailed)
		return
	}

	h.app.Logger.Info("pod deleted", "namespace", namespace, "name", name, "user", user.Username)
	response.NoContent(w)
}

// HandleLogs returns logs for a pod.
func (h *Handler) HandleLogs(w http.ResponseWriter, r *http.Request) {
	if auth.RequireAuth(r) == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	envID := r.URL.Query().Get("env")
	namespace := chi.URLParam(r, "namespace")
	name := chi.URLParam(r, "name")
	container := r.URL.Query().Get("container")

	var tail int64 = 100
	if t := r.URL.Query().Get("tail"); t != "" {
		if v, err := strconv.ParseInt(t, 10, 64); err == nil {
			tail = v
		}
	}

	stream, err := h.service.Logs(r.Context(), envID, namespace, name, container, tail)
	if err != nil {
		h.app.Logger.Error("failed to get pod logs", "namespace", namespace, "name", name, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrPodLogsFailed)
		return
	}
	defer stream.Close()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	io.Copy(w, stream)
}
