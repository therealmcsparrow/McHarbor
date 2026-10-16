// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package users

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// Phase 4 account-management HTTP handlers. Thin layer: parse body,
// invoke service, audit, respond. Permission gating is in routes.go.

// DisableAccountInput is the body for POST /users/{id}/disable.
type DisableAccountInput struct {
	Reason string `json:"reason"`
}

// AdminSetPasswordInput is the body for POST /users/{id}/password/admin.
// Distinct from ChangePasswordInput: the admin does NOT supply the
// current password.
type AdminSetPasswordInput struct {
	NewPassword string `json:"newPassword"`
}

// auditUsername resolves a user id to its username for the audit trail, empty
// when the lookup fails: the row is worth writing without a display name, and
// an audit log that only names deleted accounts by id is unreadable later.
func (h *Handler) auditUsername(ctx context.Context, id string) string {
	u, err := h.service.ByID(ctx, id)
	if err != nil || u == nil {
		return ""
	}
	return u.Username
}

// HandleDisable marks a user as disabled. POST /users/{id}/disable
// Permission: PermAccountManage.
func (h *Handler) HandleDisable(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	if targetID == user.ID {
		response.BadRequestCode(w, r, i18n.ErrUserSelfDelete)
		return
	}

	var input DisableAccountInput
	if err := response.DecodeBody(r, &input); err != nil && !errors.Is(err, io.EOF) { // an absent body is fine, the reason is optional
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	err := h.service.Disable(r.Context(), targetID, user.ID, input.Reason)
	switch {
	case errors.Is(err, ErrUserNotFound):
		response.NotFoundCode(w, r, i18n.ErrUserNotFound)
		return
	case errors.Is(err, ErrUserAlreadyDisabled):
		response.ConflictCode(w, r, i18n.ErrUserAlreadyDisabled)
		return
	case errors.Is(err, ErrLastAdmin):
		response.ConflictCode(w, r, i18n.ErrUserLastAdmin)
		return
	case errors.Is(err, ErrUserIdentityProviderLocked):
		response.ConflictCode(w, r, i18n.ErrUserIdentityProviderLocked)
		return
	case err != nil:
		h.app.Logger.Error("users: disable failed", "error", err, "id", targetID)
		response.InternalErrorCause(w, r, i18n.ErrUserUpdateFailed, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "user.disable",
		EntityType: "user",
		EntityID:   targetID,
		EntityName: h.auditUsername(r.Context(), targetID),
		Details:    "reason=" + input.Reason,
	})
	response.OKMsg(w, r, i18n.MsgUserDisabled)
}

// HandleEnable re-enables a previously disabled user. POST /users/{id}/enable
// Permission: PermAccountManage.
func (h *Handler) HandleEnable(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	err := h.service.Enable(r.Context(), targetID)
	switch {
	case errors.Is(err, ErrUserNotFound):
		response.NotFoundCode(w, r, i18n.ErrUserNotFound)
		return
	case errors.Is(err, ErrUserNotDisabled):
		response.ConflictCode(w, r, i18n.ErrUserNotDisabled)
		return
	case err != nil:
		h.app.Logger.Error("users: enable failed", "error", err, "id", targetID)
		response.InternalErrorCause(w, r, i18n.ErrUserUpdateFailed, err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "user.enable",
		EntityType: "user",
		EntityID:   targetID,
		EntityName: h.auditUsername(r.Context(), targetID),
	})
	response.OKMsg(w, r, i18n.MsgUserEnabled)
}

// HandleForceResetPassword stamps must_reset_password on the user.
// POST /users/{id}/force-reset-password
// Permission: PermAccountPasswordReset.
func (h *Handler) HandleForceResetPassword(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	err := h.service.ForceResetPassword(r.Context(), targetID, user.ID)
	switch {
	case errors.Is(err, ErrUserNotFound):
		response.NotFoundCode(w, r, i18n.ErrUserNotFound)
		return
	case errors.Is(err, ErrUserIdentityProviderLocked):
		response.ConflictCode(w, r, i18n.ErrUserIdentityProviderLocked)
		return
	case err != nil:
		h.app.Logger.Error("users: force-reset failed", "error", err, "id", targetID)
		response.InternalErrorCause(w, r, i18n.ErrUserUpdateFailed, err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "user.force_reset_password",
		EntityType: "user",
		EntityID:   targetID,
		EntityName: h.auditUsername(r.Context(), targetID),
	})
	response.OKMsg(w, r, i18n.MsgUserForceReset)
}

// HandleUnlock clears any active lockout and the failed-attempt
// counter for the target user. Idempotent — calling it on a
// non-locked user is a no-op. POST /users/{id}/unlock
// Permission: PermAccountManage.
func (h *Handler) HandleUnlock(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	if targetID == user.ID {
		response.BadRequestCode(w, r, i18n.ErrUserSelfDelete)
		return
	}
	err := h.service.Unlock(r.Context(), targetID, user.ID)
	switch {
	case errors.Is(err, ErrUserNotFound):
		response.NotFoundCode(w, r, i18n.ErrUserNotFound)
		return
	case err != nil:
		h.app.Logger.Error("users: unlock failed", "error", err, "id", targetID)
		response.InternalErrorCause(w, r, i18n.ErrUserUpdateFailed, err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "user.unlock",
		EntityType: "user",
		EntityID:   targetID,
		EntityName: h.auditUsername(r.Context(), targetID),
	})
	response.OKMsg(w, r, i18n.MsgUserUnlocked)
}

// HandleAdminSetPassword overwrites a user's password without
// requiring the current one. POST /users/{id}/password/admin
// Permission: PermAccountPasswordReset.
func (h *Handler) HandleAdminSetPassword(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	var input AdminSetPasswordInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	// No hardcoded length gate here: AdminSetPassword runs the
	// candidate through the runtime policy, which owns MinLength.
	// A local `< 8` check would silently pre-empt a policy set to
	// 12 and report the wrong reason.
	if input.NewPassword == "" {
		response.BadRequestCode(w, r, i18n.ErrUserPasswordRequired)
		return
	}
	err := h.service.AdminSetPassword(r.Context(), targetID, input.NewPassword)
	switch {
	case errors.Is(err, ErrUserNotFound):
		response.NotFoundCode(w, r, i18n.ErrUserNotFound)
		return
	case errors.Is(err, ErrUserIdentityProviderLocked):
		response.ConflictCode(w, r, i18n.ErrUserIdentityProviderLocked)
		return
	case err != nil:
		var pverr *auth.PasswordValidationError
		if errors.As(err, &pverr) {
			respondPasswordPolicyError(w, r, pverr)
			return
		}
		h.app.Logger.Error("users: admin set password failed", "error", err, "id", targetID)
		response.InternalErrorCause(w, r, i18n.ErrUserUpdateFailed, err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "user.admin_set_password",
		EntityType: "user",
		EntityID:   targetID,
		EntityName: h.auditUsername(r.Context(), targetID),
	})
	response.OKMsg(w, r, i18n.MsgUserPasswordUpdated)
}

// HandleListSessions returns the active + recent sessions for the
// target user. The caller's own session is flagged.
//
// GET /users/{id}/sessions
// Permission: self → PermSessionsView, other → PermSessionsManage.
func (h *Handler) HandleListSessions(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	current := auth.UserFromContext(r.Context())
	if current == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	currentSessionID := sessionIDFromCookie(r)
	items, err := h.service.ListSessions(r.Context(), targetID, currentSessionID)
	if err != nil {
		h.app.Logger.Error("users: list sessions failed", "error", err, "id", targetID)
		response.InternalErrorCause(w, r, i18n.ErrUserSessionListFailed, err)
		return
	}
	response.OK(w, items)
}

// HandleRevokeSession revokes a single session. Caller cannot
// revoke their own current session via this endpoint (use logout).
//
// DELETE /users/{id}/sessions/{sessionId}
func (h *Handler) HandleRevokeSession(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	sessionID := chi.URLParam(r, "sessionId")
	current := auth.UserFromContext(r.Context())
	if current == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	if currentSessionID := sessionIDFromCookie(r); currentSessionID == sessionID {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	err := h.service.RevokeSession(r.Context(), targetID, sessionID, current.ID)
	switch {
	case errors.Is(err, ErrRoleAssignmentNotFound):
		response.NotFoundCode(w, r, i18n.ErrUserNotFound)
		return
	case err != nil:
		h.app.Logger.Error("users: revoke session failed", "error", err, "id", targetID, "session", sessionID)
		response.InternalErrorCause(w, r, i18n.ErrUserSessionRevokeFailed, err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "user.session.revoke",
		EntityType: "user",
		EntityID:   targetID,
		EntityName: h.auditUsername(r.Context(), targetID),
		Details:    "sessionId=" + sessionID,
	})
	response.OKMsg(w, r, i18n.MsgUserSessionRevoked)
}

// HandleRevokeAllSessions revokes every session for the target
// user. The caller's own session is preserved by default (use
// logout to end your own). Pass ?includeSelf=true to override.
//
// POST /users/{id}/sessions/revoke-all
func (h *Handler) HandleRevokeAllSessions(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	current := auth.UserFromContext(r.Context())
	if current == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	includeSelf := r.URL.Query().Get("includeSelf") == "true"
	keep := ""
	if !includeSelf {
		keep = sessionIDFromCookie(r)
	}
	if includeSelf && current.ID == targetID {
		// Can't be both the actor and the target when revoking self.
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	n, err := h.service.RevokeAllSessions(r.Context(), targetID, keep, current.ID)
	if err != nil {
		h.app.Logger.Error("users: revoke all sessions failed", "error", err, "id", targetID)
		response.InternalErrorCause(w, r, i18n.ErrUserSessionRevokeFailed, err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "user.session.revoke_all",
		EntityType: "user",
		EntityID:   targetID,
		EntityName: h.auditUsername(r.Context(), targetID),
		Details:    fmt.Sprintf("count=%d includeSelf=%v", n, includeSelf),
	})
	response.OK(w, map[string]any{"revoked": n, "message": "all sessions revoked"})
}

// HandleMySessions is a convenience endpoint for the current user
// to list their own sessions. GET /auth/sessions
func (h *Handler) HandleMySessions(w http.ResponseWriter, r *http.Request) {
	current := auth.UserFromContext(r.Context())
	if current == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	currentSessionID := sessionIDFromCookie(r)
	items, err := h.service.ListSessions(r.Context(), current.ID, currentSessionID)
	if err != nil {
		h.app.Logger.Error("users: list my sessions failed", "error", err, "id", current.ID)
		response.InternalErrorCause(w, r, i18n.ErrUserSessionListFailed, err)
		return
	}
	response.OK(w, items)
}

// HandleRevokeMySession is a convenience endpoint for the current
// user to revoke one of their own other sessions (use /auth/logout
// to end the current one). DELETE /auth/sessions/{sessionId}
func (h *Handler) HandleRevokeMySession(w http.ResponseWriter, r *http.Request) {
	current := auth.UserFromContext(r.Context())
	if current == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	sessionID := chi.URLParam(r, "sessionId")
	if currentSessionID := sessionIDFromCookie(r); currentSessionID == sessionID {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	err := h.service.RevokeSession(r.Context(), current.ID, sessionID, current.ID)
	switch {
	case errors.Is(err, ErrRoleAssignmentNotFound):
		response.NotFoundCode(w, r, i18n.ErrUserNotFound)
		return
	case err != nil:
		h.app.Logger.Error("users: revoke my session failed", "error", err, "id", current.ID, "session", sessionID)
		response.InternalErrorCause(w, r, i18n.ErrUserSessionRevokeFailed, err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "auth.session.revoke_self",
		EntityType: "user",
		EntityID:   current.ID,
		EntityName: current.Username,
		Details:    "sessionId=" + sessionID,
	})
	response.OKMsg(w, r, i18n.MsgUserSessionRevoked)
}

// HandleRevokeAllMySessions revokes all the user's other sessions
// (the current one is preserved). POST /auth/sessions/revoke-all
func (h *Handler) HandleRevokeAllMySessions(w http.ResponseWriter, r *http.Request) {
	current := auth.UserFromContext(r.Context())
	if current == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	keep := sessionIDFromCookie(r)
	n, err := h.service.RevokeAllSessions(r.Context(), current.ID, keep, current.ID)
	if err != nil {
		h.app.Logger.Error("users: revoke all my sessions failed", "error", err, "id", current.ID)
		response.InternalErrorCause(w, r, i18n.ErrUserSessionRevokeFailed, err)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "auth.session.revoke_all_self",
		EntityType: "user",
		EntityID:   current.ID,
		EntityName: current.Username,
		Details:    fmt.Sprintf("count=%d", n),
	})
	response.OK(w, map[string]any{"revoked": n})
}

// sessionIDFromCookie is the caller's own session as the sessions table keys it: the
// hash of the cookie value, so it compares against listed and revoked row ids. It is ""
// for an API-key request, which has no session to protect.
func sessionIDFromCookie(r *http.Request) string {
	return auth.SessionKey(rawSessionCookie(r))
}

// rawSessionCookie is the session cookie's own value, which core/auth hashes itself; it is
// what a step-up window check takes.
func rawSessionCookie(r *http.Request) string {
	cookie, err := r.Cookie(auth.SessionCookie)
	if err != nil {
		return ""
	}
	return cookie.Value
}
