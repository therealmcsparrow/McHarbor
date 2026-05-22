// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

import (
	"net/http"
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

// HandleStatus returns public auth status (needsSetup, authDisabled, oidcProviders).
// This is a public endpoint — no auth required.
func (h *Handler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	providers, err := h.service.EnabledOIDCProviders()
	if err != nil {
		h.app.Logger.Warn("auth: failed to load enabled identity providers", "error", err)
		providers = []OIDCProvider{}
	}

	response.OK(w, map[string]any{
		"needsSetup":    !h.app.AuthService.HasAnyUser(),
		"authDisabled":  h.app.Config.AuthDisable,
		"oidcProviders": providers,
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
