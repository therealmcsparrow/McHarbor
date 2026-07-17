// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

import (
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
)

// Handler holds dependencies for auth HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new auth handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{
		app:     app,
		service: NewService(app.DB),
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
}

type profileRequest struct {
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
}

// HandleLogin authenticates a user and sets a session cookie.
func (h *Handler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if req.Username == "" || req.Password == "" {
		response.BadRequestCode(w, r, i18n.ErrAuthUsernameRequired)
		return
	}

	result, err := h.app.AuthService.Login(req.Username, req.Password)
	if err != nil {
		h.app.Logger.Error("login error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	if !result.Success {
		response.UnauthorizedCode(w, r, i18n.ErrAuthInvalidCredentials)
		return
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
	})

	response.OK(w, result.User)
}

// HandleLogout destroys the session cookie.
func (h *Handler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if user := auth.UserFromContext(r.Context()); user != nil {
		h.app.AuditLog.Log(r, audit.Entry{
			Action:     "logout",
			EntityType: "user",
			EntityID:   user.ID,
			EntityName: user.Username,
		})
	}

	cookie, err := r.Cookie(auth.SessionCookie)
	if err == nil && cookie.Value != "" {
		h.app.AuthService.DestroySession(cookie.Value)
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

// HandleSession returns the current authenticated user from context.
func (h *Handler) HandleSession(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	response.OK(w, user)
}

// HandleMyPermissions returns the effective permissions granted to the
// current authenticated user across every environment. The list is
// the union of the user's direct role assignments and the role
// assignments inherited via group membership. A permission set
// containing the wildcard "*" grants access to every action.
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

	perms, err := h.app.RBACService.EffectivePermissions(user.ID, "")
	if err != nil {
		h.app.Logger.Error("auth: my permissions lookup failed", "userId", user.ID, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
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

// HandleUpdatePreferences updates per-user preferences for the current
// authenticated user. The request may include any subset of
// {preferredLanguage, timeFormat, dateFormat}; absent fields are
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
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if req.PreferredLanguage != nil && !isSupportedPreferredLanguage(*req.PreferredLanguage) {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	updated, err := h.app.AuthService.UpdatePreferences(user.ID, auth.UserPreferences{
		PreferredLanguage: req.PreferredLanguage,
		TimeFormat:        req.TimeFormat,
		DateFormat:        req.DateFormat,
	})
	if err != nil {
		h.app.Logger.Error("auth: update preferences error", "error", err, "userId", user.ID)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
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
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
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
			response.BadRequestCode(w, r, i18n.ErrInvalidBody)
			return
		}
		email = parsed.Address
	}

	updated, err := h.app.AuthService.UpdateProfile(user.ID, displayName, email)
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
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
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
	providers, err := h.service.EnabledOIDCProviders()
	if err != nil {
		h.app.Logger.Warn("auth: failed to load enabled identity providers", "error", err)
		providers = []OIDCProvider{}
	}

	response.OK(w, map[string]any{
		"needsSetup":      !h.app.AuthService.HasAnyUser(),
		"authDisabled":    h.app.Config.AuthDisable,
		"oidcProviders":   providers,
		"defaultLanguage": h.app.AuthService.DefaultLanguage(r.Context()),
	})
}

// HandleSetup creates the initial admin user during first-run setup.
func (h *Handler) HandleSetup(w http.ResponseWriter, r *http.Request) {
	// Only allow setup if no users exist yet
	if h.app.AuthService.HasAnyUser() {
		response.ConflictCode(w, r, i18n.ErrAuthSetupCompleted)
		return
	}

	var req setupRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if req.Username == "" || req.Password == "" {
		response.BadRequestCode(w, r, i18n.ErrAuthUsernameRequired)
		return
	}

	if len(req.Password) < 8 {
		response.BadRequestCode(w, r, i18n.ErrAuthPasswordShort)
		return
	}

	result, err := h.app.AuthService.Register(req.Username, req.Password, req.Email)
	if err != nil {
		h.app.Logger.Error("setup registration error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}

	if !result.Success {
		response.BadRequestCode(w, r, i18n.ErrAuthUsernameTaken)
		return
	}

	localEnv, err := h.service.EnsureLocalEnvironment()
	if err != nil {
		h.app.Logger.Warn("auth: failed to auto-create local environment", "error", err)
	} else if localEnv != nil && localEnv.Created {
		h.app.Logger.Info("auto-created local environment", "id", localEnv.ID, "socket", localEnv.SocketPath, "runtime", localEnv.Runtime)
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
