// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/httpx"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
)

// Handler holds dependencies for auth HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
	// mfa gates the second login step. Injected as a narrow contract so
	// this module does not import the mfa module (see bridges.go).
	mfa MFAVerifier
}

// NewHandler creates a new auth handler with no second-factor bridge: every
// account then logs in with a password alone. main.go uses NewHandlerWithMFA.
func NewHandler(app *router.AppDeps) *Handler {
	return NewHandlerWithMFA(app, nil)
}

// NewHandlerWithMFA creates the handler with the second-factor verifier
// injected (see bridges.go).
func NewHandlerWithMFA(app *router.AppDeps, verifier MFAVerifier) *Handler {
	if verifier == nil {
		verifier = noMFA{}
	}
	return &Handler{
		app:     app,
		service: NewService(app.DB, app.Logger),
		mfa:     verifier,
	}
}

// loginRequest is the JSON body for POST /auth/login.
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// setupRequest is the JSON body for POST /auth/setup.
type setupRequest struct {
	Username string  `json:"username"`
	Password string  `json:"password"`
	Email    *string `json:"email,omitempty"`
}

type preferencesRequest struct {
	PreferredLanguage *string `json:"preferredLanguage,omitempty"`
	TimeFormat        *string `json:"timeFormat,omitempty"`
	DateFormat        *string `json:"dateFormat,omitempty"`

	// clearToastOverride:true drops this account's toast overrides so it
	// follows the app-wide default again. Sent on its own; the five value
	// fields below are what the "use my own notification settings" switch
	// sends when it is on.
	ClearToastOverride bool    `json:"clearToastOverride,omitempty"`
	ToastPosition      *string `json:"toastPosition,omitempty"`
	ToastDurationMs    *int    `json:"toastDurationMs,omitempty"`
	ToastMaxVisible    *int    `json:"toastMaxVisible,omitempty"`
	ToastRichColors    *bool   `json:"toastRichColors,omitempty"`
	ToastCloseButton   *bool   `json:"toastCloseButton,omitempty"`

	// clearAppearanceOverride:true drops this account's theme, accent, font
	// and base font size so it follows the app-wide default again. Same
	// contract as clearToastOverride: an omitted value field already means
	// "leave unchanged", so unsetting needs a flag of its own.
	ClearAppearanceOverride bool    `json:"clearAppearanceOverride,omitempty"`
	AppearanceThemeID       *string `json:"appearanceThemeId,omitempty"`
	AppearanceAccent        *string `json:"appearanceAccent,omitempty"`
	AppearanceFont          *string `json:"appearanceFont,omitempty"`
	AppearanceBaseFontPx    *int    `json:"appearanceBaseFontPx,omitempty"`
}

type profileRequest struct {
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
}

// HandleLogin authenticates a user and sets a session cookie.
func (h *Handler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if req.Username == "" || req.Password == "" {
		response.BadRequestCode(w, r, i18n.ErrAuthUsernameRequired)
		return
	}

	result, err := h.app.AuthService.LoginFromContext(r.Context(), req.Username, req.Password, resolveLoginClient(h.app, r))
	if err != nil {
		h.app.Logger.Error("login error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	if !result.Success {
		h.refuseLogin(w, r, req.Username, result, nil)
		return
	}

	h.completeLogin(w, r, result, nil)
}

// refuseLogin audits a refused sign-in and writes the refusal. Password and
// external-directory sign-ins share it, so a lock, a disabled account and a wrong
// credential get the same status and body whichever form the user filled in.
func (h *Handler) refuseLogin(w http.ResponseWriter, r *http.Request, attempted string, result *auth.AuthResult, extra map[string]string) {
	failureReason := "invalid_credentials"
	switch {
	case result.Locked:
		failureReason = "locked"
	case result.Disabled:
		failureReason = "disabled"
	}
	// Normalised (not just trimmed) so this audit trail joins with
	// login_attempts.username, which auth.LoginWithContext also
	// stores normalised.
	attemptedUsername := auth.NormalizeUsername(attempted)
	fields := map[string]string{
		"attemptedUsername": attemptedUsername,
		"reason":            failureReason,
	}
	for key, value := range extra {
		fields[key] = value
	}
	details, err := json.Marshal(fields)
	if err != nil {
		h.app.Logger.Error("failed to marshal login failure audit details", "error", err)
	} else {
		h.app.AuditLog.Log(r, audit.Entry{
			Action:     "login_failed",
			Severity:   "error",
			EntityType: "user",
			EntityName: attemptedUsername,
			Details:    string(details),
		})
	}

	// Lockout / disabled need a richer payload than the generic
	// 401 — the frontend uses the extra fields to render a
	// countdown or disable-screen instead of a blank failure
	// toast.
	status := http.StatusUnauthorized
	switch {
	case result.Locked:
		status = http.StatusLocked // 423
	case result.Disabled:
		status = http.StatusForbidden // 403
	}
	writeLoginError(w, status, result)
}

// auditDetails renders extra as an audit entry's JSON details, or "" when there is
// nothing to add, so a password login's entries stay exactly as they were.
func (h *Handler) auditDetails(extra map[string]string) string {
	if len(extra) == 0 {
		return ""
	}
	details, err := json.Marshal(extra)
	if err != nil {
		h.app.Logger.Error("failed to marshal login audit details", "error", err)
		return ""
	}
	return string(details)
}

// completeLogin finishes a sign-in whose credential already checked out and whose
// result carries a fresh session: the MFA challenge, the enrolment gate, the cookie
// and the audit trail. External-directory sign-ins end here too, so neither path
// can skip the second factor the other enforces.
func (h *Handler) completeLogin(w http.ResponseWriter, r *http.Request, result *auth.AuthResult, extra map[string]string) {
	details := h.auditDetails(extra)

	// Second-factor gate. The password check above already minted a
	// session; if the account carries MFA we throw that session away
	// and hand back a short-lived challenge instead, so a caller who
	// stops here never holds a usable cookie.
	mfaEnabled, err := h.mfa.IsEnabled(r.Context(), result.User.ID)
	if err != nil {
		h.app.Logger.Error("mfa lookup failed during login", "userId", result.User.ID, "error", err)
		h.app.AuthService.DestroySessionContext(context.WithoutCancel(r.Context()), result.SessionID)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	if mfaEnabled {
		h.app.AuthService.DestroySessionContext(context.WithoutCancel(r.Context()), result.SessionID)

		// LoginWithContext clears must_reset_password as a side
		// effect of the password check. Re-arm it so the forced
		// rotation survives to the second step instead of being
		// silently dropped by the extra round trip.
		if result.MustResetPassword {
			if err := h.app.AuthService.SetMustResetPasswordContext(context.WithoutCancel(r.Context()), result.User.ID, true); err != nil {
				h.app.Logger.Error("restoring must_reset_password failed", "userId", result.User.ID, "error", err)
			}
		}

		challengeID, err := h.mfa.CreateChallenge(r.Context(), result.User.ID)
		if err != nil {
			h.app.Logger.Error("creating mfa challenge failed", "userId", result.User.ID, "error", err)
			response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
			return
		}

		h.app.AuditLog.LogWithUser(r, result.User.ID, result.User.Username, audit.Entry{
			Action:     "login_mfa_challenge",
			EntityType: "user",
			EntityID:   result.User.ID,
			EntityName: result.User.Username,
			Details:    details,
		})

		// Same flat shape as the success body — the client reads the
		// top level, never a `data` envelope.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		writeFlatJSON(w, map[string]any{
			"mfaRequired": true,
			"challengeId": challengeID,
		})
		return
	}

	// The session is still minted when enrolment is owed, because the user needs it to
	// reach the enrolment routes; the session gate holds it there until they finish.
	enrollmentOwed := false
	if h.app.MFAEnrollmentOwed != nil {
		enrollmentOwed, err = h.app.MFAEnrollmentOwed(r.Context(), result.User)
		if err != nil {
			h.app.Logger.Error("mfa policy lookup failed during login", "userId", result.User.ID, "error", err)
			h.app.AuthService.DestroySessionContext(context.WithoutCancel(r.Context()), result.SessionID)
			response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
			return
		}
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookie,
		Value:    result.SessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   httpx.ShouldSetSecureCookie(r, h.app.Config),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(auth.SessionDuration.Seconds()),
	})

	h.app.AuditLog.LogWithUser(r, result.User.ID, result.User.Username, audit.Entry{
		Action:     "login",
		EntityType: "user",
		EntityID:   result.User.ID,
		EntityName: result.User.Username,
		Details:    details,
	})
	if enrollmentOwed {
		h.app.AuditLog.LogWithUser(r, result.User.ID, result.User.Username, audit.Entry{
			Action:     "login_mfa_enrollment_required",
			Severity:   "warning",
			EntityType: "user",
			EntityID:   result.User.ID,
			EntityName: result.User.Username,
			Details:    details,
		})
	}

	// Surface MustResetPassword in the success body so the frontend
	// can redirect to the forced-reset screen before mounting the
	// rest of the app. The flag is also written into the auth store
	// by the client.
	//
	// The success body is intentionally the FLAT shape
	// `{user, mustResetPassword}` (matching the mustResetPassword
	// branch below) — never the standard `response.OK` envelope of
	// `{success:true, data:{…}}` — because the frontend's login
	// handler reads `data.user ?? data` and assumes user fields are
	// at the top level. Wrapping the user in `data` would silently
	// look like a failed login while still setting the session
	// cookie; the operator would only get in after a hard reload.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	writeFlatJSON(w, map[string]any{
		"user":                  result.User,
		"mustResetPassword":     result.MustResetPassword,
		"mfaEnrollmentRequired": enrollmentOwed,
	})
}

// writeLoginError emits a structured JSON error body so the frontend
// can read retryAfterSeconds / locked / disabled without parsing the
// error string.
func writeLoginError(w http.ResponseWriter, status int, result *auth.AuthResult) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	writeFlatJSON(w, map[string]any{
		"success":           false,
		"error":             result.Error,
		"locked":            result.Locked,
		"disabled":          result.Disabled,
		"retryAfterSeconds": result.RetryAfterSeconds,
	})
}

// loginMFARequest is the JSON body for POST /auth/login/mfa.
type loginMFARequest struct {
	ChallengeID string `json:"challengeId"`
	Code        string `json:"code"`
}

// HandleLoginMFA completes a two-step login: it verifies the second
// factor against a live challenge and only then issues the session.
//
// The challenge is *peeked* before verification and burned after, so a
// mistyped digit costs a retry instead of the whole password round.
// The public auth router's 10-requests-per-minute limiter plus the
// five-minute challenge TTL is what keeps that affordable.
//
// POST /auth/login/mfa
func (h *Handler) HandleLoginMFA(w http.ResponseWriter, r *http.Request) {
	var req loginMFARequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if req.ChallengeID == "" || strings.TrimSpace(req.Code) == "" {
		response.BadRequestCode(w, r, i18n.ErrMFAInvalidCode)
		return
	}

	userID, err := h.mfa.PeekChallenge(r.Context(), req.ChallengeID)
	if err != nil {
		if errors.Is(err, ErrMFAChallengeInvalid) {
			response.UnauthorizedCode(w, r, i18n.ErrMFAChallengeInvalid)
			return
		}
		h.app.Logger.Error("mfa challenge lookup failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	if err := h.mfa.VerifyForLogin(r.Context(), userID, req.Code); err != nil {
		if errors.Is(err, ErrMFAInvalidCode) || errors.Is(err, ErrMFANotEnrolled) {
			details, marshalErr := json.Marshal(map[string]string{"reason": "mfa_invalid"})
			if marshalErr != nil {
				h.app.Logger.Error("failed to marshal mfa failure audit details", "error", marshalErr)
			} else {
				h.app.AuditLog.LogWithUser(r, userID, "", audit.Entry{
					Action:     "login_failed",
					Severity:   "error",
					EntityType: "user",
					EntityID:   userID,
					Details:    string(details),
				})
			}
			response.UnauthorizedCode(w, r, i18n.ErrMFAInvalidCode)
			return
		}
		h.app.Logger.Error("mfa verification failed", "userId", userID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	// Burn the challenge only once the factor checked out. The guard
	// inside ConsumeChallenge makes this the point where two racing
	// requests are resolved to exactly one winner.
	if _, err := h.mfa.ConsumeChallenge(r.Context(), req.ChallengeID); err != nil {
		if errors.Is(err, ErrMFAChallengeInvalid) {
			response.UnauthorizedCode(w, r, i18n.ErrMFAChallengeInvalid)
			return
		}
		h.app.Logger.Error("consuming mfa challenge failed", "userId", userID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	user, err := h.app.AuthService.ValidateUserByIDContext(r.Context(), userID)
	if err != nil || user == nil {
		h.app.Logger.Error("mfa login user lookup failed", "userId", userID, "error", err)
		response.UnauthorizedCode(w, r, i18n.ErrAuthInvalidCredentials)
		return
	}

	mustReset, err := h.app.AuthService.MustResetPasswordContext(r.Context(), userID)
	if err != nil {
		h.app.Logger.Error("reading must_reset_password failed", "userId", userID, "error", err)
	}
	if mustReset {
		// Mirror the one-step path, which clears the flag as soon as
		// the credential check passes; the client is what routes the
		// user to the forced-change screen.
		if err := h.app.AuthService.SetMustResetPasswordContext(r.Context(), userID, false); err != nil {
			h.app.Logger.Error("clearing must_reset_password failed", "userId", userID, "error", err)
		}
	}

	sessionID, err := h.app.AuthService.CreateSessionContext(r.Context(), userID)
	if err != nil {
		h.app.Logger.Error("creating session after mfa failed", "userId", userID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookie,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   httpx.ShouldSetSecureCookie(r, h.app.Config),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(auth.SessionDuration.Seconds()),
	})

	h.app.AuditLog.LogWithUser(r, user.ID, user.Username, audit.Entry{
		Action:     "login",
		EntityType: "user",
		EntityID:   user.ID,
		EntityName: user.Username,
	})

	// Flat body, matching HandleLogin — see the comment there for why
	// this deliberately skips the response.OK envelope.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	writeFlatJSON(w, map[string]any{
		"user":              user,
		"mustResetPassword": mustReset,
	})
}

// HandleLogout destroys the session cookie.
func (h *Handler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(auth.SessionCookie)
	user := auth.UserFromContext(r.Context())
	if user == nil && err == nil && cookie.Value != "" {
		var lookupErr error
		user, lookupErr = h.app.AuthService.ValidateSessionContext(r.Context(), cookie.Value)
		if lookupErr != nil {
			h.app.Logger.Error("logout session lookup failed", "error", lookupErr)
		}
	}

	if user != nil {
		h.app.AuditLog.LogWithUser(r, user.ID, user.Username, audit.Entry{
			Action:     "logout",
			EntityType: "user",
			EntityID:   user.ID,
			EntityName: user.Username,
		})
	}

	if err == nil && cookie.Value != "" {
		h.app.AuthService.DestroySessionContext(context.WithoutCancel(r.Context()), cookie.Value)
	}

	// Clear cookie
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   httpx.ShouldSetSecureCookie(r, h.app.Config),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})

	response.OKMsg(w, r, i18n.MsgAuthLoggedOut)
}

// sessionView is the session body: the user's own fields at the top level, as the
// client has always read them, plus whether the MFA policy holds this session.
type sessionView struct {
	*auth.User
	MFAEnrollmentRequired bool `json:"mfaEnrollmentRequired,omitempty"`
}

// HandleSession returns the current authenticated user from context.
func (h *Handler) HandleSession(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	// A reload lands here rather than on the login body, so the flag has to be
	// recomputed or a held user would see the shell and a wall of 403s.
	owed := false
	if h.app.MFAEnrollmentOwed != nil {
		var err error
		owed, err = h.app.MFAEnrollmentOwed(r.Context(), user)
		if err != nil {
			h.app.Logger.Error("mfa policy lookup failed during session read", "userId", user.ID, "error", err)
			response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
			return
		}
	}

	response.OK(w, sessionView{User: user, MFAEnrollmentRequired: owed})
}

// HandleMyPermissions returns the effective permissions granted to the
// current authenticated user across every environment. The list is
// the union of the user's direct role assignments and the role
// assignments inherited via group membership. A permission set
// containing the wildcard "*" grants access to every action.
//
// The synthetic `system` user (id="system") is the implicit admin
// when auth is disabled (the middleware injects it on every request
// without a session). It has no database role assignments, so a naive
// query would return an empty permission set and the avatar menu's
// "Restart McHarbor" item would never appear. We short-circuit and
// return the wildcard for that user so the UI gates line up with
// the server-side rbac.RequirePermission checks (which also use the
// implicit-admin path for the synthetic user).
//
// The frontend uses this to gate UI affordances (e.g. the
// "Restart McHarbor" avatar-menu item) without making speculative
// requests; the server still enforces every check via the
// rbac.RequirePermission middleware on each endpoint.
func (h *Handler) HandleMyPermissions(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	// Synthetic admin user (id="system") gets full wildcard access.
	// This applies to AUTH_DISABLE mode (no real users) and to any
	// setup-time or dev convenience flow where the middleware injects
	// the system user on requests that lack a real session.
	if user.ID == "system" {
		response.OK(w, struct {
			Permissions []string `json:"permissions"`
			Wildcard    bool     `json:"wildcard"`
		}{
			Permissions: nil,
			Wildcard:    true,
		})
		return
	}

	perms, err := h.app.RBACService.EffectivePermissions(r.Context(), user.ID, "")
	if err != nil {
		h.app.Logger.Error("auth: my permissions lookup failed", "userId", user.ID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	// Surface the wildcard explicitly so the frontend can short-circuit
	// permission checks without listing every known permission.
	hasWildcard := false
	for _, p := range perms {
		if string(p) == "*" {
			hasWildcard = true
			break
		}
	}

	values := make([]string, len(perms))
	for i, p := range perms {
		values[i] = string(p)
	}

	response.OK(w, struct {
		Permissions []string `json:"permissions"`
		Wildcard    bool     `json:"wildcard"`
	}{
		Permissions: values,
		Wildcard:    hasWildcard,
	})
}

// HandlePermissionCatalog returns the canonical permission metadata.
func (h *Handler) HandlePermissionCatalog(w http.ResponseWriter, r *http.Request) {
	if user := auth.UserFromContext(r.Context()); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	response.OK(w, h.service.PermissionCatalog(r.Context()))
}

// HandleUpdatePreferences updates per-user preferences for the current
// authenticated user. The request may include any subset of
// {preferredLanguage, timeFormat, dateFormat, toast*}; absent fields are
// left unchanged. The auth service normalizes each value to a
// known set, so an unknown time format silently falls back to
// "24h" rather than failing the request.
func (h *Handler) HandleUpdatePreferences(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	var req preferencesRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if req.PreferredLanguage != nil && !isSupportedPreferredLanguage(*req.PreferredLanguage) {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	// A toast position the frontend never offers is rejected outright rather
	// than silently normalized: unlike a time format, there is no sane
	// fallback that is also what the caller asked for.
	if req.ToastPosition != nil && !coreSettings.ValidToastPosition(*req.ToastPosition) {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	// The appearance ids are closed sets the frontend renders from, so an
	// unknown one is a bug or a forged body, not something to normalize. The
	// base font size is the exception: it is a number on a stepper, and the
	// service clamps it into the supported range rather than rejecting a
	// stale client's off-by-one.
	if req.AppearanceThemeID != nil && !coreSettings.ValidAppearanceThemeID(*req.AppearanceThemeID) {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if req.AppearanceAccent != nil && !coreSettings.ValidAppearanceAccent(*req.AppearanceAccent) {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if req.AppearanceFont != nil && !coreSettings.ValidAppearanceFont(*req.AppearanceFont) {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	updated, err := h.app.AuthService.UpdatePreferencesContext(r.Context(), user.ID, auth.UserPreferences{
		PreferredLanguage:  req.PreferredLanguage,
		TimeFormat:         req.TimeFormat,
		DateFormat:         req.DateFormat,
		ClearToastOverride: req.ClearToastOverride,
		ToastPosition:      req.ToastPosition,
		ToastDurationMs:    req.ToastDurationMs,
		ToastMaxVisible:    req.ToastMaxVisible,
		ToastRichColors:    req.ToastRichColors,
		ToastCloseButton:   req.ToastCloseButton,

		ClearAppearanceOverride: req.ClearAppearanceOverride,
		AppearanceThemeID:       req.AppearanceThemeID,
		AppearanceAccent:        req.AppearanceAccent,
		AppearanceFont:          req.AppearanceFont,
		AppearanceBaseFontPx:    req.AppearanceBaseFontPx,
	})
	if err != nil {
		h.app.Logger.Error("auth: update preferences error", "error", err, "userId", user.ID)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if updated == nil {
		response.NotFoundCode(w, r, i18n.ErrUserNotFound)
		return
	}

	response.OK(w, updated)
}

// HandleUpdateProfile updates editable account details for the current authenticated user.
func (h *Handler) HandleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	var req profileRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	displayName := strings.TrimSpace(req.DisplayName)
	email := strings.TrimSpace(req.Email)
	if len(displayName) > 120 || len(email) > 254 {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if email != "" {
		parsed, err := mail.ParseAddress(email)
		if err != nil {
			response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
			return
		}
		email = parsed.Address
	}

	updated, err := h.app.AuthService.UpdateProfileContext(r.Context(), user.ID, displayName, email)
	if err != nil {
		// Identity-provider users cannot change displayName or
		// email from McHarbor — the IdP owns those fields. The
		// service returns a clear error so the frontend can show
		// the same message under the inputs.
		if user.IdentityProviderID != "" {
			response.BadRequestCode(w, r, i18n.ErrProfileIdentityProviderLocked)
			return
		}
		h.app.Logger.Error("auth: update profile error", "error", err, "userId", user.ID)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if updated == nil {
		response.NotFoundCode(w, r, i18n.ErrUserNotFound)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "update_profile",
		EntityType: "user",
		EntityID:   user.ID,
		EntityName: user.Username,
	})

	response.OK(w, updated)
}

func isSupportedPreferredLanguage(value string) bool {
	switch value {
	case "en", "nl", "de", "es", "fr", "pt":
		return true
	default:
		return false
	}
}

// HandleStatus returns public auth status (needsSetup, authDisabled, oidcProviders, defaultLanguage).
// This is a public endpoint — no auth required.
func (h *Handler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	providers, err := h.service.EnabledOIDCProviders(r.Context())
	if err != nil {
		h.app.Logger.Warn("auth: failed to load enabled identity providers", "error", err)
		providers = []OIDCProvider{}
	}

	response.OK(w, map[string]any{
		"needsSetup":      !h.app.AuthService.HasAnyUserContext(r.Context()),
		"authDisabled":    h.app.Config.AuthDisable,
		"oidcProviders":   providers,
		"defaultLanguage": h.app.AuthService.DefaultLanguage(r.Context()),
	})
}

// HandleSetup creates the initial admin user during first-run setup.
func (h *Handler) HandleSetup(w http.ResponseWriter, r *http.Request) {
	// Serialised with the database setup routes, and re-checked under the lock,
	// so two first-admin requests (or one racing a database apply) cannot both win.
	h.app.FirstRun.Lock()
	defer h.app.FirstRun.Unlock()
	if h.app.AuthService.HasAnyUserContext(r.Context()) {
		response.ConflictCode(w, r, i18n.ErrAuthSetupCompleted)
		return
	}
	if !h.app.FirstRun.Valid(r) {
		response.ErrCodeWithData(w, r, http.StatusForbidden, i18n.ErrForbidden, map[string]string{"reason": "setup_token"})
		return
	}

	var req setupRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if req.Username == "" || req.Password == "" {
		response.BadRequestCode(w, r, i18n.ErrAuthUsernameRequired)
		return
	}

	// The first admin is held to the same policy as everyone else.
	policy := auth.LoadPasswordPolicy(r.Context(), h.app.DB)
	if verr := auth.ValidatePassword(req.Password, policy); verr != nil {
		var pverr *auth.PasswordValidationError
		if errors.As(verr, &pverr) {
			response.BadRequestCode(w, r, pverr.Code())
			return
		}
		response.BadRequestCause(w, r, i18n.ErrPasswordTooWeak, verr)
		return
	}

	result, err := h.app.AuthService.RegisterContext(r.Context(), req.Username, req.Password, req.Email)
	if err != nil {
		h.app.Logger.Error("setup registration error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	if !result.Success {
		response.BadRequestCode(w, r, i18n.ErrAuthUsernameTaken)
		return
	}

	// Set session cookie so the user is logged in immediately
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookie,
		Value:    result.SessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   httpx.ShouldSetSecureCookie(r, h.app.Config),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(auth.SessionDuration.Seconds()),
	})

	h.app.AuditLog.LogWithUser(r, result.User.ID, result.User.Username, audit.Entry{
		Action:     "setup",
		EntityType: "user",
		EntityID:   result.User.ID,
		EntityName: result.User.Username,
	})

	response.Created(w, result.User)
}

// SessionPolicyResponse tells the browser how long it may sit idle
// before the server signs it out.
type SessionPolicyResponse struct {
	// IdleTimeoutSeconds is 0 when automatic sign-out is disabled.
	IdleTimeoutSeconds int `json:"idleTimeoutSeconds"`
}

// HandleSessionPolicy returns the session policy for the caller.
//
// Deliberately ungated: the value is not a secret (every signed-in user
// is subject to it), and gating it behind `settings.view` would leave
// non-admins with no way to know when their own session dies — they
// would simply find out from a 401 mid-task.
func (h *Handler) HandleSessionPolicy(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	response.OK(w, SessionPolicyResponse{
		IdleTimeoutSeconds: h.app.AuthService.SessionPolicyContext(r.Context()).IdleTimeoutSeconds,
	})
}

// HandleTouchSession resets the idle-timeout clock on the caller's own
// session. The browser calls this while the user is actually typing or
// clicking, at most once a minute.
//
// It is not an audit event and not a mutation of anything the user can
// see, so it neither logs nor takes a permission: the session id comes
// from the cookie, never from the request body, so there is no other
// session it could touch.
func (h *Handler) HandleTouchSession(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	// Auth-disabled installs have a synthetic user and no session row.
	// Report the policy honestly (nothing to touch, nothing expires)
	// rather than 401-ing a request the middleware just let through.
	cookie, err := r.Cookie(auth.SessionCookie)
	if err != nil || cookie.Value == "" {
		response.OK(w, SessionPolicyResponse{IdleTimeoutSeconds: 0})
		return
	}

	touched, err := h.app.AuthService.TouchSessionContext(r.Context(), cookie.Value)
	if err != nil {
		h.app.Logger.Error("auth: failed to touch session", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if !touched {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	response.OK(w, SessionPolicyResponse{
		IdleTimeoutSeconds: h.app.AuthService.SessionPolicyContext(r.Context()).IdleTimeoutSeconds,
	})
}

// writeFlatJSON encodes v as the response body after the status is committed, so an encode failure can only be logged, never turned into an error response.
func writeFlatJSON(w http.ResponseWriter, v any) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("auth: writing response body failed", "error", err)
	}
}
