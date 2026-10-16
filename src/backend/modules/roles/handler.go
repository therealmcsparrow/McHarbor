// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package roles serves role CRUD and the role-preset list behind the RBAC role editor.
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
	items, err := h.svc.List(r.Context())
	if err != nil {
		h.app.Logger.Error("roles: list error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrRoleListFailed, err)
		return
	}

	response.OK(w, items)
}

// HandleGet returns a single role.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	role, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("roles: get error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
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
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if input.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrRoleNameRequired)
		return
	}

	exists, err := h.svc.NameExists(r.Context(), input.Name, "")
	if err != nil {
		h.app.Logger.Error("roles: name check error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if exists {
		response.ConflictCode(w, r, i18n.ErrRoleNameTaken)
		return
	}

	if input.Permissions == nil {
		input.Permissions = []string{}
	}

	role, err := h.svc.Create(r.Context(), &input)
	if err != nil {
		h.app.Logger.Error("roles: create error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrRoleCreateFailed, err)
		return
	}

	h.rbac.InvalidateCache("")

	entry := audit.Entry{
		Action:     "create",
		EntityType: "role",
		EntityID:   role.ID,
		EntityName: input.Name,
	}
	if diffEntry, diffErr := entry.WithDiff(nil, role); diffErr == nil {
		entry = diffEntry
	} else {
		h.app.Logger.Error("roles: audit diff error", "error", diffErr, "id", role.ID)
	}
	h.app.AuditLog.Log(r, entry)

	response.Created(w, role)
}

// HandleUpdate updates an existing custom role.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	isSystem, err := h.svc.IsSystem(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("roles: system check error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if isSystem {
		response.ForbiddenCode(w, r, i18n.ErrRoleSystemLocked)
		return
	}

	var input UpdateRoleInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if input.Name != nil && *input.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrRoleNameRequired)
		return
	}

	if input.Name != nil {
		exists, err := h.svc.NameExists(r.Context(), *input.Name, id)
		if err != nil {
			h.app.Logger.Error("roles: name check error", "error", err)
			response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
			return
		}
		if exists {
			response.ConflictCode(w, r, i18n.ErrRoleNameTaken)
			return
		}
	}

	existing, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("roles: get before update error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if existing == nil {
		response.NotFoundCode(w, r, i18n.ErrRoleNotFound)
		return
	}

	role, err := h.svc.Update(r.Context(), id, &input)
	if err != nil {
		h.app.Logger.Error("roles: update error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrRoleUpdateFailed, err)
		return
	}
	if role == nil {
		response.NotFoundCode(w, r, i18n.ErrRoleNotFound)
		return
	}

	h.rbac.InvalidateCache("")

	entry := audit.Entry{
		Action:     "update",
		EntityType: "role",
		EntityID:   id,
		EntityName: role.Name,
	}
	if diffEntry, diffErr := entry.WithDiff(existing, role); diffErr == nil {
		entry = diffEntry
	} else {
		h.app.Logger.Error("roles: audit diff error", "error", diffErr, "id", id)
	}
	h.app.AuditLog.Log(r, entry)

	response.OK(w, role)
}

// HandleDelete removes a custom role.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	isSystem, err := h.svc.IsSystem(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("roles: system check error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if isSystem {
		response.ForbiddenCode(w, r, i18n.ErrRoleSystemLocked)
		return
	}

	existing, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("roles: get error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if existing == nil {
		response.NotFoundCode(w, r, i18n.ErrRoleNotFound)
		return
	}

	if err := h.svc.Delete(r.Context(), id); err != nil {
		h.app.Logger.Error("roles: delete error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrRoleRemoveFailed, err)
		return
	}

	h.rbac.InvalidateCache("")

	entry := audit.Entry{
		Action:     "delete",
		EntityType: "role",
		EntityID:   id,
		EntityName: existing.Name,
	}
	if diffEntry, diffErr := entry.WithDiff(existing, nil); diffErr == nil {
		entry = diffEntry
	} else {
		h.app.Logger.Error("roles: audit diff error", "error", diffErr, "id", id)
	}
	h.app.AuditLog.Log(r, entry)

	response.NoContent(w)
}

// HandleListPermissions returns all available permissions.
func (h *Handler) HandleListPermissions(w http.ResponseWriter, _ *http.Request) {
	all := rbac.AllPermissions()
	perms := make([]string, len(all))
	for i, p := range all {
		perms[i] = string(p)
	}
	response.OK(w, perms)
}

// ImpactInput is the body for POST /api/roles/{id}/impact. Phase 7
// of the RBAC roadmap.
type ImpactInput struct {
	PermissionKeys []string `json:"permissionKeys"`
	EnvironmentID  *string  `json:"environmentId,omitempty"`
}

// ImpactUser is one row of the affected-users list.
type ImpactUser struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName *string `json:"displayName,omitempty"`
	Email       *string `json:"email,omitempty"`
	IsActive    bool    `json:"isActive"`
	HasRole     bool    `json:"hasRole"`
}

// ImpactResult is the response body for POST /api/roles/{id}/impact.
type ImpactResult struct {
	RoleID        string       `json:"roleId"`
	GrantedCount  int          `json:"grantedCount"`
	RevokedCount  int          `json:"revokedCount"`
	AffectedUsers []ImpactUser `json:"affectedUsers"`
}

// HandleImpact computes the user impact of setting a role's
// permission list to the supplied set. Phase 7 of the RBAC roadmap.
// The endpoint is gated by users.manage at the route level.
func (h *Handler) HandleImpact(w http.ResponseWriter, r *http.Request) {
	roleID := chi.URLParam(r, "id")

	var in ImpactInput
	if err := response.DecodeBody(r, &in); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if len(in.PermissionKeys) == 0 {
		in.PermissionKeys = []string{}
	}

	// Compare each user against the candidate set. A user "hasRole"
	// today iff their effective permission set contains the entire
	// candidate set (set inclusion). After the change, the same
	// predicate is checked against the candidate set. The diff is
	// granted - revoked.
	users, err := h.svc.ActiveUsers(r.Context(), 200)
	if err != nil {
		h.app.Logger.Error("roles: impact query", "error", err, "role", roleID)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	candidate := make(map[string]struct{}, len(in.PermissionKeys))
	for _, p := range in.PermissionKeys {
		candidate[p] = struct{}{}
	}

	out := ImpactResult{RoleID: roleID, AffectedUsers: []ImpactUser{}}
	granted := 0
	revoked := 0
	for _, u := range users {
		// Read the user's current effective permission set.
		perms, err := h.rbac.EffectivePermissions(r.Context(), u.ID, derefStr(in.EnvironmentID))
		if err != nil {
			// Skip on RBAC error; don't fail the whole endpoint.
			continue
		}
		hasAll := true
		for p := range candidate {
			if !hasPerm(perms, p) {
				hasAll = false
				break
			}
		}
		// If the candidate is empty, "granted" = "lost the role entirely".
		isEmpty := len(candidate) == 0
		currentlyHas := hasAll && !isEmpty
		out.AffectedUsers = append(out.AffectedUsers, ImpactUser{
			ID: u.ID, Username: u.Username, DisplayName: u.DisplayName, Email: u.Email,
			IsActive: u.IsActive, HasRole: currentlyHas,
		})
		if currentlyHas {
			// Would lose the role if we save the empty list, OR the
			// role's new perm set is a strict subset of what they have.
			// For the preview we count "revoked" when currentlyHas is
			// true and the candidate doesn't fully cover them — i.e.
			// they'd lose something.
			if !hasAll {
				revoked++
			}
		} else if !isEmpty && hasAll {
			granted++
		}
	}

	out.GrantedCount = granted
	out.RevokedCount = revoked
	response.OK(w, out)
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func hasPerm(perms []rbac.Permission, key string) bool {
	for _, p := range perms {
		if string(p) == key || string(p) == "*" {
			return true
		}
	}
	return false
}
