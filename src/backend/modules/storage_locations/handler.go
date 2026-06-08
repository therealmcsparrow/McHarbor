// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package storage_locations

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/oauth2"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for storage location handlers.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new storage location handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{app: app, service: NewService(app.DB, app.Encryption)}
}

var validLocationTypes = map[string]bool{
	"ftp": true, "ftps": true, "sftp": true, "samba": true, "aws": true,
	"google_drive": true, "onedrive_personal": true, "onedrive_business": true,
	"sharepoint": true,
}

var consentLocationTypes = map[string]bool{
	"google_drive":      true,
	"onedrive_personal": true,
	"onedrive_business": true,
	"sharepoint":        true,
}

var microsoftTenantPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

var validStorageAuthMethods = map[string]bool{
	"": true, "password": true, "private_key": true, "password_private_key": true,
}

var validStorageTLSModes = map[string]bool{
	"": true, "explicit": true, "implicit": true,
}

// HandleList returns all storage locations.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	items, err := h.service.List(r.Context())
	if err != nil {
		h.app.Logger.Error("storage locations: list error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrSettingsFailed)
		return
	}

	response.OK(w, items)
}

// HandleCreate creates a storage location.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	var input CreateStorageLocationInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if input.Name == "" || !validLocationTypes[input.LocationType] || !validStorageAuthMethods[input.AuthMethod] || !validStorageTLSModes[input.TLSMode] {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	item, err := h.service.Create(r.Context(), input)
	if err != nil {
		h.app.Logger.Error("storage locations: create error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrSettingsUpdateFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "storage_location.created",
		EntityType: "storage_location",
		EntityID:   item.ID,
		EntityName: item.Name,
	})

	response.Created(w, item)
}

// HandleGet returns a storage location.
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	id := chi.URLParam(r, "id")
	item, err := h.service.ByID(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("storage locations: get error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrSettingsFailed)
		return
	}
	if item == nil {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}

	response.OK(w, item)
}

// HandleUpdate updates a storage location.
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	id := chi.URLParam(r, "id")
	var input UpdateStorageLocationInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if input.LocationType != nil && !validLocationTypes[*input.LocationType] {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if input.AuthMethod != nil && !validStorageAuthMethods[*input.AuthMethod] {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	if input.TLSMode != nil && !validStorageTLSModes[*input.TLSMode] {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	item, err := h.service.Update(r.Context(), id, input)
	if err != nil {
		h.app.Logger.Error("storage locations: update error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrSettingsUpdateFailed)
		return
	}
	if item == nil {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "storage_location.updated",
		EntityType: "storage_location",
		EntityID:   item.ID,
		EntityName: item.Name,
	})

	response.OK(w, item)
}

// HandleDelete removes a storage location.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	id := chi.URLParam(r, "id")
	deleted, err := h.service.Delete(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("storage locations: delete error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrSettingsUpdateFailed)
		return
	}
	if !deleted {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "storage_location.deleted",
		EntityType: "storage_location",
		EntityID:   id,
	})

	response.NoContent(w)
}

// HandleOAuthAuthorize creates a delegated provider consent URL.
func (h *Handler) HandleOAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	id := chi.URLParam(r, "id")
	creds, err := h.service.OAuthCredentials(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("storage locations: oauth credentials error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrSettingsFailed)
		return
	}
	if creds == nil {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	if !storageOAuthCredentialsReady(creds) {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	state, err := h.service.CreateOAuthState(r.Context(), id, creds.LocationType)
	if err != nil {
		h.app.Logger.Error("storage locations: oauth state error", "error", err, "id", id)
		response.InternalErrorCode(w, r, i18n.ErrSettingsUpdateFailed)
		return
	}

	cfg := storageOAuthConfig(r, creds)
	authURL := cfg.AuthCodeURL(state.State, storageOAuthOptions(creds.LocationType)...)
	response.OK(w, OAuthAuthorizeResponse{AuthorizationURL: authURL, ExpiresAt: state.ExpiresAt})
}

// HandleOAuthCallback stores delegated provider tokens after consent.
func (h *Handler) HandleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	stateValue := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if stateValue == "" || code == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	state, err := h.service.OAuthState(r.Context(), stateValue)
	if err != nil {
		h.app.Logger.Error("storage locations: oauth state lookup error", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrSettingsFailed)
		return
	}
	if state == nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	defer func() {
		if err := h.service.DeleteOAuthState(context.Background(), stateValue); err != nil {
			h.app.Logger.Error("storage locations: oauth state cleanup error", "error", err)
		}
	}()

	creds, err := h.service.OAuthCredentials(r.Context(), state.LocationID)
	if err != nil {
		h.app.Logger.Error("storage locations: oauth credentials callback error", "error", err, "id", state.LocationID)
		response.InternalErrorCode(w, r, i18n.ErrSettingsFailed)
		return
	}
	if creds == nil || creds.LocationType != state.Provider || !storageOAuthCredentialsReady(creds) {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	exchangeCtx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	cfg := storageOAuthConfig(r, creds)
	token, err := cfg.Exchange(exchangeCtx, code)
	if err != nil {
		h.app.Logger.Error("storage locations: oauth token exchange error", "error", err, "provider", creds.LocationType)
		response.InternalErrorCode(w, r, i18n.ErrSettingsUpdateFailed)
		return
	}
	if err := h.service.StoreOAuthToken(r.Context(), state.LocationID, token.AccessToken, token.RefreshToken); err != nil {
		h.app.Logger.Error("storage locations: oauth token store error", "error", err, "id", state.LocationID)
		response.InternalErrorCode(w, r, i18n.ErrSettingsUpdateFailed)
		return
	}

	http.Redirect(w, r, "/settings?tab=storage&storageOAuth=success", http.StatusFound)
}

func storageOAuthConfig(r *http.Request, creds *storageOAuthCredentials) oauth2.Config {
	return oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		RedirectURL:  publicBaseURL(r) + "/api/storage-locations/oauth/callback",
		Scopes:       storageOAuthScopes(creds.LocationType),
		Endpoint:     storageOAuthEndpoint(creds),
	}
}

func storageOAuthCredentialsReady(creds *storageOAuthCredentials) bool {
	if creds == nil || !consentLocationTypes[creds.LocationType] || creds.ClientID == "" || creds.ClientSecret == "" {
		return false
	}
	if creds.LocationType == "onedrive_business" || creds.LocationType == "sharepoint" {
		return creds.TenantID != "" && microsoftTenantPattern.MatchString(creds.TenantID)
	}
	return true
}

func storageOAuthScopes(locationType string) []string {
	switch locationType {
	case "google_drive":
		return []string{"https://www.googleapis.com/auth/drive.file"}
	case "onedrive_personal":
		return []string{"offline_access", "Files.ReadWrite", "User.Read"}
	case "onedrive_business":
		return []string{"offline_access", "Files.ReadWrite.All", "User.Read"}
	case "sharepoint":
		return []string{"offline_access", "Files.ReadWrite.All", "Sites.ReadWrite.All", "User.Read"}
	default:
		return nil
	}
}

func storageOAuthEndpoint(creds *storageOAuthCredentials) oauth2.Endpoint {
	switch creds.LocationType {
	case "google_drive":
		return oauth2.Endpoint{
			AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
		}
	case "onedrive_personal":
		return oauth2.Endpoint{
			AuthURL:  "https://login.microsoftonline.com/consumers/oauth2/v2.0/authorize",
			TokenURL: "https://login.microsoftonline.com/consumers/oauth2/v2.0/token",
		}
	case "onedrive_business", "sharepoint":
		tenant := strings.TrimSpace(creds.TenantID)
		return oauth2.Endpoint{
			AuthURL:  "https://login.microsoftonline.com/" + tenant + "/oauth2/v2.0/authorize",
			TokenURL: "https://login.microsoftonline.com/" + tenant + "/oauth2/v2.0/token",
		}
	default:
		return oauth2.Endpoint{}
	}
}

func storageOAuthOptions(locationType string) []oauth2.AuthCodeOption {
	switch locationType {
	case "google_drive":
		return []oauth2.AuthCodeOption{
			oauth2.AccessTypeOffline,
			oauth2.SetAuthURLParam("prompt", "consent"),
			oauth2.SetAuthURLParam("include_granted_scopes", "true"),
		}
	case "onedrive_personal":
		return []oauth2.AuthCodeOption{
			oauth2.SetAuthURLParam("prompt", "select_account"),
		}
	case "onedrive_business", "sharepoint":
		return []oauth2.AuthCodeOption{
			oauth2.SetAuthURLParam("prompt", "select_account"),
		}
	default:
		return nil
	}
}

func publicBaseURL(r *http.Request) string {
	scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	return scheme + "://" + host
}
