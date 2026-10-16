// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package identity

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/httpx"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/middleware"
	"github.com/therealmcsparrow/mcharbor/core/pluginruntime"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for identity provider HTTP handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new identity handler using whatever plugin
// runtime is currently on app.PluginAdapters. Callers that mount
// before bootPluginRuntime has run must use NewHandlerWithRuntime
// instead — see MountWithRuntime.
func NewHandler(app *router.AppDeps) *Handler {
	return NewHandlerWithRuntime(app, app.PluginAdapters)
}

// NewHandlerWithRuntime creates a new identity handler with the plugin
// runtime injected explicitly. The runtime is what lets the service
// dispatch per-provider-type work to the matching plugin's
// IdentityAdapter implementation; with a nil runtime every provider
// type is unknown, so Create returns ErrIdentityInvalidType and the
// whole login flow rejects every provider.
func NewHandlerWithRuntime(app *router.AppDeps, adapters any) *Handler {
	var runtime *pluginruntime.Runtime
	if rt, ok := adapters.(*pluginruntime.Runtime); ok {
		runtime = rt
	}
	return &Handler{
		app:     app,
		service: NewService(app.DB, app.Encryption, app.AuthService, runtime),
	}
}

// HandleList returns all identity providers.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	providers, err := h.service.List(r.Context())
	if err != nil {
		h.app.Logger.Error("failed to list identity providers", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrIdentityListFailed, err)
		return
	}
	response.OK(w, providers)
}

// HandleGet returns a single identity provider.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	provider, err := h.service.ByID(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("failed to get identity provider", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrIdentityListFailed, err)
		return
	}
	if provider == nil {
		response.NotFoundCode(w, r, i18n.ErrIdentityNotFound)
		return
	}
	response.OK(w, provider)
}

// HandleCreate creates a new identity provider. Protocol-specific
// field validation lives in the plugin's BeginAuth/Test; the host
// only validates that the plugin runtime recognizes the requested
// provider_type (i.e. the user installed the matching plugin).
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var input CreateProviderInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	if input.Name == "" {
		response.BadRequestCode(w, r, i18n.ErrIdentityNameRequired)
		return
	}
	if _, _, ok := h.service.identityAdapter(input.ProviderType); !ok {
		response.BadRequestCode(w, r, i18n.ErrIdentityInvalidType)
		return
	}

	provider, err := h.service.Create(r.Context(), input)
	if err != nil {
		h.app.Logger.Error("failed to create identity provider", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrIdentityCreateFailed, err)
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
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	provider, err := h.service.Update(r.Context(), id, input)
	if errors.Is(err, ErrIdentityAdapterUnavailable) {
		h.app.Logger.Warn("identity provider config change refused: adapter not loaded", "id", id)
		response.BadRequestCode(w, r, i18n.ErrIdentityInvalidType)
		return
	}
	if err != nil {
		h.app.Logger.Error("failed to update identity provider", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrIdentityUpdateFailed, err)
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
	provider, lookupErr := h.service.ByID(r.Context(), id)
	if lookupErr != nil {
		h.app.Logger.Warn("identity: reading provider name for the audit row", "error", lookupErr, "id", id)
	}

	err := h.service.Delete(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		response.NotFoundCode(w, r, i18n.ErrIdentityNotFound)
		return
	}
	if err != nil {
		h.app.Logger.Error("failed to delete identity provider", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrIdentityRemoveFailed, err)
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

// HandleTest runs the per-step connection test against an identity
// provider and returns the full report.
//
// A failed *test* is a successful *request*: the response is 200 with
// OverallStatus "fail" so the modal can show which phase broke. Only a
// missing row (404) or an internal fault (500) is a non-200. Mirrors
// storage_locations and ai_providers.
func (h *Handler) HandleTest(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	provider, lookupErr := h.service.ByID(r.Context(), id)
	if lookupErr != nil {
		h.app.Logger.Error("identity provider audit lookup failed", "error", lookupErr, "id", id)
	}
	if provider == nil && lookupErr == nil {
		response.NotFoundCode(w, r, i18n.ErrIdentityNotFound)
		return
	}

	result, err := h.service.TestConnectionSteps(r.Context(), id)
	if err != nil {
		h.auditTest(r, id, provider, "fail")
		h.app.Logger.Error("identity provider test failed", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrIdentityTestFailed, err)
		return
	}

	h.auditTest(r, id, provider, result.OverallStatus)
	response.OK(w, result)
}

func (h *Handler) auditTest(r *http.Request, id string, provider *Provider, status string) {
	entityName := id
	providerType := ""
	if provider != nil {
		entityName = provider.Name
		providerType = provider.ProviderType
	}
	details, err := json.Marshal(map[string]string{"status": status, "providerType": providerType})
	if err != nil {
		h.app.Logger.Error("identity provider audit details failed", "error", err, "id", id)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "identity_provider.tested",
		Severity:   auditStatusSeverity(status),
		EntityType: "identity_provider",
		EntityID:   id,
		EntityName: entityName,
		Details:    string(details),
	})
}

func auditStatusSeverity(status string) string {
	if status == "fail" {
		return "error"
	}
	return "success"
}

// HandleFetchGroups fetches groups from the provider's API.
func (h *Handler) HandleFetchGroups(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	groups, err := h.service.FetchProviderGroups(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("failed to fetch provider groups", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrIdentityFetchGroupsFailed, err)
		return
	}

	response.OK(w, groups)
}

// HandleEnabledProviders returns the list of enabled providers for the login page.
// This is a public endpoint — no auth required.
func (h *Handler) HandleEnabledProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := h.service.AllEnabled(r.Context())
	if err != nil {
		h.app.Logger.Error("failed to list enabled providers", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrIdentityListFailed, err)
		return
	}
	response.OK(w, providers)
}

// samlPostBindingCSP replaces the host policy on the SAML POST-binding page only: that page
// submits itself from an inline onload handler to the IdP's own origin, which script-src
// 'self' and form-action 'self' would both block. Nothing else is served under it.
const samlPostBindingCSP = "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; form-action *; frame-ancestors 'none'; base-uri 'none'"

// HandleAuthorize initiates the OIDC/SAML flow by redirecting to the
// provider's auth page. The plugin's BeginAuth returns either a
// redirect URL (OIDC + SAML redirect binding) or an auto-submitting
// HTML form (SAML POST binding); the host renders one or the other.
// This is a public endpoint — no auth required.
func (h *Handler) HandleAuthorize(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	baseURL := httpx.BaseURL(r)

	result, err := h.service.BeginAuth(r.Context(), id, baseURL)
	if err != nil {
		h.app.Logger.Error("failed to build auth URL", "error", err, "providerId", id)
		http.Redirect(w, r, "/login?error=oidc_failed", http.StatusFound)
		return
	}

	if result.HTMLForm != "" {
		middleware.ReplaceContentSecurityPolicy(w.Header(), samlPostBindingCSP)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, writeErr := w.Write([]byte(result.HTMLForm)); writeErr != nil {
			h.app.Logger.Error("failed to write auth form", "error", writeErr, "providerId", id)
		}
		return
	}

	http.Redirect(w, r, result.RedirectURL, http.StatusFound)
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

// HandleSAMLMetadata returns service-provider metadata for a configured SAML provider.
// This is a public endpoint — no auth required.
func (h *Handler) HandleSAMLMetadata(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	baseURL := httpx.BaseURL(r)

	metadataXML, err := h.service.SAMLMetadata(r.Context(), id, baseURL)
	if err != nil {
		h.app.Logger.Error("failed to generate saml metadata", "error", err, "providerId", id)
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/samlmetadata+xml")
	if _, writeErr := w.Write(metadataXML); writeErr != nil {
		h.app.Logger.Error("failed to write saml metadata", "error", writeErr, "providerId", id)
	}
}

// HandleSAMLACS processes a SAML assertion consumer service callback.
// This is a public endpoint — no auth required.
func (h *Handler) HandleSAMLACS(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	baseURL := httpx.BaseURL(r)

	sessionID, user, err := h.service.HandleSAMLACS(r.Context(), id, baseURL, r)
	if err != nil {
		h.app.Logger.Error("saml acs failed", "error", err, "providerId", id)
		http.Redirect(w, r, "/login?error=oidc_failed", http.StatusFound)
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
		Action:     "saml_login",
		EntityType: "user",
		EntityID:   user.ID,
		EntityName: user.Username,
	})

	http.Redirect(w, r, "/", http.StatusFound)
}
