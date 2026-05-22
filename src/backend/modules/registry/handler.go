// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package registry

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Registry represents a container registry configuration.
type Registry struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	Username  string `json:"username"`
	Password  string `json:"password,omitempty"` // encrypted at rest; omitted in list responses
	IsDefault bool   `json:"isDefault"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// CreateRegistryInput is the request body for adding a registry.
type CreateRegistryInput struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// UpdateRegistryInput is the request body for updating a registry.
type UpdateRegistryInput struct {
	Name      *string `json:"name"`
	URL       *string `json:"url"`
	Username  *string `json:"username"`
	Password  *string `json:"password"`
	IsDefault *bool   `json:"isDefault"`
}

// Handler holds dependencies for registry HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new registry handler.
func NewHandler(app *router.AppDeps) *Handler {
	svc := NewService(app.DB, app.Encryption)
	return &Handler{app: app, service: svc}
}

// HandleList returns a paginated list of registries.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	page, perPage := response.ParsePagination(r)

	items, total, err := h.service.List(page, perPage)
	if err != nil {
		h.app.Logger.Error("registry: list error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrRegistryListFailed)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}

// HandleCreate adds a new registry.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var input CreateRegistryInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if input.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrRegistryNameRequired)
		return
	}
	if input.URL == "" {
		response.BadRequestCode(w, r, i18n.ErrRegistryUrlRequired)
		return
	}

	reg, err := h.service.Create(input)
	if err != nil {
		h.app.Logger.Error("registry: create error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrRegistryCreateFailed)
		return
	}

	response.Created(w, reg)
}

// HandleGet returns a single registry (password omitted).
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	reg, err := h.service.ByID(id)
	if err != nil {
		h.app.Logger.Error("registry: get error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrRegistryFailed)
		return
	}
	if reg == nil {
		response.NotFoundCode(w, r, i18n.ErrRegistryNotFound)
		return
	}

	response.OK(w, reg)
}

// HandleUpdate updates an existing registry.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input UpdateRegistryInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	reg, err := h.service.Update(id, input)
	if err != nil {
		h.app.Logger.Error("registry: update error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrRegistryUpdateFailed)
		return
	}
	if reg == nil {
		response.NotFoundCode(w, r, i18n.ErrRegistryNotFound)
		return
	}

	response.OK(w, reg)
}

// HandleDelete removes a registry.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	deleted, err := h.service.Delete(id)
	if err != nil {
		h.app.Logger.Error("registry: delete error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrRegistryRemoveFailed)
		return
	}
	if !deleted {
		response.NotFoundCode(w, r, i18n.ErrRegistryNotFound)
		return
	}

	response.NoContent(w)
}

// HandleTestConnection tests the registry connection.
func (h *Handler) HandleTestConnection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	result, err := h.service.TestConnection(id)
	if err != nil {
		h.app.Logger.Error("registry: test connection error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrRegistryFailed)
		return
	}
	if result == nil {
		response.NotFoundCode(w, r, i18n.ErrRegistryNotFound)
		return
	}

	response.OK(w, result)
}
