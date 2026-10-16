// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package users

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// respondPasswordPolicyError maps a policy rejection onto the
// matching i18n code. Shared by every write path that runs a
// candidate through auth.ValidatePassword.
func respondPasswordPolicyError(w http.ResponseWriter, r *http.Request, err *auth.PasswordValidationError) {
	response.BadRequestCode(w, r, err.Code())
}

// User represents a user record returned from the API.
type User struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName *string `json:"displayName"`
	Email       *string `json:"email"`
	Role        string  `json:"role"`
	IsActive    bool    `json:"isActive"`
	LastLogin   *string `json:"lastLogin"`
	// LockedUntil carries password_locked_until so the UI can tell a
	// locked-out account (too many failed sign-ins, self-clearing) from a
	// disabled one (an admin turned it off). Nil when the account is not
	// currently locked out. No secret: it is a timestamp, not a counter of
	// attempts or a hash.
	LockedUntil *string `json:"lockedUntil"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   string  `json:"updatedAt"`
	// RoleNames and GroupNames are populated only by List, from a
	// GROUP_CONCAT join; every other path (Get, Create, Update) omits
	// them, so the tag drops the field rather than send an empty array
	// that path never actually computed.
	RoleNames  []string `json:"roleNames,omitempty"`
	GroupNames []string `json:"groupNames,omitempty"`
	// Kind is "user" for a person and "service" for a service principal, which
	// holds roles like anyone else but acts unattended and can never sign in.
	Kind string `json:"kind"`
}

// KindUser and KindService are the two principal kinds a users row can carry.
const (
	KindUser    = "user"
	KindService = "service"
)

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
	// Kind "service" creates a service principal: no password, an explicit role.
	Kind string `json:"kind,omitempty"`
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

// UserPermissionOverride is a single grant/deny entry for a user.
// Phase 2 of the RBAC roadmap. The data model lives in
// migration 070_rbac_v2.sql (`user_permissions` table) and is
// already consulted by EffectivePermissions — this just exposes
// a CRUD surface for admins to manage it from the UI.
type UserPermissionOverride struct {
	ID            string  `json:"id"`
	PermissionKey string  `json:"permissionKey"`
	Effect        string  `json:"effect"` // "grant" or "deny"
	EnvironmentID *string `json:"environmentId"`
	StackName     *string `json:"stackName"`
	ResourceType  *string `json:"resourceType"`
	ResourceID    *string `json:"resourceId"`
	GrantedBy     string  `json:"grantedBy"`
	ExpiresAt     *string `json:"expiresAt"`
	Reason        *string `json:"reason"`
	CreatedAt     string  `json:"createdAt"`
	UpdatedAt     string  `json:"updatedAt"`
}

// SetUserOverridesInput is the PUT body: a complete replacement of
// the user's override list. The server diffs this against the
// existing rows and writes one audit entry per change.
type SetUserOverridesInput struct {
	Overrides []UserPermissionOverride `json:"overrides"`
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
		service: NewService(app.DB, app.RBACService, app.AuthService),
	}
}

// HandleList returns a paginated list of users.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	page, perPage := response.ParsePagination(r)

	items, total, err := h.service.List(r.Context(), page, perPage)
	if err != nil {
		h.app.Logger.Error("users: list error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
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
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	input.Username = strings.TrimSpace(input.Username)
	input.DisplayName = normalizeOptionalString(input.DisplayName)
	input.Email = normalizeOptionalString(input.Email)
	input.RoleID = normalizeOptionalString(input.RoleID)

	switch input.Kind {
	case "", KindUser:
	case KindService:
		if input.Password != "" {
			response.BadRequestCode(w, r, i18n.ErrUserServicePrincipalSignIn)
			return
		}
		if input.RoleID == nil {
			response.BadRequestCode(w, r, i18n.ErrUserServicePrincipalRole)
			return
		}
	default:
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if input.Username == "" || (input.Kind != KindService && input.Password == "") {
		response.BadRequestCode(w, r, i18n.ErrAuthUsernameRequired)
		return
	}
	policy := auth.LoadPasswordPolicy(r.Context(), h.app.DB)
	if err := auth.ValidatePassword(input.Password, policy); input.Kind != KindService && err != nil {
		var pverr *auth.PasswordValidationError
		if errors.As(err, &pverr) {
			respondPasswordPolicyError(w, r, pverr)
			return
		}
		response.BadRequestCause(w, r, i18n.ErrPasswordTooWeak, err)
		return
	}

	u, err := h.service.Create(r.Context(), input)
	if errors.Is(err, ErrUserExists) {
		response.ConflictCode(w, r, i18n.ErrAuthUsernameTaken)
		return
	}
	if errors.Is(err, ErrRoleNotFound) {
		response.BadRequestCode(w, r, i18n.ErrRoleNotFound)
		return
	}
	if errors.Is(err, ErrNoDefaultRole) {
		response.BadRequestCode(w, r, i18n.ErrUserCreateNoDefault)
		return
	}
	if err != nil {
		h.app.Logger.Error("users: create error", "error", err, "username", input.Username)
		response.InternalErrorCause(w, r, i18n.ErrUserCreateFailed, err)
		return
	}

	entry := audit.Entry{
		Action:     "create",
		EntityType: "user",
		EntityID:   u.ID,
		EntityName: u.Username,
	}
	if diffEntry, diffErr := entry.WithDiff(nil, u); diffErr == nil {
		entry = diffEntry
	} else {
		h.app.Logger.Error("users: audit diff error", "error", diffErr, "id", u.ID)
	}
	h.app.AuditLog.LogWithUser(r, currentUser.ID, currentUser.Username, entry)

	response.Created(w, u)
}

// HandleGet returns a single user.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	u, err := h.service.ByID(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("users: get error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
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
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	existing, err := h.service.ByID(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("users: get before update error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if existing == nil {
		response.NotFoundCode(w, r, i18n.ErrUserNotFound)
		return
	}

	u, err := h.service.Update(r.Context(), id, input)
	if errors.Is(err, ErrUserNotFound) {
		response.NotFoundCode(w, r, i18n.ErrUserNotFound)
		return
	}
	if err != nil {
		h.app.Logger.Error("users: update error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	entry := audit.Entry{
		Action:     "update",
		EntityType: "user",
		EntityID:   id,
		EntityName: u.Username,
	}
	if diffEntry, diffErr := entry.WithDiff(existing, u); diffErr == nil {
		entry = diffEntry
	} else {
		h.app.Logger.Error("users: audit diff error", "error", diffErr, "id", id)
	}
	h.app.AuditLog.Log(r, entry)

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

	existing, err := h.service.ByID(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("users: get before delete error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if existing == nil {
		response.NotFoundCode(w, r, i18n.ErrUserNotFound)
		return
	}

	err = h.service.Delete(r.Context(), id)
	if errors.Is(err, ErrUserNotFound) {
		response.NotFoundCode(w, r, i18n.ErrUserNotFound)
		return
	}
	if err != nil {
		h.app.Logger.Error("users: delete error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	entry := audit.Entry{
		Action:     "delete",
		EntityType: "user",
		EntityID:   id,
		EntityName: existing.Username,
	}
	if diffEntry, diffErr := entry.WithDiff(existing, nil); diffErr == nil {
		entry = diffEntry
	} else {
		h.app.Logger.Error("users: audit diff error", "error", diffErr, "id", id)
	}
	h.app.AuditLog.Log(r, entry)

	response.NoContent(w)
}

// HandleChangePassword changes a user's password.
func (h *Handler) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input ChangePasswordInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if input.NewPassword == "" {
		response.BadRequestCode(w, r, i18n.ErrUserPasswordRequired)
		return
	}

	// This route serves two audiences: a user changing their own
	// password (proved by the current password) and an admin
	// changing someone else's (proved by account.password.reset).
	// Neither may be skipped — without the second branch any
	// authenticated caller could overwrite an admin's password.
	currentUser := auth.RequireAuth(r)
	if currentUser == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	isSelf := currentUser.ID == id
	if !isSelf && currentUser.ID != "system" {
		allowed, permErr := h.app.RBACService.HasPermission(r.Context(), currentUser.ID, "", rbac.PermAccountPasswordReset)
		if permErr != nil {
			h.app.Logger.Error("users: password permission check error", "error", permErr, "id", id)
			response.InternalErrorCause(w, r, i18n.ErrInternalServer, permErr)
			return
		}
		if !allowed {
			rbac.RecordDenied(r, currentUser, rbac.PermAccountPasswordReset, "")
			response.ForbiddenCode(w, r, i18n.ErrRBACPermissionDenied)
			return
		}
	}

	// Verify current password
	passwordHash, err := h.service.PasswordHash(r.Context(), id)
	if errors.Is(err, ErrUserNotFound) {
		response.NotFoundCode(w, r, i18n.ErrUserNotFound)
		return
	}
	if err != nil {
		h.app.Logger.Error("users: password lookup error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	// Only require current password if changing your own (not for admin changing another user)
	if isSelf {
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

	if err := h.service.ChangePassword(r.Context(), id, input.NewPassword); err != nil {
		var pverr *auth.PasswordValidationError
		if errors.As(err, &pverr) {
			respondPasswordPolicyError(w, r, pverr)
			return
		}
		if errors.Is(err, ErrUserIdentityProviderLocked) {
			response.ConflictCode(w, r, i18n.ErrUserIdentityProviderLocked)
			return
		}
		h.app.Logger.Error("users: change password error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	entry := audit.Entry{
		Action:     "update",
		EntityType: "user",
		EntityID:   id,
		EntityName: h.auditUsername(r.Context(), id),
		Details:    "password changed",
	}
	if diffEntry, diffErr := entry.WithDiff(map[string]string{"password": "previous"}, map[string]string{"password": "updated"}); diffErr == nil {
		entry = diffEntry
	} else {
		h.app.Logger.Error("users: audit diff error", "error", diffErr, "id", id)
	}
	h.app.AuditLog.Log(r, entry)

	response.OKMsg(w, r, i18n.MsgUserPasswordUpdated)
}

// HandleListGroups returns group memberships for a user.
func (h *Handler) HandleListGroups(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")

	items, err := h.service.ListGroups(r.Context(), userID)
	if err != nil {
		h.app.Logger.Error("users: list groups error", "error", err, "userId", userID)
		response.InternalErrorCause(w, r, i18n.ErrUserGroupListFailed, err)
		return
	}

	response.OK(w, items)
}

// HandleListRoles returns role assignments for a user.
func (h *Handler) HandleListRoles(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")

	items, err := h.service.ListRoles(r.Context(), userID)
	if err != nil {
		h.app.Logger.Error("users: list roles error", "error", err, "userId", userID)
		response.InternalErrorCause(w, r, i18n.ErrUserRoleListFailed, err)
		return
	}

	response.OK(w, items)
}

// HandleAssignRole assigns a role to a user.
func (h *Handler) HandleAssignRole(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")

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

	before, err := h.service.ListRoles(r.Context(), userID)
	if err != nil {
		h.app.Logger.Error("users: list roles before assign error", "error", err, "userId", userID)
		response.InternalErrorCause(w, r, i18n.ErrUserRoleListFailed, err)
		return
	}

	if err := h.service.AssignRole(r.Context(), userID, input); err != nil {
		h.app.Logger.Error("users: assign role error", "error", err, "userId", userID)
		response.InternalErrorCause(w, r, i18n.ErrUserRoleAssignFailed, err)
		return
	}

	// Audit details include the scope so the audit log is searchable
	// by environment / stack. Phase 3 of the RBAC roadmap.
	scopeDesc := "global"
	if input.EnvironmentID != nil {
		scopeDesc = "env=" + *input.EnvironmentID
		if input.StackName != nil {
			scopeDesc += "/stack=" + *input.StackName
		}
	}

	after, err := h.service.ListRoles(r.Context(), userID)
	if err != nil {
		h.app.Logger.Error("users: list roles after assign error", "error", err, "userId", userID)
		after = before
	}
	entry := audit.Entry{
		Action:     "assign_role",
		EntityType: "user",
		EntityID:   userID,
		EntityName: h.auditUsername(r.Context(), userID),
		Details:    "role " + input.RoleID + " assigned (scope: " + scopeDesc + ")",
	}
	if diffEntry, diffErr := entry.WithDiff(map[string]any{"roles": before}, map[string]any{"roles": after}); diffErr == nil {
		entry = diffEntry
	} else {
		h.app.Logger.Error("users: audit diff error", "error", diffErr, "userId", userID)
	}
	h.app.AuditLog.Log(r, entry)

	response.OKMsg(w, r, i18n.MsgUserRoleAssigned)
}

// HandleUnassignRole removes a role assignment from a user.
func (h *Handler) HandleUnassignRole(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	assignmentID := chi.URLParam(r, "assignmentId")

	before, err := h.service.ListRoles(r.Context(), userID)
	if err != nil {
		h.app.Logger.Error("users: list roles before unassign error", "error", err, "userId", userID)
		response.InternalErrorCause(w, r, i18n.ErrUserRoleListFailed, err)
		return
	}

	scopeDesc, err := h.service.UnassignRole(r.Context(), userID, assignmentID)
	if errors.Is(err, ErrRoleAssignmentNotFound) {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	if err != nil {
		h.app.Logger.Error("users: unassign role error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrUserRoleUnassignFailed, err)
		return
	}

	after, err := h.service.ListRoles(r.Context(), userID)
	if err != nil {
		h.app.Logger.Error("users: list roles after unassign error", "error", err, "userId", userID)
		after = before
	}
	entry := audit.Entry{
		Action:     "unassign_role",
		EntityType: "user",
		EntityID:   userID,
		EntityName: h.auditUsername(r.Context(), userID),
		Details:    "assignment " + assignmentID + " removed (scope: " + scopeDesc + ")",
	}
	if diffEntry, diffErr := entry.WithDiff(map[string]any{"roles": before}, map[string]any{"roles": after}); diffErr == nil {
		entry = diffEntry
	} else {
		h.app.Logger.Error("users: audit diff error", "error", diffErr, "userId", userID)
	}
	h.app.AuditLog.Log(r, entry)

	response.OKMsg(w, r, i18n.MsgUserRoleUnassigned)
}

// HandleListOverrides returns the user's per-permission grant/deny
// overrides. Phase 2 of the RBAC roadmap.
func (h *Handler) HandleListOverrides(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")

	if _, err := h.service.ByID(r.Context(), userID); err != nil {
		response.InternalErrorCause(w, r, i18n.ErrUserListFailed, err)
		return
	}

	overrides, err := h.service.ListOverrides(r.Context(), userID)
	if err != nil {
		h.app.Logger.Error("users: list overrides error", "error", err, "userId", userID)
		response.InternalErrorCause(w, r, i18n.ErrUserListFailed, err)
		return
	}

	response.OK(w, map[string]any{"overrides": overrides})
}

// HandleSetOverrides replaces the user's per-permission grant/deny
// overrides atomically. One audit entry summarises the diff.
func (h *Handler) HandleSetOverrides(w http.ResponseWriter, r *http.Request) {
	currentUser := auth.RequireAuth(r)
	if currentUser == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	userID := chi.URLParam(r, "id")

	var input SetUserOverridesInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	for _, o := range input.Overrides {
		if o.Effect != "grant" && o.Effect != "deny" {
			response.BadRequestCode(w, r, i18n.ErrInvalidBody)
			return
		}
		if strings.TrimSpace(o.PermissionKey) == "" {
			response.BadRequestCode(w, r, i18n.ErrInvalidBody)
			return
		}
	}

	added, removed, err := h.service.SetOverrides(r.Context(), userID, currentUser.ID, input)
	if err != nil {
		h.app.Logger.Error("users: set overrides error", "error", err, "userId", userID)
		response.InternalErrorCause(w, r, i18n.ErrUserListFailed, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "set_overrides",
		EntityType: "user",
		EntityID:   userID,
		EntityName: h.auditUsername(r.Context(), userID),
		Details:    fmt.Sprintf("+ %d / - %d permission overrides", added, removed),
	})

	response.OK(w, map[string]any{
		"added":   added,
		"removed": removed,
	})
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
