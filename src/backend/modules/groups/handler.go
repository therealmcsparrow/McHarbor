// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package groups manages user groups, their members and their role assignments.
package groups

import (
	"context"
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
	items, err := h.svc.List(r.Context())
	if err != nil {
		h.app.Logger.Error("groups: list error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrGroupListFailed, err)
		return
	}

	response.OK(w, items)
}

// HandleGet returns a group with members and role assignments.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	group, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("groups: get error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
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
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if input.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrGroupNameRequired)
		return
	}

	exists, err := h.svc.NameExists(r.Context(), input.Name, "")
	if err != nil {
		h.app.Logger.Error("groups: name check error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if exists {
		response.ConflictCode(w, r, i18n.ErrGroupNameTaken)
		return
	}

	group, err := h.svc.Create(r.Context(), &input)
	if err != nil {
		h.app.Logger.Error("groups: create error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrGroupCreateFailed, err)
		return
	}

	entry := audit.Entry{
		Action:     "create",
		EntityType: "group",
		EntityID:   group.ID,
		EntityName: input.Name,
	}
	if diffEntry, diffErr := entry.WithDiff(nil, group); diffErr == nil {
		entry = diffEntry
	} else {
		h.app.Logger.Error("groups: audit diff error", "error", diffErr, "id", group.ID)
	}
	h.app.AuditLog.Log(r, entry)

	response.Created(w, group)
}

// HandleUpdate updates an existing group.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	isSys, err := h.svc.IsSystem(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("groups: system check error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if isSys {
		response.ForbiddenCode(w, r, i18n.ErrGroupSystemLocked)
		return
	}

	var input UpdateGroupInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if input.Name != nil && *input.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrGroupNameRequired)
		return
	}

	if input.Name != nil {
		exists, err := h.svc.NameExists(r.Context(), *input.Name, id)
		if err != nil {
			h.app.Logger.Error("groups: name check error", "error", err)
			response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
			return
		}
		if exists {
			response.ConflictCode(w, r, i18n.ErrGroupNameTaken)
			return
		}
	}

	existing, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("groups: get before update error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if existing == nil {
		response.NotFoundCode(w, r, i18n.ErrGroupNotFound)
		return
	}

	group, err := h.svc.Update(r.Context(), id, &input)
	if err != nil {
		h.app.Logger.Error("groups: update error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrGroupUpdateFailed, err)
		return
	}
	if group == nil {
		response.NotFoundCode(w, r, i18n.ErrGroupNotFound)
		return
	}

	entry := audit.Entry{
		Action:     "update",
		EntityType: "group",
		EntityID:   id,
		EntityName: group.Name,
	}
	if diffEntry, diffErr := entry.WithDiff(existing, group); diffErr == nil {
		entry = diffEntry
	} else {
		h.app.Logger.Error("groups: audit diff error", "error", diffErr, "id", id)
	}
	h.app.AuditLog.Log(r, entry)

	response.OK(w, group)
}

// HandleDelete removes a group.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	isSys, err := h.svc.IsSystem(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("groups: system check error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if isSys {
		response.ForbiddenCode(w, r, i18n.ErrGroupSystemLocked)
		return
	}

	existing, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("groups: get error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if existing == nil {
		response.NotFoundCode(w, r, i18n.ErrGroupNotFound)
		return
	}

	if err := h.svc.Delete(r.Context(), id); err != nil {
		h.app.Logger.Error("groups: delete error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrGroupRemoveFailed, err)
		return
	}

	h.rbac.InvalidateCache("")

	entry := audit.Entry{
		Action:     "delete",
		EntityType: "group",
		EntityID:   id,
		EntityName: existing.Name,
	}
	if diffEntry, diffErr := entry.WithDiff(existing, nil); diffErr == nil {
		entry = diffEntry
	} else {
		h.app.Logger.Error("groups: audit diff error", "error", diffErr, "id", id)
	}
	h.app.AuditLog.Log(r, entry)

	response.NoContent(w)
}

// auditGroupName resolves a group id to its name for the audit trail, empty
// when the lookup fails: a row naming a since-deleted group only by id is
// unreadable, but it is still worth writing.
func (h *Handler) auditGroupName(ctx context.Context, id string) string {
	g, err := h.svc.Get(ctx, id)
	if err != nil || g == nil {
		return ""
	}
	return g.Name
}

// HandleAddMember adds a user to a group.
func (h *Handler) HandleAddMember(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "id")

	var input AddMemberInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if input.UserID == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	exists, err := h.svc.MemberExists(r.Context(), groupID, input.UserID)
	if err != nil {
		h.app.Logger.Error("groups: member check error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if exists {
		response.ConflictCode(w, r, i18n.ErrGroupMemberExists)
		return
	}

	if err := h.svc.AddMember(r.Context(), groupID, input.UserID); err != nil {
		h.app.Logger.Error("groups: add member error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	h.rbac.InvalidateCache(input.UserID)

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "add_member",
		EntityType: "group",
		EntityID:   groupID,
		EntityName: h.auditGroupName(r.Context(), groupID),
		Details:    "user " + input.UserID + " added",
	})

	response.OKMsg(w, r, i18n.MsgGroupMemberAdded)
}

// HandleRemoveMember removes a user from a group.
func (h *Handler) HandleRemoveMember(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "id")
	userID := chi.URLParam(r, "userId")

	if err := h.svc.RemoveMember(r.Context(), groupID, userID); err != nil {
		h.app.Logger.Error("groups: remove member error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	h.rbac.InvalidateCache(userID)

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "remove_member",
		EntityType: "group",
		EntityID:   groupID,
		EntityName: h.auditGroupName(r.Context(), groupID),
		Details:    "user " + userID + " removed",
	})

	response.OKMsg(w, r, i18n.MsgGroupMemberRemoved)
}

// HandleAssignRole assigns a role to a group.
func (h *Handler) HandleAssignRole(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "id")

	var input AssignRoleInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
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

	before, err := h.svc.Get(r.Context(), groupID)
	if err != nil {
		h.app.Logger.Error("groups: get before assign role error", "error", err, "id", groupID)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if before == nil {
		response.NotFoundCode(w, r, i18n.ErrGroupNotFound)
		return
	}

	if _, err := h.svc.AssignRole(r.Context(), groupID, input); err != nil {
		h.app.Logger.Error("groups: assign role error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	h.rbac.InvalidateCache("")

	after, err := h.svc.Get(r.Context(), groupID)
	if err != nil {
		h.app.Logger.Error("groups: get after assign role error", "error", err, "id", groupID)
		after = before
	}
	entry := audit.Entry{
		Action:     "assign_role",
		EntityType: "group",
		EntityID:   groupID,
		EntityName: before.Name,
		Details:    "role " + input.RoleID + " assigned",
	}
	if diffEntry, diffErr := entry.WithDiff(before, after); diffErr == nil {
		entry = diffEntry
	} else {
		h.app.Logger.Error("groups: audit diff error", "error", diffErr, "id", groupID)
	}
	h.app.AuditLog.Log(r, entry)

	response.OKMsg(w, r, i18n.MsgGroupRoleAssigned)
}

// HandleUnassignRole unassigns a role from a group.
func (h *Handler) HandleUnassignRole(w http.ResponseWriter, r *http.Request) {
	assignmentID := chi.URLParam(r, "assignmentId")
	groupID := chi.URLParam(r, "id")
	before, err := h.svc.Get(r.Context(), groupID)
	if err != nil {
		h.app.Logger.Error("groups: get before unassign role error", "error", err, "id", groupID)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if before == nil {
		response.NotFoundCode(w, r, i18n.ErrGroupNotFound)
		return
	}

	if err := h.svc.UnassignRole(r.Context(), assignmentID); err != nil {
		h.app.Logger.Error("groups: unassign role error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	h.rbac.InvalidateCache("")

	after, err := h.svc.Get(r.Context(), groupID)
	if err != nil {
		h.app.Logger.Error("groups: get after unassign role error", "error", err, "id", groupID)
		after = before
	}
	entry := audit.Entry{
		Action:     "unassign_role",
		EntityType: "group",
		EntityID:   groupID,
		EntityName: before.Name,
		Details:    "assignment " + assignmentID + " removed",
	}
	if diffEntry, diffErr := entry.WithDiff(before, after); diffErr == nil {
		entry = diffEntry
	} else {
		h.app.Logger.Error("groups: audit diff error", "error", diffErr, "id", groupID)
	}
	h.app.AuditLog.Log(r, entry)

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
