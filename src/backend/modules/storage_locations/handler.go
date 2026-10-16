// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package storage_locations

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for storage location handlers.
type Handler struct {
	app            *router.AppDeps
	service        *Service
	backupMigrator BackupMigrator
}

// NewHandler creates a new storage location handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{
		app:            app,
		service:        NewService(app.DB, app.Encryption),
		backupMigrator: nil, // injected via SetBackupMigrator from main.go
	}
}

// SetBackupMigrator registers the runtime adapter that fulfils the
// container-backup migration flow when a non-local storage location
// is deleted. Without this the migration endpoint returns 503.
func (h *Handler) SetBackupMigrator(m BackupMigrator) {
	h.backupMigrator = m
}

// SetPluginRuntime injects the loaded plugin runtime so the Test
// endpoint can dispatch to plugin-owned LocationTypes (any
// cloud-drive type contributed by a storage plugin). Called from
// main.go via MountWithRuntime after bootPluginRuntime finishes.
func (h *Handler) SetPluginRuntime(runtime any) {
	h.service.SetPluginRuntime(runtime)
}

// lookupStorageAdapterOAuth is a Handler-level pass-through to
// Service.lookupStorageAdapterOAuth. Used by HandleOAuthAuthorize /
// HandleOAuthCallback to detect plugin-owned LocationTypes (e.g.
// google_drive) and delegate the consent URL + token exchange to
// the plugin instead of falling through to the host's built-in
// (which only knows Microsoft-family scopes).
func (h *Handler) lookupStorageAdapterOAuth(locationType string) (pluginStorageOAuthClient, bool) {
	return h.service.lookupStorageAdapterOAuth(locationType)
}

// storeOAuthTokensFromPluginUpdates is a Handler-level pass-
// through to Service.StoreOAuthTokensFromPluginUpdates.
func (h *Handler) storeOAuthTokensFromPluginUpdates(ctx context.Context, locationID string, updates map[string]string) error {
	return h.service.StoreOAuthTokensFromPluginUpdates(ctx, locationID, updates)
}

// Location types are no longer allow-listed here. "local" is the only
// built-in; every other type is owned by a storage plugin and is
// accepted when that plugin has registered an adapter for it — see
// Service.SupportsLocationType.

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
		response.InternalErrorCause(w, r, i18n.ErrSettingsFailed, err)
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
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if input.Name == "" || !h.service.SupportsLocationType(input.LocationType) || !validStorageAuthMethods[input.AuthMethod] || !validStorageTLSModes[input.TLSMode] {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	item, err := h.service.Create(r.Context(), input)
	if err != nil {
		h.app.Logger.Error("storage locations: create error", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrSettingsUpdateFailed, err)
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
		response.InternalErrorCause(w, r, i18n.ErrSettingsFailed, err)
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
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if input.LocationType != nil && !h.service.SupportsLocationType(*input.LocationType) {
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
		if errors.Is(err, ErrLocalStorageProtected) {
			response.ConflictCode(w, r, i18n.ErrStorageLocationProtected)
			return
		}
		h.app.Logger.Error("storage locations: update error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrSettingsUpdateFailed, err)
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

// auditLocationName resolves a storage location id to its name for the audit
// trail, empty when the lookup fails: a deleted location that appears in the
// log only as an xid is what this is here to prevent.
func (h *Handler) auditLocationName(ctx context.Context, id string) string {
	location, err := h.service.ByID(ctx, id)
	if err != nil || location == nil {
		return ""
	}
	return location.Name
}

// HandleDelete removes a storage location.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	id := chi.URLParam(r, "id")
	// Read before the delete: afterwards there is no row left to name.
	name := h.auditLocationName(r.Context(), id)
	deleted, err := h.service.Delete(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrLocalStorageProtected) {
			response.ConflictCode(w, r, i18n.ErrStorageLocationProtected)
			return
		}
		h.app.Logger.Error("storage locations: delete error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrSettingsUpdateFailed, err)
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
		EntityName: name,
	})

	response.NoContent(w)
}

// HandleMigrateContainerBackups copies existing completed backups to a local storage location.
func (h *Handler) HandleMigrateContainerBackups(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	id := chi.URLParam(r, "id")
	migrateCtx, cancel := context.WithTimeout(r.Context(), 2*time.Hour)
	defer cancel()

	result, err := h.backupMigrator.MigrateCompletedRunsToLocalStorage(migrateCtx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.NotFoundCode(w, r, i18n.ErrNotFound)
			return
		}
		if errors.Is(err, ErrBackupMigrationStorageNotLocal) || errors.Is(err, ErrBackupMigrationStorageDisabled) {
			response.BadRequestCode(w, r, i18n.ErrInvalidBody)
			return
		}
		h.app.Logger.Error("storage locations: backup migration error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrSettingsUpdateFailed, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "storage_location.container_backups_migrated",
		EntityType: "storage_location",
		EntityID:   id,
		EntityName: h.auditLocationName(r.Context(), id),
	})

	response.OK(w, result)
}

// HandleGetSecrets returns the decrypted credential columns for a
// single storage location. The endpoint is permission-gated by
// the route handler (settings.manage) so only operators who can
// edit the destination can see the secrets. The plugin's
// ConfigForm relies on this to populate fields on edit — the
// GET /api/storage-locations/{id} response deliberately omits
// credentials by design, so without this dedicated endpoint the
// operator would have to re-enter every credential every time
// they edited a working destination.
func (h *Handler) HandleGetSecrets(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	id := chi.URLParam(r, "id")
	secrets, err := h.service.DecryptedSecrets(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("storage locations: get secrets error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if secrets == nil {
		response.NotFoundCode(w, r, i18n.ErrNotFound)
		return
	}
	response.OK(w, secrets)
}

// HandleOAuthAuthorize creates a delegated provider consent URL.
// When the location_type is owned by a plugin (a cloud-drive type
// contributed by a storage plugin), the plugin's ConsentURL
// is consulted first — the plugin owns the redirect target,
// scopes, and prompt options. Otherwise the host's built-in
// Microsoft-family OAuth config takes over.
func (h *Handler) HandleOAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	id := chi.URLParam(r, "id")
	creds, err := h.service.OAuthCredentials(r.Context(), id)
	if err != nil {
		h.app.Logger.Error("storage locations: oauth credentials error", "error", err, "id", id)
		response.InternalErrorCause(w, r, i18n.ErrSettingsFailed, err)
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
		response.InternalErrorCause(w, r, i18n.ErrSettingsUpdateFailed, err)
		return
	}

	if adapter, ok := h.lookupStorageAdapterOAuth(creds.LocationType); ok {
		cfg := h.buildOAuthConfigForPlugin(creds)
		authURL, err := adapter.ConsentURL(r.Context(), id, creds.LocationType, publicBaseURL(r), state.State, cfg)
		if err != nil {
			h.app.Logger.Error("storage locations: plugin oauth authorize error", "error", err, "locationType", creds.LocationType)
			response.InternalErrorCause(w, r, i18n.ErrSettingsUpdateFailed, err)
			return
		}
		response.OK(w, OAuthAuthorizeResponse{AuthorizationURL: authURL, ExpiresAt: state.ExpiresAt})
		return
	}

	// No plugin adapter registered for this location type. The host
	// no longer ships any built-in OAuth flow — refuse cleanly so the
	// operator sees a real error instead of a 500 from a partial
	// redirect.
	h.app.Logger.Error("storage locations: no OAuth adapter for location type",
		"locationType", creds.LocationType, "id", id)
	response.InternalErrorCode(w, r, i18n.ErrSettingsUpdateFailed)
}

// buildOAuthConfigForPlugin assembles the cfg map handed to the
// plugin's ConsentURL. Includes location_type, name, the
// decrypted client_id / client_secret / refresh_token (when
// present), and base_path — everything a plugin needs to build
// its authorize URL.
func (h *Handler) buildOAuthConfigForPlugin(creds *storageOAuthCredentials) map[string]string {
	cfg := make(map[string]string, len(creds.Config)+4)
	for k, v := range creds.Config {
		if v != "" {
			cfg[k] = v
		}
	}
	cfg["location_type"] = creds.LocationType
	if creds.TenantID != "" {
		cfg["tenant_id"] = creds.TenantID
	}
	if creds.ClientID != "" {
		cfg["client_id"] = creds.ClientID
	}
	if creds.ClientSecret != "" {
		cfg["client_secret"] = creds.ClientSecret
	}
	return cfg
}

// HandleTestConnection exercises write / change / delete against
// the configured storage location and returns a per-step report.
// For local storage the full cycle runs synchronously. For
// cloud / network storage the endpoint is reached at the TCP
// level so DNS, firewall, and credential-config errors surface
// here instead of in the next scheduled backup run. The full
// write/change/delete cycle for cloud providers is exercised by
// the next scheduled backup run (the heavy SDKs live in the
// container_backups module; adding them to the self-test path
// would inflate the storage_locations dependency tree for
// no real benefit).
func (h *Handler) HandleTestConnection(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")
	result, err := h.service.TestStorageLocation(r.Context(), id)
	if err != nil {
		h.auditTest(r, id, nil, "fail")
		if errors.Is(err, sql.ErrNoRows) {
			response.NotFoundCode(w, r, i18n.ErrNotFound)
			return
		}
		h.app.Logger.Error("storage location connection test failed", "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	h.auditTest(r, id, result, result.OverallStatus)
	response.OK(w, result)
}

// HandleUsage returns used / total capacity for one storage
// location so the settings card can render a usage bar. Read-only;
// gated by settings.view in routes.go. Unsupported destinations
// answer 200 with supported=false.
func (h *Handler) HandleUsage(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	id := chi.URLParam(r, "id")
	usage, err := h.service.Usage(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.NotFoundCode(w, r, i18n.ErrNotFound)
			return
		}
		h.app.Logger.Error("storage location usage failed", "id", id, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrSettingsFailed, err)
		return
	}
	response.OK(w, usage)
}

func (h *Handler) auditTest(r *http.Request, id string, result *TestStorageLocationResult, status string) {
	entityName := id
	locationType := ""
	if result != nil {
		entityName = result.LocationName
		locationType = result.LocationType
	}
	details, err := json.Marshal(map[string]string{"status": status, "locationType": locationType})
	if err != nil {
		h.app.Logger.Error("storage location audit details failed", "error", err, "id", id)
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "storage_location.tested",
		Severity:   auditStatusSeverity(status),
		EntityType: "storage_location",
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

// HandleOAuthCallback stores delegated provider tokens after consent.
// Plugin-owned LocationTypes delegate the token-exchange round
// trip to the plugin's ConsentCallback; otherwise the host's
// built-in OAuth2 config handles the exchange.
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
		response.InternalErrorCause(w, r, i18n.ErrSettingsFailed, err)
		return
	}
	if state == nil {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	defer func(detached context.Context) {
		// Detached on purpose: this runs after the response, when r.Context()
		// is already cancelled, and the one-shot state row has to go away
		// whether the exchange succeeded or not. Bounded so it cannot hang.
		cleanupCtx, cancel := context.WithTimeout(detached, 10*time.Second)
		defer cancel()
		if err := h.service.DeleteOAuthState(cleanupCtx, stateValue); err != nil {
			h.app.Logger.Error("storage locations: oauth state cleanup error", "error", err)
		}
	}(context.WithoutCancel(r.Context()))

	creds, err := h.service.OAuthCredentials(r.Context(), state.LocationID)
	if err != nil {
		h.app.Logger.Error("storage locations: oauth credentials callback error", "error", err, "id", state.LocationID)
		response.InternalErrorCause(w, r, i18n.ErrSettingsFailed, err)
		return
	}
	if creds == nil || creds.LocationType != state.Provider || !storageOAuthCredentialsReady(creds) {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	if adapter, ok := h.lookupStorageAdapterOAuth(creds.LocationType); ok {
		exchangeCtx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		cfg := h.buildOAuthConfigForPlugin(creds)
		updates, err := adapter.ConsentCallback(exchangeCtx, state.LocationID, creds.LocationType, publicBaseURL(r), code, stateValue, cfg)
		if err != nil {
			h.app.Logger.Error("storage locations: plugin oauth callback error", "error", err, "locationType", creds.LocationType)
			response.InternalErrorCause(w, r, i18n.ErrSettingsUpdateFailed, err)
			return
		}
		if err := h.storeOAuthTokensFromPluginUpdates(r.Context(), state.LocationID, updates); err != nil {
			h.app.Logger.Error("storage locations: plugin oauth token store error", "error", err, "id", state.LocationID)
			response.InternalErrorCause(w, r, i18n.ErrSettingsUpdateFailed, err)
			return
		}
		http.Redirect(w, r, "/settings?tab=storage&storageOAuth=success", http.StatusFound)
		return
	}

	// No plugin adapter registered for this location type. The host
	// no longer ships any built-in OAuth flow — refuse cleanly so the
	// operator sees a real error instead of a 500 from a partial
	// redirect.
	h.app.Logger.Error("storage locations: no OAuth adapter for location type",
		"locationType", creds.LocationType, "id", state.LocationID)
	http.Redirect(w, r, "/settings?tab=storage&storageOAuth=failed", http.StatusFound)
}

func storageOAuthCredentialsReady(creds *storageOAuthCredentials) bool {
	// The plugin runtime owns the OAuth handshake for every non-local
	// storage type. The host only needs to confirm that client_id +
	// client_secret are present so the plugin's ConsentURL has
	// something to render against.
	if creds == nil || creds.ClientID == "" || creds.ClientSecret == "" {
		return false
	}
	return true
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
