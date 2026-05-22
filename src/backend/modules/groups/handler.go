// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package groups

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for group HTTP handlers.
type Handler struct {
	svc  *Service
	rbac *rbac.Service
	app  *router.AppDeps
}

// NewHandler creates a new groups handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{
		svc:  NewService(app.DB),
		rbac: app.RBACService,
		app:  app,
	}
}

// HandleList returns all groups with member counts.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List()
	if err != nil {
		h.app.Logger.Error("groups: list error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrGroupListFailed)
		return
	}

	response.OK(w, items)
}

// HandleGet returns a group with members and role assignments.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	group, err := h.svc.Get(id)
	if err != nil {
		h.app.Logger.Error("groups: get error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if group == nil {
		response.NotFoundCode(w, r, i18n.ErrGroupNotFound)
		return
	}

	response.OK(w, group)
}

// HandleCreate creates a new group.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var input CreateGroupInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if input.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrGroupNameRequired)
		return
	}

	exists, err := h.svc.NameExists(input.Name, "")
	if err != nil {
		h.app.Logger.Error("groups: name check error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if exists {
		response.ConflictCode(w, r, i18n.ErrGroupNameTaken)
		return
	}

	group, err := h.svc.Create(&input)
	if err != nil {
		h.app.Logger.Error("groups: create error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrGroupCreateFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "create",
		EntityType: "group",
		EntityID:   group.ID,
		EntityName: input.Name,
	})

	response.Created(w, group)
}

// HandleUpdate updates an existing group.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	isSys, err := h.svc.IsSystem(id)
	if err != nil {
		h.app.Logger.Error("groups: system check error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if isSys {
		response.ForbiddenCode(w, r, i18n.ErrGroupSystemLocked)
		return
	}

	var input UpdateGroupInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if input.Name != nil && *input.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrGroupNameRequired)
		return
	}

	if input.Name != nil {
		exists, err := h.svc.NameExists(*input.Name, id)
		if err != nil {
			h.app.Logger.Error("groups: name check error", "error", err)
			response.InternalErrorCode(w, r, i18n.ErrInternalServer)
			return
		}
		if exists {
			response.ConflictCode(w, r, i18n.ErrGroupNameTaken)
			return
		}
	}

	group, err := h.svc.Update(id, &input)
	if err != nil {
		h.app.Logger.Error("groups: update error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrGroupUpdateFailed)
		return
	}
	if group == nil {
		response.NotFoundCode(w, r, i18n.ErrGroupNotFound)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "update",
		EntityType: "group",
		EntityID:   id,
		EntityName: group.Name,
	})

	response.OK(w, group)
}

// HandleDelete removes a group.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	isSys, err := h.svc.IsSystem(id)
	if err != nil {
		h.app.Logger.Error("groups: system check error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if isSys {
		response.ForbiddenCode(w, r, i18n.ErrGroupSystemLocked)
		return
	}

	existing, err := h.svc.Get(id)
	if err != nil {
		h.app.Logger.Error("groups: get error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if existing == nil {
		response.NotFoundCode(w, r, i18n.ErrGroupNotFound)
		return
	}

	if err := h.svc.Delete(id); err != nil {
		h.app.Logger.Error("groups: delete error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrGroupRemoveFailed)
		return
	}

	h.rbac.InvalidateCache("")

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "delete",
		EntityType: "group",
		EntityID:   id,
		EntityName: existing.Name,
	})

	response.NoContent(w)
}

// HandleAddMember adds a user to a group.
func (h *Handler) HandleAddMember(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "id")

	var input AddMemberInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if input.UserID == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	exists, err := h.svc.MemberExists(groupID, input.UserID)
	if err != nil {
		h.app.Logger.Error("groups: member check error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if exists {
		response.ConflictCode(w, r, i18n.ErrGroupMemberExists)
		return
	}

	if err := h.svc.AddMember(groupID, input.UserID); err != nil {
		h.app.Logger.Error("groups: add member error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	h.rbac.InvalidateCache(input.UserID)

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "add_member",
		EntityType: "group",
		EntityID:   groupID,
		Details:    "user " + input.UserID + " added",
	})

	response.OKMsg(w, r, i18n.MsgGroupMemberAdded)
}

// HandleRemoveMember removes a user from a group.
func (h *Handler) HandleRemoveMember(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "id")
	userID := chi.URLParam(r, "userId")

	if err := h.svc.RemoveMember(groupID, userID); err != nil {
		h.app.Logger.Error("groups: remove member error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	h.rbac.InvalidateCache(userID)

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "remove_member",
		EntityType: "group",
		EntityID:   groupID,
		Details:    "user " + userID + " removed",
	})

	response.OKMsg(w, r, i18n.MsgGroupMemberRemoved)
}

// HandleAssignRole assigns a role to a group.
func (h *Handler) HandleAssignRole(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "id")

	var input AssignRoleInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if input.RoleID == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	input.EnvironmentID = normalizeOptionalString(input.EnvironmentID)
	input.StackName = normalizeOptionalString(input.StackName)
	if input.StackName != nil && input.EnvironmentID == nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if _, err := h.svc.AssignRole(groupID, input); err != nil {
		h.app.Logger.Error("groups: assign role error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	h.rbac.InvalidateCache("")

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "assign_role",
		EntityType: "group",
		EntityID:   groupID,
		Details:    "role " + input.RoleID + " assigned",
	})

	response.OKMsg(w, r, i18n.MsgGroupRoleAssigned)
}

// HandleUnassignRole unassigns a role from a group.
func (h *Handler) HandleUnassignRole(w http.ResponseWriter, r *http.Request) {
	assignmentID := chi.URLParam(r, "assignmentId")

	if err := h.svc.UnassignRole(assignmentID); err != nil {
		h.app.Logger.Error("groups: unassign role error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	h.rbac.InvalidateCache("")

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "unassign_role",
		EntityType: "group",
		EntityID:   chi.URLParam(r, "id"),
		Details:    "assignment " + assignmentID + " removed",
	})

	response.OKMsg(w, r, i18n.MsgGroupRoleUnassigned)
}

func normalizeOptionalString(value *string) *string {
	if value == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}
