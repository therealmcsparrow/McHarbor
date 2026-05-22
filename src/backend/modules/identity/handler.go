// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package identity

import (
	"database/sql"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/httpx"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for identity provider HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new identity handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{
		app:     app,
		service: NewService(app.DB, app.Encryption, app.AuthService),
	}
}

// HandleList returns all identity providers.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	providers, err := h.service.List()
	if err != nil {
		h.app.Logger.Error("failed to list identity providers", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrIdentityListFailed)
		return
	}
	response.OK(w, providers)
}

// HandleGet returns a single identity provider.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	provider, err := h.service.ByID(id)
	if err != nil {
		h.app.Logger.Error("failed to get identity provider", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrIdentityListFailed)
		return
	}
	if provider == nil {
		response.NotFoundCode(w, r, i18n.ErrIdentityNotFound)
		return
	}
	response.OK(w, provider)
}

// HandleCreate creates a new identity provider.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var input CreateProviderInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if input.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrIdentityNameRequired)
		return
	}
	if input.ClientID == "" {
		response.BadRequestCode(w, r, i18n.ErrIdentityClientRequired)
		return
	}
	if input.ClientSecret == "" {
		response.BadRequestCode(w, r, i18n.ErrIdentitySecretRequired)
		return
	}
	if input.ProviderType != "entra_id" && input.ProviderType != "google" {
		response.BadRequestCode(w, r, i18n.ErrIdentityInvalidType)
		return
	}
	if input.ProviderType == "entra_id" && (input.TenantID == nil || *input.TenantID == "") {
		response.BadRequestCode(w, r, i18n.ErrIdentityTenantRequired)
		return
	}

	provider, err := h.service.Create(input)
	if err != nil {
		h.app.Logger.Error("failed to create identity provider", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrIdentityCreateFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "create",
		EntityType: "identity_provider",
		EntityID:   provider.ID,
		EntityName: provider.Name,
	})

	response.Created(w, provider)
}

// HandleUpdate updates an identity provider.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input UpdateProviderInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	provider, err := h.service.Update(id, input)
	if err != nil {
		h.app.Logger.Error("failed to update identity provider", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrIdentityUpdateFailed)
		return
	}
	if provider == nil {
		response.NotFoundCode(w, r, i18n.ErrIdentityNotFound)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "update",
		EntityType: "identity_provider",
		EntityID:   provider.ID,
		EntityName: provider.Name,
	})

	response.OK(w, provider)
}

// HandleDelete removes an identity provider.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Get name for audit log before deleting
	provider, _ := h.service.ByID(id)

	err := h.service.Delete(id)
	if err == sql.ErrNoRows {
		response.NotFoundCode(w, r, i18n.ErrIdentityNotFound)
		return
	}
	if err != nil {
		h.app.Logger.Error("failed to delete identity provider", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrIdentityRemoveFailed)
		return
	}

	name := id
	if provider != nil {
		name = provider.Name
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "delete",
		EntityType: "identity_provider",
		EntityID:   id,
		EntityName: name,
	})

	response.OKMsg(w, r, i18n.MsgIdentityRemoved)
}

// HandleTest tests the connection to an identity provider.
func (h *Handler) HandleTest(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	err := h.service.TestConnection(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("identity provider test failed", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrIdentityTestFailed)
		return
	}

	response.OKMsg(w, r, i18n.MsgIdentityTestSuccess)
}

// HandleFetchGroups fetches groups from the provider's API.
func (h *Handler) HandleFetchGroups(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	groups, err := h.service.FetchProviderGroups(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("failed to fetch provider groups", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrIdentityFetchGroupsFailed)
		return
	}

	response.OK(w, groups)
}

// HandleEnabledProviders returns the list of enabled providers for the login page.
// This is a public endpoint — no auth required.
func (h *Handler) HandleEnabledProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := h.service.AllEnabled()
	if err != nil {
		h.app.Logger.Error("failed to list enabled providers", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrIdentityListFailed)
		return
	}
	response.OK(w, providers)
}

// HandleAuthorize initiates the OIDC flow by redirecting to the provider's auth page.
// This is a public endpoint — no auth required.
func (h *Handler) HandleAuthorize(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	baseURL := httpx.BaseURL(r)

	authURL, err := h.service.BuildAuthURL(id, baseURL)
	if err != nil {
		h.app.Logger.Error("failed to build auth URL", "error", err, "providerId", id)
		http.Redirect(w, r, "/login?error=oidc_failed", http.StatusFound)
		return
	}

	http.Redirect(w, r, authURL, http.StatusFound)
}

// HandleCallback processes the OIDC callback after provider authentication.
// This is a public endpoint — no auth required.
func (h *Handler) HandleCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	errorParam := r.URL.Query().Get("error")

	if errorParam != "" {
		h.app.Logger.Warn("OIDC callback error from provider", "error", errorParam,
			"description", r.URL.Query().Get("error_description"))
		http.Redirect(w, r, "/login?error=oidc_failed", http.StatusFound)
		return
	}

	if state == "" || code == "" {
		http.Redirect(w, r, "/login?error=oidc_failed", http.StatusFound)
		return
	}

	baseURL := httpx.BaseURL(r)

	sessionID, user, err := h.service.ExchangeAndProvision(r.Context(), state, code, baseURL)
	if err != nil {
		h.app.Logger.Error("OIDC exchange failed", "error", err)
		http.Redirect(w, r, "/login?error=oidc_failed", http.StatusFound)
		return
	}

	// Set session cookie
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
		Action:     "oidc_login",
		EntityType: "user",
		EntityID:   user.ID,
		EntityName: user.Username,
	})

	// Redirect to dashboard
	http.Redirect(w, r, "/", http.StatusFound)
}
