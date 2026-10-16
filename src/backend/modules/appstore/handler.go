// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package appstore

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for app store HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new app store handler.
func NewHandler(app *router.AppDeps, service *Service) *Handler {
	return &Handler{app: app, service: service}
}

// HandleList returns paginated catalog items with optional category/search filter.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	category := r.URL.Query().Get("category")
	search := r.URL.Query().Get("search")
	page, perPage := response.ParsePagination(r)

	apps, total, err := h.service.List(r.Context(), category, search, page, perPage)
	if err != nil {
		h.app.Logger.Error("failed to list app store catalog", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	response.Paginated(w, apps, total, page, perPage)
}

// HandleGet returns a single catalog item by slug.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	slug := chi.URLParam(r, "slug")
	app, err := h.service.BySlug(r.Context(), slug)
	if err != nil {
		h.app.Logger.Error("failed to get app", "slug", slug, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if app == nil {
		response.NotFoundCode(w, r, i18n.ErrAppStoreNotFound)
		return
	}

	response.OK(w, app)
}

// HandleCategories returns category names with counts.
func (h *Handler) HandleCategories(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	cats, err := h.service.Categories(r.Context())
	if err != nil {
		h.app.Logger.Error("failed to get categories", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	response.OK(w, cats)
}

// HandleInstallStream streams install progress via Server-Sent Events.
func (h *Handler) HandleInstallStream(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	var req InstallRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if req.Slug == "" {
		response.BadRequestCode(w, r, i18n.ErrAppStoreSlugRequired)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	events := make(chan InstallEvent)
	go h.service.InstallWithProgress(r.Context(), req, events)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			h.writeSSE(w, event)
			flusher.Flush()

			if event.Status == "done" {
				h.app.Logger.Info("app installed (streamed)", "slug", req.Slug, "stack", event.StackName, "user", user.Username)
			}
			if event.Status == "done" || event.Status == "error" {
				return
			}
		}
	}
}

// HandleSync reloads enabled Docker application catalogs.
func (h *Handler) HandleSync(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	if err := h.service.SyncCatalogs(r.Context()); err != nil {
		h.app.Logger.Error("failed to sync catalog", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	h.app.Logger.Info("catalog sync triggered", "user", user.Username)
	response.OK(w, map[string]string{"status": "synced"})
}

// HandleSyncStatus returns the latest sync status.
func (h *Handler) HandleSyncStatus(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	status, err := h.service.SyncStatus(r.Context())
	if err != nil {
		h.app.Logger.Error("failed to get sync status", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	response.OK(w, status)
}

// HandleUninstallStream streams uninstall progress for one app installation.
func (h *Handler) HandleUninstallStream(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	events := make(chan InstallEvent)
	go h.service.UninstallWithProgress(r.Context(), id, events)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			h.writeSSE(w, event)
			flusher.Flush()

			if event.Status == "done" {
				h.app.AuditLog.Log(r, audit.Entry{
					Action:     "uninstall",
					EntityType: "app",
					EntityID:   id,
					EntityName: event.StackName,
				})
				h.app.Logger.Info("app uninstalled (streamed)", "installation", id, "stack", event.StackName, "user", user.Username)
			}
			if event.Status == "done" || event.Status == "error" {
				return
			}
		}
	}
}

// writeSSE writes one server-sent event and only logs a failure: the install or uninstall keeps running and must still be audited when the client has gone.
func (h *Handler) writeSSE(w http.ResponseWriter, event any) {
	data, err := json.Marshal(event)
	if err != nil {
		h.app.Logger.Error("app store: encoding stream event failed", "error", err)
		return
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		h.app.Logger.Debug("app store: stream client went away", "error", err)
	}
}
