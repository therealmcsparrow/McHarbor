// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package users

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// User represents a user record returned from the API.
type User struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName *string `json:"displayName"`
	Email       *string `json:"email"`
	Role        string  `json:"role"`
	IsActive    bool    `json:"isActive"`
	LastLogin   *string `json:"lastLogin"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   string  `json:"updatedAt"`
}

// UpdateUserInput is the request body for updating a user.
type UpdateUserInput struct {
	DisplayName *string `json:"displayName"`
	Email       *string `json:"email"`
	Role        *string `json:"role"`
	IsActive    *bool   `json:"isActive"`
}

// CreateUserInput is the request body for creating a local user.
type CreateUserInput struct {
	Username    string  `json:"username"`
	Password    string  `json:"password"`
	DisplayName *string `json:"displayName"`
	Email       *string `json:"email"`
	RoleID      *string `json:"roleId"`
	IsActive    *bool   `json:"isActive"`
}

// ChangePasswordInput is the request body for changing a user's password.
type ChangePasswordInput struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

// UserGroup represents a group membership for a user.
type UserGroup struct {
	ID          string `json:"id"`
	GroupID     string `json:"groupId"`
	GroupName   string `json:"groupName"`
	MemberCount int    `json:"memberCount"`
	IsSystem    bool   `json:"isSystem"`
}

// UserRole represents a role assignment for a user.
type UserRole struct {
	ID              string  `json:"id"`
	RoleID          string  `json:"roleId"`
	RoleName        string  `json:"roleName"`
	EnvironmentID   *string `json:"environmentId"`
	EnvironmentName *string `json:"environmentName"`
	StackName       *string `json:"stackName"`
}

// AssignRoleInput is the request body for assigning a role.
type AssignRoleInput struct {
	RoleID        string  `json:"roleId"`
	EnvironmentID *string `json:"environmentId"`
	StackName     *string `json:"stackName"`
}

// Handler holds dependencies for user HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new users handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{
		app:     app,
		service: NewService(app.DB, app.RBACService),
	}
}

// HandleList returns a paginated list of users.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	page, perPage := response.ParsePagination(r)

	items, total, err := h.service.List(page, perPage)
	if err != nil {
		h.app.Logger.Error("users: list error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	response.Paginated(w, items, total, page, perPage)
}

// HandleCreate creates a local user.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	currentUser := auth.RequireAuth(r)
	if currentUser == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	var input CreateUserInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	input.Username = strings.TrimSpace(input.Username)
	input.DisplayName = normalizeOptionalString(input.DisplayName)
	input.Email = normalizeOptionalString(input.Email)
	input.RoleID = normalizeOptionalString(input.RoleID)

	if input.Username == "" || input.Password == "" {
		response.BadRequestCode(w, r, i18n.ErrAuthUsernameRequired)
		return
	}
	if len(input.Password) < 8 {
		response.BadRequestCode(w, r, i18n.ErrAuthPasswordShort)
		return
	}

	u, err := h.service.Create(input)
	if errors.Is(err, ErrUserExists) {
		response.ConflictCode(w, r, i18n.ErrAuthUsernameTaken)
		return
	}
	if errors.Is(err, ErrRoleNotFound) {
		response.BadRequestCode(w, r, i18n.ErrRoleNotFound)
		return
	}
	if err != nil {
		h.app.Logger.Error("users: create error", "error", err, "username", input.Username)
		response.InternalErrorCode(w, r, i18n.ErrUserCreateFailed)
		return
	}

	h.app.AuditLog.LogWithUser(r, currentUser.ID, currentUser.Username, audit.Entry{
		Action:     "create",
		EntityType: "user",
		EntityID:   u.ID,
		EntityName: u.Username,
	})

	response.Created(w, u)
}

// HandleGet returns a single user.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	u, err := h.service.ByID(id)
	if err != nil {
		h.app.Logger.Error("users: get error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if u == nil {
		response.NotFoundCode(w, r, i18n.ErrUserNotFound)
		return
	}

	response.OK(w, u)
}

// HandleUpdate updates an existing user.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input UpdateUserInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	u, err := h.service.Update(id, input)
	if errors.Is(err, ErrUserNotFound) {
		response.NotFoundCode(w, r, i18n.ErrUserNotFound)
		return
	}
	if err != nil {
		h.app.Logger.Error("users: update error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	response.OK(w, u)
}

// HandleDelete removes a user.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Prevent deleting yourself
	currentUser := auth.UserFromContext(r.Context())
	if currentUser != nil && currentUser.ID == id {
		response.BadRequestCode(w, r, i18n.ErrUserSelfDelete)
		return
	}

	err := h.service.Delete(id)
	if errors.Is(err, ErrUserNotFound) {
		response.NotFoundCode(w, r, i18n.ErrUserNotFound)
		return
	}
	if err != nil {
		h.app.Logger.Error("users: delete error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	response.NoContent(w)
}

// HandleChangePassword changes a user's password.
func (h *Handler) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input ChangePasswordInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if input.NewPassword == "" {
		response.BadRequestCode(w, r, i18n.ErrUserPasswordRequired)
		return
	}
	if len(input.NewPassword) < 8 {
		response.BadRequestCode(w, r, i18n.ErrAuthPasswordShort)
		return
	}

	// Verify current password
	passwordHash, err := h.service.PasswordHash(id)
	if errors.Is(err, ErrUserNotFound) {
		response.NotFoundCode(w, r, i18n.ErrUserNotFound)
		return
	}
	if err != nil {
		h.app.Logger.Error("users: password lookup error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	// Only require current password if changing your own (not for admin changing another user)
	currentUser := auth.UserFromContext(r.Context())
	if currentUser != nil && currentUser.ID == id {
		if input.CurrentPassword == "" {
			response.BadRequestCode(w, r, i18n.ErrUserPasswordRequired)
			return
		}
		valid, verifyErr := auth.VerifyPassword(input.CurrentPassword, passwordHash)
		if verifyErr != nil || !valid {
			response.BadRequestCode(w, r, i18n.ErrUserPasswordIncorrect)
			return
		}
	}

	if err := h.service.ChangePassword(id, input.NewPassword); err != nil {
		h.app.Logger.Error("users: change password error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	response.OKMsg(w, r, i18n.MsgUserPasswordUpdated)
}

// HandleListGroups returns group memberships for a user.
func (h *Handler) HandleListGroups(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")

	items, err := h.service.ListGroups(userID)
	if err != nil {
		h.app.Logger.Error("users: list groups error", "error", err, "userId", userID)
		response.InternalErrorCode(w, r, i18n.ErrUserGroupListFailed)
		return
	}

	response.OK(w, items)
}

// HandleListRoles returns role assignments for a user.
func (h *Handler) HandleListRoles(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")

	items, err := h.service.ListRoles(userID)
	if err != nil {
		h.app.Logger.Error("users: list roles error", "error", err, "userId", userID)
		response.InternalErrorCode(w, r, i18n.ErrUserRoleListFailed)
		return
	}

	response.OK(w, items)
}

// HandleAssignRole assigns a role to a user.
func (h *Handler) HandleAssignRole(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")

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

	if err := h.service.AssignRole(userID, input); err != nil {
		h.app.Logger.Error("users: assign role error", "error", err, "userId", userID)
		response.InternalErrorCode(w, r, i18n.ErrUserRoleAssignFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "assign_role",
		EntityType: "user",
		EntityID:   userID,
		Details:    "role " + input.RoleID + " assigned",
	})

	response.OKMsg(w, r, i18n.MsgUserRoleAssigned)
}

// HandleUnassignRole removes a role assignment from a user.
func (h *Handler) HandleUnassignRole(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	assignmentID := chi.URLParam(r, "assignmentId")

	err := h.service.UnassignRole(userID, assignmentID)
	if errors.Is(err, ErrRoleAssignmentNotFound) {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	if err != nil {
		h.app.Logger.Error("users: unassign role error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrUserRoleUnassignFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "unassign_role",
		EntityType: "user",
		EntityID:   userID,
		Details:    "assignment " + assignmentID + " removed",
	})

	response.OKMsg(w, r, i18n.MsgUserRoleUnassigned)
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
