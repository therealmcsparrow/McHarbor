// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package roles

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for role HTTP handlers.
type Handler struct {
	svc  *Service
	rbac *rbac.Service
	app  *router.AppDeps
}

// NewHandler creates a new roles handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{
		svc:  NewService(app.DB),
		rbac: app.RBACService,
		app:  app,
	}
}

// HandleList returns all roles.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List()
	if err != nil {
		h.app.Logger.Error("roles: list error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrRoleListFailed)
		return
	}

	response.OK(w, items)
}

// HandleGet returns a single role.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	role, err := h.svc.Get(id)
	if err != nil {
		h.app.Logger.Error("roles: get error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if role == nil {
		response.NotFoundCode(w, r, i18n.ErrRoleNotFound)
		return
	}

	response.OK(w, role)
}

// HandleCreate creates a new custom role.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var input CreateRoleInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if input.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrRoleNameRequired)
		return
	}

	exists, err := h.svc.NameExists(input.Name, "")
	if err != nil {
		h.app.Logger.Error("roles: name check error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if exists {
		response.ConflictCode(w, r, i18n.ErrRoleNameTaken)
		return
	}

	if input.Permissions == nil {
		input.Permissions = []string{}
	}

	role, err := h.svc.Create(&input)
	if err != nil {
		h.app.Logger.Error("roles: create error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrRoleCreateFailed)
		return
	}

	h.rbac.InvalidateCache("")

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "create",
		EntityType: "role",
		EntityID:   role.ID,
		EntityName: input.Name,
	})

	response.Created(w, role)
}

// HandleUpdate updates an existing custom role.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	isSystem, err := h.svc.IsSystem(id)
	if err != nil {
		h.app.Logger.Error("roles: system check error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if isSystem {
		response.ForbiddenCode(w, r, i18n.ErrRoleSystemLocked)
		return
	}

	var input UpdateRoleInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if input.Name != nil && *input.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrRoleNameRequired)
		return
	}

	if input.Name != nil {
		exists, err := h.svc.NameExists(*input.Name, id)
		if err != nil {
			h.app.Logger.Error("roles: name check error", "error", err)
			response.InternalErrorCode(w, r, i18n.ErrInternalServer)
			return
		}
		if exists {
			response.ConflictCode(w, r, i18n.ErrRoleNameTaken)
			return
		}
	}

	role, err := h.svc.Update(id, &input)
	if err != nil {
		h.app.Logger.Error("roles: update error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrRoleUpdateFailed)
		return
	}
	if role == nil {
		response.NotFoundCode(w, r, i18n.ErrRoleNotFound)
		return
	}

	h.rbac.InvalidateCache("")

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "update",
		EntityType: "role",
		EntityID:   id,
		EntityName: role.Name,
	})

	response.OK(w, role)
}

// HandleDelete removes a custom role.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	isSystem, err := h.svc.IsSystem(id)
	if err != nil {
		h.app.Logger.Error("roles: system check error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if isSystem {
		response.ForbiddenCode(w, r, i18n.ErrRoleSystemLocked)
		return
	}

	existing, err := h.svc.Get(id)
	if err != nil {
		h.app.Logger.Error("roles: get error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if existing == nil {
		response.NotFoundCode(w, r, i18n.ErrRoleNotFound)
		return
	}

	if err := h.svc.Delete(id); err != nil {
		h.app.Logger.Error("roles: delete error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrRoleRemoveFailed)
		return
	}

	h.rbac.InvalidateCache("")

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "delete",
		EntityType: "role",
		EntityID:   id,
		EntityName: existing.Name,
	})

	response.NoContent(w)
}

// HandleListPermissions returns all available permissions.
func (h *Handler) HandleListPermissions(w http.ResponseWriter, _ *http.Request) {
	perms := make([]string, len(rbac.AllPermissions))
	for i, p := range rbac.AllPermissions {
		perms[i] = string(p)
	}
	response.OK(w, perms)
}
