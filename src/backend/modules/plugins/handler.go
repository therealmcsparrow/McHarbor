// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Plugin represents a plugin record.
type Plugin struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Author      string `json:"author"`
	Source      string `json:"source"` // URL or local path
	Config      string `json:"config"` // JSON string of plugin config
	Enabled     bool   `json:"enabled"`
	InstalledAt string `json:"installedAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// InstallPluginInput is the request body for installing a plugin.
type InstallPluginInput struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Source  string `json:"source"`
	Config  string `json:"config"`
}

// UpdatePluginInput is the request body for updating plugin config.
type UpdatePluginInput struct {
	Config  *string `json:"config"`
	Version *string `json:"version"`
}

// Handler holds dependencies for plugin HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new plugins handler.
func NewHandler(app *router.AppDeps) *Handler {
	svc := NewService(app.DB)
	return &Handler{app: app, service: svc}
}

// HandleList returns a paginated list of plugins.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	page, perPage := response.ParsePagination(r)

	items, total, err := h.service.List(r.Context(), page, perPage)
	if err != nil {
		h.app.Logger.Error("plugins: list error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrPluginListFailed)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}

// HandleInstall installs a new plugin.
func (h *Handler) HandleInstall(w http.ResponseWriter, r *http.Request) {
	var input InstallPluginInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if input.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrPluginNameRequired)
		return
	}
	if input.Source == "" {
		response.BadRequestCode(w, r, i18n.ErrPluginSourceRequired)
		return
	}

	plugin, err := h.service.Install(r.Context(), input)
	if err != nil {
		h.app.Logger.Error("plugins: install error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrPluginInstallFailed)
		return
	}

	response.Created(w, plugin)
}

// HandleGet returns a single plugin.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	plugin, err := h.service.ByID(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("plugins: get error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrPluginListFailed)
		return
	}
	if plugin == nil {
		response.NotFoundCode(w, r, i18n.ErrPluginNotFound)
		return
	}

	response.OK(w, plugin)
}

// HandleUpdateConfig updates a plugin's config.
func (h *Handler) HandleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input UpdatePluginInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	plugin, err := h.service.Update(r.Context(), id, input)
	if err != nil {
		h.app.Logger.Error("plugins: update error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrPluginListFailed)
		return
	}
	if plugin == nil {
		response.NotFoundCode(w, r, i18n.ErrPluginNotFound)
		return
	}

	response.OK(w, plugin)
}

// HandleUninstall removes a plugin.
func (h *Handler) HandleUninstall(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	deleted, err := h.service.Uninstall(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("plugins: uninstall error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrPluginRemoveFailed)
		return
	}
	if !deleted {
		response.NotFoundCode(w, r, i18n.ErrPluginNotFound)
		return
	}

	response.NoContent(w)
}

// HandleToggle enables or disables a plugin.
func (h *Handler) HandleToggle(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	newEnabled, err := h.service.Toggle(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		response.NotFoundCode(w, r, i18n.ErrPluginNotFound)
		return
	}
	if err != nil {
		h.app.Logger.Error("plugins: toggle error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrPluginListFailed)
		return
	}

	response.OK(w, map[string]any{
		"id":      id,
		"enabled": newEnabled,
	})
}
