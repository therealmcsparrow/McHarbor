// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package appstore

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

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

	apps, total, err := h.service.List(category, search, page, perPage)
	if err != nil {
		h.app.Logger.Error("failed to list app store catalog", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
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
	app, err := h.service.BySlug(slug)
	if err != nil {
		h.app.Logger.Error("failed to get app", "slug", slug, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
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

	cats, err := h.service.Categories()
	if err != nil {
		h.app.Logger.Error("failed to get categories", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	response.OK(w, cats)
}

// HandleInstall installs an app from the catalog by creating a Stack.
func (h *Handler) HandleInstall(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	var req InstallRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if req.Slug == "" {
		response.BadRequestCode(w, r, i18n.ErrAppStoreSlugRequired)
		return
	}

	result, err := h.service.Install(r.Context(), req)
	if err != nil {
		h.app.Logger.Error("failed to install app", "slug", req.Slug, "error", err)
		response.BadRequestCode(w, r, i18n.ErrAppStoreInstallFailed)
		return
	}

	h.app.Logger.Info("app installed", "slug", req.Slug, "stack", result.StackName, "user", user.Username)
	response.Created(w, result)
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
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
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
			data, _ := json.Marshal(event) // safe: simple struct
			fmt.Fprintf(w, "data: %s\n\n", data)
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

// HandleSync triggers a remote catalog sync.
func (h *Handler) HandleSync(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	catalogURL := h.app.Config.AppStoreCatalogURL
	if err := h.service.SyncRemoteCatalog(catalogURL); err != nil {
		h.app.Logger.Error("failed to sync catalog", "error", err)
		response.BadRequestCode(w, r, i18n.ErrAppStoreInstallFailed)
		return
	}

	h.app.Logger.Info("catalog sync triggered", "user", user.Username)
	response.OK(w, map[string]string{"status": "syncing"})
}

// HandleSyncStatus returns the latest sync status.
func (h *Handler) HandleSyncStatus(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	status, err := h.service.SyncStatus()
	if err != nil {
		h.app.Logger.Error("failed to get sync status", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	response.OK(w, status)
}

// HandleInstalled returns all installed apps with stack status.
func (h *Handler) HandleInstalled(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	apps, err := h.service.InstalledApps()
	if err != nil {
		h.app.Logger.Error("failed to list installed apps", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	response.OK(w, apps)
}
