// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package blueprints

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for blueprint HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new blueprints handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{
		app:     app,
		service: NewService(app.DB),
	}
}

// HandleList returns a paginated list of blueprints.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	page, perPage := response.ParsePagination(r)

	items, total, err := h.service.List(page, perPage)
	if err != nil {
		h.app.Logger.Error("blueprints: list error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}

// HandleGet returns a single blueprint by ID.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	bp, err := h.service.ByID(id)
	if err != nil {
		h.app.Logger.Error("blueprints: get error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if bp == nil {
		response.NotFoundCode(w, r, i18n.ErrBlueprintNotFound)
		return
	}

	response.OK(w, bp)
}

// HandleCreate creates a new blueprint.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var input CreateBlueprintInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if input.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrBlueprintNameRequired)
		return
	}
	if input.ComposeYAML == "" {
		response.BadRequestCode(w, r, i18n.ErrBlueprintComposeRequired)
		return
	}

	bp, err := h.service.Create(input)
	if err != nil {
		h.app.Logger.Error("blueprints: create error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	response.Created(w, bp)
}

// HandleUpdate updates an existing blueprint.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input UpdateBlueprintInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	bp, err := h.service.Update(id, input)
	if err != nil {
		h.app.Logger.Error("blueprints: update error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if bp == nil {
		response.NotFoundCode(w, r, i18n.ErrBlueprintNotFound)
		return
	}

	response.OK(w, bp)
}

// HandleDelete removes a blueprint.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.service.Delete(id); err != nil {
		h.app.Logger.Error("blueprints: delete error", "error", err, "id", id)
		response.NotFoundCode(w, r, i18n.ErrBlueprintNotFound)
		return
	}

	response.NoContent(w)
}

// HandleDeploy deploys a blueprint as a new stack.
func (h *Handler) HandleDeploy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	bp, err := h.service.ByID(id)
	if err != nil {
		h.app.Logger.Error("blueprints: deploy lookup error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if bp == nil {
		response.NotFoundCode(w, r, i18n.ErrBlueprintNotFound)
		return
	}

	var input DeployInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if input.StackName == "" {
		response.BadRequestCode(w, r, i18n.ErrBlueprintStackNameReq)
		return
	}

	// Stub: stacks module integration for deploying compose YAML is not yet implemented.
	// Return the blueprint data that would be deployed as a placeholder.
	response.OK(w, map[string]any{
		"blueprint": bp,
		"stackName": input.StackName,
		"envId":     input.EnvID,
		"status":    "pending",
		"message":   "Blueprint deploy queued",
	})
}
