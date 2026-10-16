// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package identity

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/pluginruntime"
	"github.com/therealmcsparrow/mcharbor/core/secrets"
)

const stateExpiry = 10 * time.Minute

// authFlowResult is the result of BeginAuth. The host's HTTP handler
// translates it into either a 302 (RedirectURL) or an auto-submitting
// HTML response (HTMLForm). Mirrors plugin.IdentityAuthResult one-to-
// one so callers don't need to know about the plugin SDK.
type authFlowResult struct {
	RedirectURL string
	HTMLForm    string
}

// externalUserInfo is what the host hands to findOrProvisionUser.
// Mirrors plugin.IdentityCallbackResult but with the host-side
// UsernamePrefix convention.
type externalUserInfo struct {
	Subject        string
	Email          string
	Name           string
	Groups         []string
	UsernamePrefix string
	// PreferredUsername is the login name the user typed, used (prefixed) for a
	// new account when the provider has no email; a DN makes a poor username.
	PreferredUsername string
}

// runtimeProvider is the host's typed-column view of one row in
// identity_providers. The host's CRUD surface reads/writes this;
// per-provider-type runtime work is delegated to the plugin that
// registered the matching ProviderType via RegisterAdapters.
type runtimeProvider struct {
	ID                    string
	Name                  string
	ProviderType          string
	ClientID              string
	EncryptedClientSecret string
	TenantID              sql.NullString
	Domain                sql.NullString
	IssuerURL             sql.NullString
	MetadataURL           sql.NullString
	EntityID              sql.NullString
	EncryptedSPCert       sql.NullString
	EncryptedSPKey        sql.NullString
	Scopes                string
	Enabled               bool
	AutoProvision         bool
	DefaultRoleID         sql.NullString
	GroupMappingEnabled   bool
	GroupMappingsJSON     string
	AutoImportGroups      bool

	// ConfigJSON is the raw identity_providers.config column: the
	// plugin-owned field map, secrets still encrypted. Decoded and
	// decrypted in providerToConfigMap.
	ConfigJSON sql.NullString
}

// Service handles identity provider operations. After the Phase 3
// port, Service no longer knows how to build OIDC config or parse
// SAML assertions — those concerns live in plugins, registered under
// the six adapter families (storage, notification, identity, email,
// ai, snapshot). Service is a thin dispatcher that:
//
//  1. Owns CRUD over identity_providers (List/ByID/Create/Update/
//     Delete/AllEnabled).
//  2. Owns protocol-agnostic provisioning helpers (findOrProvisionUser,
//     applyGroupMappings, createSessionForUser, autoImportGroups,
//     loadRuntimeProvider).
//  3. Owns the oidc_states + saml_requests tables used to map the
//     inbound state/relay-state token back to provider_id during the
//     /callback and /acs routes. (Plugins retain their own per-flow
//     session data — nonce, AuthnRequest ID — keyed by the same
//     token. The two layers agree on the same opaque token without
//     seeing each other's storage.)
//  4. Dispatches per-provider-type runtime work to the matching
//     plugin via runtime.IdentityAdapter(providerType).
type Service struct {
	db         *sql.DB
	encryption *encryption.Service
	authSvc    *auth.Service
	runtime    *pluginruntime.Runtime
	// vault reads this module's adopted credential through the secrets vault
	// once Phase 5b's start-up backfill has linked it, and falls back to the
	// legacy column until then. Built here rather than injected: every
	// construction site already supplies the database and the encryption
	// service, and a service built without it would quietly read the legacy
	// column forever.
	vault *secrets.Service
}

// NewService constructs an identity Service. The runtime handle may
// be nil during early boot; per-provider-type dispatch methods treat
// that as "no plugin registered" and surface a 500/400 to callers.
func NewService(db *sql.DB, enc *encryption.Service, authSvc *auth.Service, runtime *pluginruntime.Runtime) *Service {
	svc := &Service{db: db, encryption: enc, authSvc: authSvc, runtime: runtime}
	if db != nil && enc != nil {
		svc.vault = secrets.NewService(db, enc, nil)
	}
	return svc
}

// readClientSecret reads a provider's OIDC/SAML client secret through the
// vault when Phase 5b has adopted it, falling back to the legacy
// client_secret column until then. The rotating access and refresh tokens
// beside it are deliberately not adopted, so they keep their own path.
func (s *Service) readClientSecret(ctx context.Context, providerID, legacy string) (string, error) {
	if s.vault == nil {
		return s.encryption.Decrypt(legacy)
	}
	var secretID sql.NullString
	if err := s.db.QueryRowContext(ctx,
		"SELECT client_secret_secret_id FROM identity_providers WHERE id = ?", providerID,
	).Scan(&secretID); err != nil {
		// The row may be gone, or this may be a provider assembled in memory.
		// Either way the legacy ciphertext in hand is still good.
		return s.encryption.Decrypt(legacy)
	}
	return s.vault.ReadThrough(ctx, secretID, legacy)
}

// List returns all identity providers with secrets redacted.
func (s *Service) List(ctx context.Context) ([]Provider, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT ip.id, ip.name, ip.provider_type, ip.enabled, ip.client_id, ip.tenant_id, ip.domain, ip.issuer_url,
		        ip.metadata_url, ip.entity_id, ip.scopes, ip.auto_provision, ip.default_role_id,
		        ip.group_mapping_enabled, ip.group_mappings, ip.auto_import_groups, ip.config, ip.created_at, ip.updated_at,
		        dr.name
		 FROM identity_providers ip
		 LEFT JOIN roles dr ON dr.id = ip.default_role_id
		 ORDER BY ip.created_at DESC LIMIT 1000`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying identity providers: %w", err)
	}
	defer closeRows(rows)

	var providers []Provider
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		providers = append(providers, p)
	}
	if providers == nil {
		providers = []Provider{}
	}
	return providers, rows.Err()
}

// ByID returns a single identity provider by ID (secret redacted).
func (s *Service) ByID(ctx context.Context, id string) (*Provider, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, provider_type, enabled, client_id, tenant_id, domain, issuer_url,
		        metadata_url, entity_id, scopes, auto_provision, default_role_id,
		        group_mapping_enabled, group_mappings, auto_import_groups, config, created_at, updated_at
		 FROM identity_providers WHERE id = ?`, id,
	)

	p, err := scanProviderRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying identity provider: %w", err)
	}
	return &p, nil
}

// Create creates a new identity provider with encrypted secrets.
//
// The plugin-defined `input.Config` map is the source of truth for
// typed columns when present: known keys (client_id, client_secret,
// tenant_id, domain, issuer_url, metadata_url, entity_id, scopes,
// auto_provision, auto_import_groups, group_mapping_enabled,
// group_mappings) populate the typed columns. The pre-port typed
// fields on CreateProviderInput are kept as fallbacks for callers
// that haven't migrated.
func (s *Service) Create(ctx context.Context, input CreateProviderInput) (*Provider, error) {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	cfg := input.Config

	clientID := pickConfig(cfg, "client_id", input.ClientID)
	clientSecret := pickConfig(cfg, "client_secret", input.ClientSecret)
	scopes := pickConfig(cfg, "scopes", "")
	if scopes == "" {
		if input.Scopes != nil && *input.Scopes != "" {
			scopes = *input.Scopes
		} else {
			scopes = "openid profile email"
		}
	}

	var metadataURL, entityID *string
	if v := pickConfigPtr(cfg, "metadata_url"); v != "" {
		metadataURL = &v
	} else {
		metadataURL = input.MetadataURL
	}
	if v := pickConfigPtr(cfg, "entity_id"); v != "" {
		entityID = &v
	} else {
		entityID = input.EntityID
	}

	encSecret, err := s.encryption.Encrypt(clientSecret)
	if err != nil {
		return nil, fmt.Errorf("encrypting client secret: %w", err)
	}

	autoProvision := pickConfigBool(cfg, "auto_provision", true)
	if input.AutoProvision != nil {
		autoProvision = *input.AutoProvision
	}
	groupMappingEnabled := pickConfigBool(cfg, "group_mapping_enabled", false)
	if input.GroupMappingEnabled != nil {
		groupMappingEnabled = *input.GroupMappingEnabled
	}
	autoImportGroups := pickConfigBool(cfg, "auto_import_groups", false)
	if input.AutoImportGroups != nil {
		autoImportGroups = *input.AutoImportGroups
	}

	groupMappings := input.GroupMappings
	if v := pickConfig(cfg, "group_mappings", ""); v != "" {
		if err := json.Unmarshal([]byte(v), &groupMappings); err != nil {
			return nil, fmt.Errorf("parsing group_mappings from config: %w", err)
		}
	}
	mappingsJSON, err := json.Marshal(groupMappings)
	if err != nil {
		return nil, fmt.Errorf("marshaling group mappings: %w", err)
	}
	if groupMappings == nil {
		mappingsJSON = []byte("[]")
	}

	var tenantID, domain, issuerURL, defaultRoleID *string
	if v := pickConfigPtr(cfg, "tenant_id"); v != "" {
		tenantID = &v
	} else {
		tenantID = input.TenantID
	}
	if v := pickConfigPtr(cfg, "domain"); v != "" {
		domain = &v
	} else {
		domain = input.Domain
	}
	if v := pickConfigPtr(cfg, "issuer_url"); v != "" {
		issuerURL = &v
	} else {
		issuerURL = input.IssuerURL
	}
	if v := pickConfigPtr(cfg, "default_role_id"); v != "" {
		defaultRoleID = &v
	} else {
		defaultRoleID = input.DefaultRoleID
	}

	// The full plugin map is persisted alongside the typed columns, so
	// keys the host has no column for survive. Secrets inside it are
	// encrypted per key; see encodeConfig.
	configJSON, err := s.encodeConfig(input.ProviderType, cfg)
	if err != nil {
		return nil, err
	}

	// 22 named columns → 22 value slots: 19 placeholders plus the three
	// literals (enabled=0, sp_certificate=NULL, sp_private_key=NULL).
	// The 19 arguments below must stay in column order.
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO identity_providers
		 (id, name, provider_type, enabled, client_id, client_secret, tenant_id, domain, issuer_url,
		  metadata_url, entity_id, sp_certificate, sp_private_key, scopes, auto_provision,
		  default_role_id, group_mapping_enabled, group_mappings, auto_import_groups, config,
		  created_at, updated_at)
		 VALUES (?, ?, ?, 0, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, input.Name, input.ProviderType, clientID, encSecret, tenantID, domain, issuerURL,
		metadataURL, entityID, scopes,
		autoProvision, defaultRoleID, groupMappingEnabled, string(mappingsJSON),
		autoImportGroups, configJSON, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting identity provider: %w", err)
	}

	return s.ByID(ctx, id)
}

// Update partially updates an identity provider.
func (s *Service) Update(ctx context.Context, id string, input UpdateProviderInput) (*Provider, error) {
	existing, err := s.ByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	sets := []string{"updated_at = ?"}
	args := []any{now}

	if input.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *input.Name)
	}
	if input.Enabled != nil {
		sets = append(sets, "enabled = ?")
		args = append(args, *input.Enabled)
	}
	if input.ClientID != nil {
		sets = append(sets, "client_id = ?")
		args = append(args, *input.ClientID)
	}
	if input.ClientSecret != nil {
		enc, err := s.encryption.Encrypt(*input.ClientSecret)
		if err != nil {
			return nil, fmt.Errorf("encrypting client secret: %w", err)
		}
		// Clearing the pointer alongside the value is what keeps the adopted
		// copy from shadowing a freshly rotated client secret.
		sets = append(sets, "client_secret = ?", "client_secret_secret_id = NULL")
		args = append(args, enc)
	}
	if input.TenantID != nil {
		sets = append(sets, "tenant_id = ?")
		args = append(args, *input.TenantID)
	}
	if input.Domain != nil {
		sets = append(sets, "domain = ?")
		args = append(args, *input.Domain)
	}
	if input.IssuerURL != nil {
		sets = append(sets, "issuer_url = ?")
		args = append(args, *input.IssuerURL)
	}
	if input.MetadataURL != nil {
		sets = append(sets, "metadata_url = ?")
		args = append(args, *input.MetadataURL)
	}
	if input.EntityID != nil {
		sets = append(sets, "entity_id = ?")
		args = append(args, *input.EntityID)
	}
	if input.Scopes != nil {
		sets = append(sets, "scopes = ?")
		args = append(args, *input.Scopes)
	}
	if input.AutoProvision != nil {
		sets = append(sets, "auto_provision = ?")
		args = append(args, *input.AutoProvision)
	}
	if input.DefaultRoleID != nil {
		sets = append(sets, "default_role_id = ?")
		args = append(args, *input.DefaultRoleID)
	}
	if input.GroupMappingEnabled != nil {
		sets = append(sets, "group_mapping_enabled = ?")
		args = append(args, *input.GroupMappingEnabled)
	}
	if input.GroupMappings != nil {
		m, err := json.Marshal(input.GroupMappings)
		if err != nil {
			return nil, fmt.Errorf("marshaling group mappings: %w", err)
		}
		sets = append(sets, "group_mappings = ?")
		args = append(args, string(m))
	}
	if input.AutoImportGroups != nil {
		sets = append(sets, "auto_import_groups = ?")
		args = append(args, *input.AutoImportGroups)
	}

	// The plugin map is merged over what is stored rather than
	// replacing it, so a secret the operator left blank keeps its
	// existing value. Known keys are mirrored into their typed
	// columns as well, but only where the caller did not already set
	// that column through the legacy typed field above.
	if input.Config != nil {
		// Without the adapter the secret keys are unknown, so a merge would store
		// every submitted secret as plaintext.
		if _, _, ok := s.identityAdapter(existing.ProviderType); !ok {
			return nil, ErrIdentityAdapterUnavailable
		}
		storedConfig, err := s.storedConfig(ctx, id)
		if err != nil {
			return nil, err
		}
		merged := s.mergeConfig(existing.ProviderType, storedConfig, input.Config)
		encoded, err := s.encodeConfig(existing.ProviderType, merged)
		if err != nil {
			return nil, err
		}
		sets = append(sets, "config = ?")
		args = append(args, encoded)

		for _, mirror := range configColumnMirrors(input) {
			value, ok := input.Config[mirror.key]
			if !ok || strings.TrimSpace(value) == "" {
				continue
			}
			if mirror.encrypt {
				enc, err := s.encryption.Encrypt(value)
				if err != nil {
					return nil, fmt.Errorf("encrypting %s: %w", mirror.key, err)
				}
				value = enc
			}
			sets = append(sets, mirror.column+" = ?")
			args = append(args, value)
		}
	}

	args = append(args, id)
	query := "UPDATE identity_providers SET " + strings.Join(sets, ", ") + " WHERE id = ?" // SQL identifier: sets holds only constant column assignments and configColumnMirrors names
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return nil, fmt.Errorf("updating identity provider: %w", err)
	}

	return s.ByID(ctx, id)
}

// Delete removes an identity provider.
func (s *Service) Delete(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM identity_providers WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting identity provider: %w", err)
	}
	if db.RowsAffected(result) == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// AllEnabled returns enabled providers for the login page (minimal info only).
func (s *Service) AllEnabled(ctx context.Context) ([]EnabledProvider, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, provider_type FROM identity_providers WHERE enabled = 1 ORDER BY name LIMIT 1000`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying enabled providers: %w", err)
	}
	defer closeRows(rows)

	var providers []EnabledProvider
	for rows.Next() {
		var p EnabledProvider
		if err := rows.Scan(&p.ID, &p.Name, &p.ProviderType); err != nil {
			return nil, fmt.Errorf("scanning enabled provider: %w", err)
		}
		providers = append(providers, p)
	}
	if providers == nil {
		providers = []EnabledProvider{}
	}
	return providers, rows.Err()
}

// BeginAuth delegates to the plugin that owns providerType. After the
// plugin returns the redirect URL / HTML form, host extracts the
// state token (OIDC `state=` query param or SAML `RelayState=`) and
// persists state → provider_id in oidc_states / saml_requests so the
// /callback + /acs routes can dispatch HandleCallback without
// re-reading the plugin's in-memory map.
func (s *Service) BeginAuth(ctx context.Context, providerID, baseURL string) (*authFlowResult, error) {
	s.CleanupExpiredStates(ctx)

	provider, err := s.loadRuntimeProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	if !provider.Enabled {
		return nil, fmt.Errorf("provider is disabled")
	}

	adapter, spec, ok := s.identityAdapter(provider.ProviderType)
	if !ok {
		return nil, fmt.Errorf("no plugin registered for provider type: %s", provider.ProviderType)
	}

	cfg, err := s.providerToConfigMap(ctx, provider)
	if err != nil {
		return nil, err
	}

	result, err := adapter.BeginAuth(ctx, cfg, plugin.IdentityAuthRequest{
		ProviderID: providerID,
		BaseURL:    baseURL,
	})
	if err != nil {
		return nil, err
	}

	// Persist state → provider_id mapping so the callback handler
	// can resolve which plugin to dispatch to.
	switch {
	case spec.Capabilities.OIDCRedirect:
		if state := extractQueryParam(result.RedirectURL, "state"); state != "" {
			if storeErr := s.storeOIDCState(ctx, state, providerID); storeErr != nil {
				return nil, storeErr
			}
		}
	case spec.Capabilities.SAMLRedirect:
		var relayState string
		if result.RedirectURL != "" {
			relayState = extractQueryParam(result.RedirectURL, "RelayState")
		}
		if relayState == "" && result.HTMLForm != "" {
			relayState = extractFormInput(result.HTMLForm, "RelayState")
		}
		if relayState != "" {
			if storeErr := s.storeSAMLRequest(ctx, relayState, providerID); storeErr != nil {
				return nil, storeErr
			}
		}
	}

	return &authFlowResult{RedirectURL: result.RedirectURL, HTMLForm: result.HTMLForm}, nil
}

// ExchangeAndProvision validates the OIDC callback, delegates the
// code-exchange + userinfo-fetch to the plugin, and runs the host's
// protocol-agnostic provisioning + session-issuance pipeline.
func (s *Service) ExchangeAndProvision(ctx context.Context, stateParam, code, baseURL string) (string, *auth.User, error) {
	providerID, nonce, expiresAt, err := s.consumeOIDCState(ctx, stateParam)
	if err != nil {
		return "", nil, err
	}
	if expires, parseErr := time.Parse(time.RFC3339, expiresAt); parseErr == nil || time.Now().UTC().After(expires) {
		return "", nil, fmt.Errorf("state expired")
	}
	_ = nonce // nonce is consumed by the plugin; host just retains it on the row for audit/observability

	provider, err := s.loadRuntimeProvider(ctx, providerID)
	if err != nil {
		return "", nil, err
	}
	if !provider.Enabled {
		return "", nil, fmt.Errorf("provider is disabled")
	}

	adapter, _, ok := s.identityAdapter(provider.ProviderType)
	if !ok {
		return "", nil, fmt.Errorf("no plugin registered for provider type: %s", provider.ProviderType)
	}

	cfg, err := s.providerToConfigMap(ctx, provider)
	if err != nil {
		return "", nil, err
	}

	cbResult, err := adapter.HandleCallback(ctx, cfg, plugin.IdentityCallbackRequest{
		ProviderID: providerID,
		BaseURL:    baseURL,
		State:      stateParam,
		Code:       code,
	})
	if err != nil {
		return "", nil, fmt.Errorf("plugin handle callback: %w", err)
	}
	if cbResult.Subject == "" {
		return "", nil, fmt.Errorf("plugin callback missing subject")
	}

	user, err := s.findOrProvisionUser(ctx, providerID, externalUserInfo{
		Subject:        cbResult.Subject,
		Email:          cbResult.Email,
		Name:           cbResult.DisplayName,
		Groups:         cbResult.Groups,
		UsernamePrefix: s.usernamePrefix(provider.ProviderType),
	}, provider.AutoProvision, provider.DefaultRoleID)
	if err != nil {
		return "", nil, fmt.Errorf("provisioning user: %w", err)
	}

	s.syncProviderGroups(ctx, provider, user.ID, cbResult.Groups)

	return s.createSessionForUser(ctx, user)
}

// HandleSAMLACS delegates the SAML assertion parse to the plugin and
// runs provisioning + session issuance.
func (s *Service) HandleSAMLACS(ctx context.Context, providerID, baseURL string, r *http.Request) (string, *auth.User, error) {
	provider, err := s.loadRuntimeProvider(ctx, providerID)
	if err != nil {
		return "", nil, err
	}
	if !provider.Enabled {
		return "", nil, fmt.Errorf("provider is disabled")
	}

	adapter, _, ok := s.identityAdapter(provider.ProviderType)
	if !ok {
		return "", nil, fmt.Errorf("no plugin registered for provider type: %s", provider.ProviderType)
	}

	cfg, err := s.providerToConfigMap(ctx, provider)
	if err != nil {
		return "", nil, err
	}

	if err := r.ParseForm(); err != nil {
		return "", nil, fmt.Errorf("parsing saml response form: %w", err)
	}
	relayState := strings.TrimSpace(r.Form.Get("RelayState"))
	if relayState != "" {
		if _, _, _, lookupErr := s.lookupSAMLRequest(ctx, providerID, relayState); lookupErr != nil {
			return "", nil, lookupErr
		}
	}
	formMap := make(map[string]string, len(r.Form))
	for k, vs := range r.Form {
		if len(vs) > 0 {
			formMap[k] = vs[0]
		}
	}

	cbResult, err := adapter.HandleCallback(ctx, cfg, plugin.IdentityCallbackRequest{
		ProviderID: providerID,
		BaseURL:    baseURL,
		Form:       formMap,
		Query:      map[string]string{"RelayState": relayState},
		RawRequest: r,
	})
	if err != nil {
		return "", nil, fmt.Errorf("plugin handle callback: %w", err)
	}
	if cbResult.Subject == "" {
		return "", nil, fmt.Errorf("plugin callback missing subject")
	}

	user, err := s.findOrProvisionUser(ctx, providerID, externalUserInfo{
		Subject:        cbResult.Subject,
		Email:          cbResult.Email,
		Name:           cbResult.DisplayName,
		Groups:         cbResult.Groups,
		UsernamePrefix: s.usernamePrefix(provider.ProviderType),
	}, provider.AutoProvision, provider.DefaultRoleID)
	if err != nil {
		return "", nil, fmt.Errorf("provisioning user: %w", err)
	}

	s.syncProviderGroups(ctx, provider, user.ID, cbResult.Groups)

	return s.createSessionForUser(ctx, user)
}

// SAMLMetadata returns the SP metadata document. Plugins implement
// MetadataXML; the host just relays the bytes.
func (s *Service) SAMLMetadata(ctx context.Context, providerID, baseURL string) ([]byte, error) {
	provider, err := s.loadRuntimeProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}

	adapter, spec, ok := s.identityAdapter(provider.ProviderType)
	if !ok {
		return nil, fmt.Errorf("no plugin registered for provider type: %s", provider.ProviderType)
	}
	if !spec.Capabilities.SupportsMetadata {
		return nil, fmt.Errorf("provider type %q does not support SP metadata", provider.ProviderType)
	}

	cfg, err := s.providerToConfigMap(ctx, provider)
	if err != nil {
		return nil, err
	}

	return adapter.MetadataXML(ctx, cfg, baseURL)
}

// CleanupExpiredStates sweeps both state tables. Called lazily from
// BeginAuth; cheap because the tables stay small (10-minute TTL).
func (s *Service) CleanupExpiredStates(ctx context.Context) {
	now := time.Now().UTC().Format(time.RFC3339)

	if _, err := s.db.ExecContext(ctx, "DELETE FROM oidc_states WHERE expires_at < ?", now); err != nil {
		slog.Error("failed to cleanup expired oidc states", "error", err)
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM saml_requests WHERE expires_at < ?", now); err != nil {
		slog.Error("failed to cleanup expired saml requests", "error", err)
	}
}

// FetchProviderGroups delegates to the plugin's FetchGroups method.
// Adapters that declared SupportsGroupFetch=false return
// ErrUnsupported which we surface as-is.
func (s *Service) FetchProviderGroups(ctx context.Context, providerID string) ([]ProviderGroupInfo, error) {
	provider, err := s.loadRuntimeProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}

	adapter, spec, ok := s.identityAdapter(provider.ProviderType)
	if !ok {
		return nil, fmt.Errorf("no plugin registered for provider type: %s", provider.ProviderType)
	}
	if !spec.Capabilities.SupportsGroupFetch {
		return nil, fmt.Errorf("provider type %q does not support group fetch", provider.ProviderType)
	}

	cfg, err := s.providerToConfigMap(ctx, provider)
	if err != nil {
		return nil, err
	}

	groups, err := adapter.FetchGroups(ctx, cfg)
	if err != nil {
		return nil, err
	}

	result := make([]ProviderGroupInfo, 0, len(groups))
	for _, g := range groups {
		result = append(result, ProviderGroupInfo{
			ID:          g.ID,
			Name:        g.Name,
			Description: g.Description,
		})
	}
	return result, nil
}

// applyGroupMappings syncs group memberships based on provider group claims.
func (s *Service) applyGroupMappings(ctx context.Context, userID, mappingsJSON string, providerGroups []string) {
	var mappings []GroupMapping
	if err := json.Unmarshal([]byte(mappingsJSON), &mappings); err != nil {
		slog.Warn("failed to unmarshal group mappings", "userID", userID, "error", err)
		return
	}

	// The decisions are shared with the mapping preview, so what the preview
	// promises is what a sign-in writes.
	order, decisions := mappingDecisions(mappings, providerGroups)
	now := time.Now().UTC().Format(time.RFC3339)
	// bounded by the group mappings of one provider; the sync repeats on every sign-in
	for _, groupID := range order {
		if decisions[groupID] {
			gmID := xid.New().String()
			if _, gmErr := s.db.ExecContext(ctx,
				`INSERT OR IGNORE INTO group_members (id, group_id, user_id, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?)`,
				gmID, groupID, userID, now, now,
			); gmErr != nil {
				slog.Error("failed to assign group", "userID", userID, "groupID", groupID, "error", gmErr)
			}
		} else {
			if _, rmErr := s.db.ExecContext(ctx,
				`DELETE FROM group_members WHERE group_id = ? AND user_id = ?`,
				groupID, userID,
			); rmErr != nil {
				slog.Error("failed to remove group membership", "userID", userID, "groupID", groupID, "error", rmErr)
			}
		}
	}
}

// syncProviderGroups records a sign-in's group claims and applies the provider's
// auto-import and mappings. Every sign-in path calls it, so the mapping preview
// describes what any of them writes.
func (s *Service) syncProviderGroups(ctx context.Context, provider runtimeProvider, userID string, groups []string) {
	s.recordGroupClaims(ctx, provider.ID, userID, groups)
	if len(groups) == 0 {
		return
	}
	if provider.AutoImportGroups {
		s.autoImportGroups(ctx, userID, groups, provider.ProviderType)
	}
	if provider.GroupMappingEnabled {
		s.applyGroupMappings(ctx, userID, provider.GroupMappingsJSON, groups)
	}
}

// logIdentityWarning records a non-fatal sign-in side effect that failed; the
// login itself still succeeds.
func logIdentityWarning(what, providerID, userID string, err error) {
	slog.Warn(what+" failed", "provider_id", providerID, "user_id", userID, "error", err)
}

// createSessionForUser issues a new session via the host's auth
// service and stamps last_login. Used by every callback flow.
func (s *Service) createSessionForUser(ctx context.Context, user *auth.User) (string, *auth.User, error) {
	sessionID, err := s.authSvc.CreateSessionContext(ctx, user.ID)
	if err != nil {
		return "", nil, fmt.Errorf("creating session: %w", err)
	}

	if _, loginErr := s.db.ExecContext(ctx,
		"UPDATE users SET last_login = ? WHERE id = ?",
		time.Now().UTC().Format(time.RFC3339),
		user.ID,
	); loginErr != nil {
		slog.Error("failed to update last login", "userID", user.ID, "error", loginErr)
	}

	return sessionID, user, nil
}

// loadRuntimeProvider reads the typed columns from identity_providers.
// Plugins read the same data via providerToConfigMap.
func (s *Service) loadRuntimeProvider(ctx context.Context, providerID string) (runtimeProvider, error) {
	var provider runtimeProvider
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, provider_type, client_id, client_secret, tenant_id, domain, issuer_url,
		        metadata_url, entity_id, sp_certificate, sp_private_key, scopes, enabled,
		        auto_provision, default_role_id, group_mapping_enabled, group_mappings,
		        auto_import_groups, config
		 FROM identity_providers WHERE id = ?`,
		providerID,
	).Scan(
		&provider.ID,
		&provider.Name,
		&provider.ProviderType,
		&provider.ClientID,
		&provider.EncryptedClientSecret,
		&provider.TenantID,
		&provider.Domain,
		&provider.IssuerURL,
		&provider.MetadataURL,
		&provider.EntityID,
		&provider.EncryptedSPCert,
		&provider.EncryptedSPKey,
		&provider.Scopes,
		&provider.Enabled,
		&provider.AutoProvision,
		&provider.DefaultRoleID,
		&provider.GroupMappingEnabled,
		&provider.GroupMappingsJSON,
		&provider.AutoImportGroups,
		&provider.ConfigJSON,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return provider, fmt.Errorf("provider not found")
	}
	if err != nil {
		return provider, fmt.Errorf("querying provider: %w", err)
	}
	return provider, nil
}

// providerToConfigMap converts a stored provider into the flat
// map[string]string plugins consume. Two layers, in order:
//
//  1. The host's typed columns — the keys every provider has had since
//     before the plugin port. Rows written before migration 125 have
//     nothing else, so this layer keeps them working untouched.
//  2. The plugin-owned config map, overlaid on top. Known keys resolve
//     to the same value either way (Create and Update write both);
//     keys the host has no column for exist only here.
//
// Secrets are decrypted on the way out — client_secret,
// sp_certificate, sp_private_key from their columns, and anything the
// plugin's ConfigSchema marked secret from the config map. The result
// is server-side only and must never be returned to a client.
func (s *Service) providerToConfigMap(ctx context.Context, provider runtimeProvider) (map[string]string, error) {
	cfg := map[string]string{
		"provider_id":    provider.ID,
		"provider_name":  provider.Name,
		"provider_type":  provider.ProviderType,
		"client_id":      provider.ClientID,
		"tenant_id":      provider.nullTenantID(),
		"domain":         provider.nullDomain(),
		"issuer_url":     provider.nullIssuerURL(),
		"metadata_url":   provider.nullMetadataURL(),
		"entity_id":      provider.nullEntityID(),
		"scopes":         provider.Scopes,
		"enabled":        boolStr(provider.Enabled),
		"auto_provision": boolStr(provider.AutoProvision),
		"auto_import":    boolStr(provider.AutoImportGroups),
	}

	if provider.EncryptedClientSecret != "" {
		// The OIDC/SAML *client* secret is adopted; the rotating access and
		// refresh tokens at :390 and :1383 deliberately are not.
		dec, err := s.readClientSecret(ctx, provider.ID, provider.EncryptedClientSecret)
		if err != nil {
			return nil, fmt.Errorf("decrypting client secret: %w", err)
		}
		cfg["client_secret"] = dec
	}

	if provider.EncryptedSPCert.Valid && provider.EncryptedSPCert.String != "" {
		dec, err := decryptOptionalString(s.encryption, provider.EncryptedSPCert)
		if err != nil {
			return nil, fmt.Errorf("decrypting sp_certificate: %w", err)
		}
		cfg["sp_certificate"] = dec
	}
	if provider.EncryptedSPKey.Valid && provider.EncryptedSPKey.String != "" {
		dec, err := decryptOptionalString(s.encryption, provider.EncryptedSPKey)
		if err != nil {
			return nil, fmt.Errorf("decrypting sp_private_key: %w", err)
		}
		cfg["sp_private_key"] = dec
	}

	pluginCfg, err := s.decryptConfigValues(decodeConfig(provider.ConfigJSON))
	if err != nil {
		return nil, fmt.Errorf("decrypting provider config: %w", err)
	}
	for key, value := range pluginCfg {
		// An empty stored value must not blank a typed column that
		// still holds something — the config map is an overlay, not a
		// replacement.
		if value == "" {
			if _, exists := cfg[key]; exists {
				continue
			}
		}
		cfg[key] = value
	}

	return cfg, nil
}

// identityAdapter returns the plugin that owns providerType, or ok=false
// when no plugin registered for that type (e.g. plugin disabled).
func (s *Service) identityAdapter(providerType string) (plugin.IdentityAdapterClient, plugin.IdentityAdapterSpec, bool) {
	if s.runtime == nil {
		return nil, plugin.IdentityAdapterSpec{}, false
	}
	return s.runtime.IdentityAdapter(providerType)
}

// usernamePrefix returns the username-prefix convention for auto-
// generated usernames, mirroring the pre-plugin host behavior:
//
//	entra_id     → "oidc_"
//	google       → "oidc_"
//	generic_oidc → "oidc_"
//	saml_2_0     → "saml_"
//	ldap         → "ldap_"
//	(anything else) → "idp_"
func (s *Service) usernamePrefix(providerType string) string {
	switch providerType {
	case "entra_id", "google", "generic_oidc":
		return "oidc_"
	case "saml_2_0":
		return "saml_"
	case "ldap":
		return "ldap_"
	default:
		return "idp_"
	}
}

// findOrProvisionUser looks up a user by external_id or creates a new one.
func (s *Service) findOrProvisionUser(ctx context.Context, providerID string, info externalUserInfo, autoProvision bool, defaultRoleID sql.NullString) (*auth.User, error) {
	var userID, username string
	var displayName, email sql.NullString

	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, display_name, email FROM users
		 WHERE external_id = ? AND identity_provider_id = ?`,
		info.Subject, providerID,
	).Scan(&userID, &username, &displayName, &email)

	if err == nil {
		now := time.Now().UTC().Format(time.RFC3339)
		if _, updErr := s.db.ExecContext(ctx,
			`UPDATE users SET display_name = ?, email = ?, updated_at = ? WHERE id = ?`,
			nullableStr(info.Name), nullableStr(info.Email), now, userID,
		); updErr != nil {
			slog.Error("failed to update user profile", "userID", userID, "error", updErr)
		}
		user := &auth.User{ID: userID, Username: username}
		if info.Name != "" {
			user.DisplayName = &info.Name
		} else if displayName.Valid {
			user.DisplayName = &displayName.String
		}
		if info.Email != "" {
			user.Email = &info.Email
		} else if email.Valid {
			user.Email = &email.String
		}
		return user, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("querying user by external_id: %w", err)
	}

	if info.Email != "" {
		err = s.db.QueryRowContext(ctx,
			`SELECT id, username, display_name, email FROM users WHERE email = ?`,
			info.Email,
		).Scan(&userID, &username, &displayName, &email)
		if err == nil {
			now := time.Now().UTC().Format(time.RFC3339)
			if _, linkErr := s.db.ExecContext(ctx,
				`UPDATE users SET external_id = ?, identity_provider_id = ?, updated_at = ? WHERE id = ?`,
				info.Subject, providerID, now, userID,
			); linkErr != nil {
				slog.Error("failed to link user to provider", "userID", userID, "error", linkErr)
			}
			user := &auth.User{ID: userID, Username: username}
			if displayName.Valid {
				user.DisplayName = &displayName.String
			}
			if email.Valid {
				user.Email = &email.String
			}
			return user, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("querying user by email: %w", err)
		}
	}

	if !autoProvision {
		return nil, errUserNotProvisioned
	}

	userID = xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	username = info.Email
	prefix := info.UsernamePrefix
	if prefix == "" {
		prefix = "idp_"
	}
	if username == "" && strings.TrimSpace(info.PreferredUsername) != "" {
		username, err = s.freeUsername(ctx, prefix+strings.TrimSpace(info.PreferredUsername))
		if err != nil {
			return nil, err
		}
	}
	if username == "" {
		subLen := min(len(info.Subject), 8)
		username = prefix + info.Subject[:subLen]
	}
	// Usernames are case-insensitive; normalise before it reaches the
	// INSERT below so IdP-provisioned accounts (this path, and the
	// plugin ProvisionUser bridge which calls into this same function)
	// can't create a case-variant duplicate of an existing local user.
	username = auth.NormalizeUsername(username)

	randomPass := make([]byte, 32)
	if _, err := rand.Read(randomPass); err != nil {
		return nil, fmt.Errorf("generating random password: %w", err)
	}
	passwordHash, err := auth.HashPassword(hex.EncodeToString(randomPass))
	if err != nil {
		return nil, fmt.Errorf("hashing random password: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO users (id, username, password_hash, display_name, email,
		                     external_id, identity_provider_id, is_active, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		userID, username, passwordHash, nullableStr(info.Name), nullableStr(info.Email),
		info.Subject, providerID, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting user: %w", err)
	}

	if defaultRoleID.Valid && defaultRoleID.String != "" {
		roleAssignID := xid.New().String()
		if _, roleErr := s.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO user_roles (id, user_id, role_id, environment_id, created_at, updated_at)
			 VALUES (?, ?, ?, NULL, ?, ?)`,
			roleAssignID, userID, defaultRoleID.String, now, now,
		); roleErr != nil {
			slog.Error("failed to assign default role", "userID", userID, "roleID", defaultRoleID.String, "error", roleErr)
		}
	}

	user := &auth.User{ID: userID, Username: username}
	if info.Name != "" {
		user.DisplayName = &info.Name
	}
	if info.Email != "" {
		user.Email = &info.Email
	}
	return user, nil
}

// autoImportGroups automatically creates McHarbor groups from provider
// claims and syncs the user's membership for the given identity
// source.
func (s *Service) autoImportGroups(ctx context.Context, userID string, providerGroups []string, source string) {
	now := time.Now().UTC().Format(time.RFC3339)
	shouldBeIn := make(map[string]bool, len(providerGroups))

	// One lookup per claimed group, bounded by the group list the provider
	// put in the token, and run once per login.
	for _, groupName := range providerGroups {
		var groupID string
		err := s.db.QueryRowContext(ctx, `SELECT id FROM groups WHERE name = ?`, groupName).Scan(&groupID)

		if errors.Is(err, sql.ErrNoRows) {
			groupID = xid.New().String()
			if _, createErr := s.db.ExecContext(ctx,
				`INSERT INTO groups (id, name, description, is_system, created_at, updated_at)
				 VALUES (?, ?, ?, 0, ?, ?)`,
				groupID, groupName, "Auto-imported from identity provider", now, now,
			); createErr != nil {
				slog.Error("failed to auto-create group", "name", groupName, "error", createErr)
				continue
			}
		} else if err != nil {
			slog.Error("failed to query group by name", "name", groupName, "error", err)
			continue
		}

		shouldBeIn[groupID] = true

		gmID := xid.New().String()
		if _, gmErr := s.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO group_members (id, group_id, user_id, source, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			gmID, groupID, userID, source, now, now,
		); gmErr != nil {
			slog.Error("failed to assign auto-imported group", "userID", userID, "groupID", groupID, "error", gmErr)
		}
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT group_id FROM group_members WHERE user_id = ? AND source = ?`,
		userID, source,
	)
	if err != nil {
		slog.Error("failed to query identity group memberships", "userID", userID, "source", source, "error", err)
		return
	}
	defer closeRows(rows)

	// bounded by the memberships one user holds from one source; the sync repeats on every sign-in
	for rows.Next() {
		var groupID string
		if err := rows.Scan(&groupID); err != nil {
			slog.Error("failed to scan group membership", "error", err)
			continue
		}
		if !shouldBeIn[groupID] {
			if _, rmErr := s.db.ExecContext(ctx,
				`DELETE FROM group_members WHERE group_id = ? AND user_id = ? AND source = ?`,
				groupID, userID, source,
			); rmErr != nil {
				slog.Error("failed to remove stale identity group membership", "userID", userID, "groupID", groupID, "source", source, "error", rmErr)
			}
		}
	}
	if err := rows.Err(); err != nil {
		slog.Error("failed to iterate identity group memberships", "userID", userID, "source", source, "error", err)
	}
}

// --- state persistence (oidc_states + saml_requests) ---------------
//
// Host-owned tables that map an opaque state / relay-state token
// back to the provider_id that originated the /authorize request.
// Plugins retain their own per-flow session data keyed by the same
// token (nonce, AuthnRequest ID, expiry) — the two layers agree on
// the token without seeing each other's storage.

func (s *Service) storeOIDCState(ctx context.Context, state, providerID string) error {
	stateID := xid.New().String()
	now := time.Now().UTC()
	expiresAt := now.Add(stateExpiry).Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO oidc_states (id, provider_id, state, nonce, redirect_url, expires_at, created_at)
		 VALUES (?, ?, ?, '', '', ?, ?)`,
		stateID, providerID, state, expiresAt, now.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("inserting oidc state: %w", err)
	}
	return nil
}

func (s *Service) consumeOIDCState(ctx context.Context, state string) (providerID, nonce, expiresAt string, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT provider_id, nonce, expires_at FROM oidc_states WHERE state = ?`, state,
	).Scan(&providerID, &nonce, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", fmt.Errorf("invalid state")
	}
	if err != nil {
		return "", "", "", fmt.Errorf("querying oidc state: %w", err)
	}
	if _, delErr := s.db.ExecContext(ctx, "DELETE FROM oidc_states WHERE state = ?", state); delErr != nil {
		slog.Error("failed to delete oidc state", "state", state, "error", delErr)
	}
	return providerID, nonce, expiresAt, nil
}

func (s *Service) storeSAMLRequest(ctx context.Context, relayState, providerID string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO saml_requests (id, provider_id, request_id, relay_state, expires_at, created_at)
		 VALUES (?, ?, '', ?, ?, ?)`,
		xid.New().String(), providerID, relayState,
		time.Now().UTC().Add(stateExpiry).Format(time.RFC3339),
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("inserting saml request: %w", err)
	}
	return nil
}

// lookupSAMLRequest returns the request_id + expiry for the given
// relay state, deleting the row. Used by HandleSAMLACS to gate the
// plugin's HandleCallback call on a known relay state.
func (s *Service) lookupSAMLRequest(ctx context.Context, providerID, relayState string) (requestID, expiresAt string, ok bool, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT request_id, expires_at FROM saml_requests WHERE provider_id = ? AND relay_state = ?`,
		providerID, relayState,
	).Scan(&requestID, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, fmt.Errorf("invalid relay state")
	}
	if err != nil {
		return "", "", false, fmt.Errorf("querying saml request: %w", err)
	}
	if _, delErr := s.db.ExecContext(ctx,
		`DELETE FROM saml_requests WHERE provider_id = ? AND relay_state = ?`,
		providerID, relayState,
	); delErr != nil {
		slog.Error("failed to delete saml request", "providerID", providerID, "relayState", relayState, "error", delErr)
	}
	exp, parseErr := time.Parse(time.RFC3339, expiresAt)
	if parseErr != nil || time.Now().UTC().After(exp) {
		return "", "", false, fmt.Errorf("saml request expired")
	}
	return requestID, expiresAt, true, nil
}

// --- scanning helpers ------------------------------------------------

// rowScanner is the shared surface of *sql.Row and *sql.Rows, so the
// column list is decoded in exactly one place. The two scanners below
// used to be byte-identical copies; keeping them apart meant every new
// column had to be added twice.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanProviderFrom decodes one identity_providers row into the API shape,
// appending any extra scan targets a caller's query added trailing columns
// for (List's defaultRoleName join). The config map it produces is
// REDACTED — encrypted values are dropped, never decrypted. Plugin dispatch
// reads config through providerToConfigMap instead.
func scanProviderFrom(sc rowScanner, extra ...any) (Provider, error) {
	var p Provider
	var tenantID, domain, issuerURL, metadataURL, entityID, defaultRoleID sql.NullString
	var configJSON sql.NullString
	var enabled, autoProvision, groupMappingEnabled, autoImportGroups bool
	var mappingsJSON string

	args := []any{
		&p.ID, &p.Name, &p.ProviderType, &enabled, &p.ClientID,
		&tenantID, &domain, &issuerURL, &metadataURL, &entityID, &p.Scopes, &autoProvision,
		&defaultRoleID, &groupMappingEnabled, &mappingsJSON, &autoImportGroups, &configJSON,
		&p.CreatedAt, &p.UpdatedAt,
	}
	args = append(args, extra...)

	err := sc.Scan(args...)
	if err != nil {
		return p, err
	}

	p.Enabled = enabled
	p.AutoProvision = autoProvision
	p.GroupMappingEnabled = groupMappingEnabled
	p.AutoImportGroups = autoImportGroups
	if tenantID.Valid {
		p.TenantID = &tenantID.String
	}
	if domain.Valid {
		p.Domain = &domain.String
	}
	if issuerURL.Valid {
		p.IssuerURL = &issuerURL.String
	}
	if metadataURL.Valid {
		p.MetadataURL = &metadataURL.String
	}
	if entityID.Valid {
		p.EntityID = &entityID.String
	}
	if defaultRoleID.Valid {
		p.DefaultRoleID = &defaultRoleID.String
	}

	p.Config = redactConfigValues(decodeConfig(configJSON))

	if err := json.Unmarshal([]byte(mappingsJSON), &p.GroupMappings); err != nil {
		p.GroupMappings = []GroupMapping{}
	}
	if p.GroupMappings == nil {
		p.GroupMappings = []GroupMapping{}
	}

	return p, nil
}

// scanProvider scans an Provider from a List row, whose query joins
// roles for a trailing default-role-name column — the one extra target
// scanProviderFrom's variadic exists for.
func scanProvider(rows *sql.Rows) (Provider, error) {
	var defaultRoleName sql.NullString
	p, err := scanProviderFrom(rows, &defaultRoleName)
	if err != nil {
		return p, fmt.Errorf("scanning identity provider: %w", err)
	}
	if defaultRoleName.Valid {
		p.DefaultRoleName = &defaultRoleName.String
	}
	return p, nil
}

// scanProviderRow scans a single row. The error is returned unwrapped
// so callers can still compare it against sql.ErrNoRows.
func scanProviderRow(row *sql.Row) (Provider, error) {
	return scanProviderFrom(row)
}

// --- small helpers ---------------------------------------------------

func decryptOptionalString(enc *encryption.Service, value sql.NullString) (string, error) {
	if !value.Valid || value.String == "" {
		return "", nil
	}
	return enc.Decrypt(value.String)
}

func (p runtimeProvider) nullTenantID() string {
	if p.TenantID.Valid {
		return p.TenantID.String
	}
	return ""
}

func (p runtimeProvider) nullDomain() string {
	if p.Domain.Valid {
		return p.Domain.String
	}
	return ""
}

func (p runtimeProvider) nullIssuerURL() string {
	if p.IssuerURL.Valid {
		return p.IssuerURL.String
	}
	return ""
}

func (p runtimeProvider) nullMetadataURL() string {
	if p.MetadataURL.Valid {
		return p.MetadataURL.String
	}
	return ""
}

func (p runtimeProvider) nullEntityID() string {
	if p.EntityID.Valid {
		return p.EntityID.String
	}
	return ""
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func nullableStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// encPrefix marks a value inside the identity_providers.config JSON
// map as ciphertext. Secrets are encrypted per key rather than as one
// blob so the non-secret keys stay readable without a decrypt, and the
// marker is stored inline so the redaction path can recognise a secret
// without consulting the plugin's ConfigSchema — an uninstalled or
// failed-to-load plugin therefore still redacts correctly instead of
// leaking. A plaintext value that happens to start with this prefix is
// harmless: decryptConfigValues falls back to returning it verbatim
// when the decrypt fails.
const encPrefix = "enc:v1:"

// ErrIdentityAdapterUnavailable refuses a config change for a provider whose plugin is
// not loaded: its ConfigSchema is the only record of which keys must be encrypted.
var ErrIdentityAdapterUnavailable = errors.New("identity: provider adapter is not loaded")

// configSecretKeys returns the set of config keys the plugin owning
// providerType declares as Type "secret". An unknown provider type
// (plugin disabled or uninstalled) yields an empty set — callers must
// treat that as "encrypt nothing new", never as "decrypt everything".
func (s *Service) configSecretKeys(providerType string) map[string]bool {
	out := map[string]bool{}
	_, spec, ok := s.identityAdapter(providerType)
	if !ok {
		return out
	}
	for _, field := range spec.ConfigSchema {
		if strings.EqualFold(field.Type, "secret") {
			out[field.Key] = true
		}
	}
	return out
}

// encodeConfig serialises a plugin config map for storage, encrypting
// every value the plugin's ConfigSchema declares as a secret. Empty
// values are never encrypted (an empty ciphertext would be
// indistinguishable from "not set" on the way back out).
func (s *Service) encodeConfig(providerType string, cfg map[string]string) (string, error) {
	if len(cfg) == 0 {
		return "{}", nil
	}
	secrets := s.configSecretKeys(providerType)
	out := make(map[string]string, len(cfg))
	for key, value := range cfg {
		// Already-encrypted values arrive here via mergeConfig, which
		// carries stored ciphertext forward for secrets the caller
		// left blank. Re-encrypting would nest the ciphertext and
		// break every future decrypt.
		if !secrets[key] || value == "" || strings.HasPrefix(value, encPrefix) {
			out[key] = value
			continue
		}
		enc, err := s.encryption.Encrypt(value)
		if err != nil {
			return "", fmt.Errorf("encrypting config key %s: %w", key, err)
		}
		out[key] = encPrefix + enc
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("marshaling provider config: %w", err)
	}
	return string(encoded), nil
}

// decodeConfig parses the stored JSON map. Values stay in their stored
// form — still prefixed and encrypted where applicable. Callers pick
// decryptConfigValues (plugin dispatch) or redactConfigValues (API
// responses); there is deliberately no path that returns raw
// ciphertext to a caller that asked for neither.
func decodeConfig(raw sql.NullString) map[string]string {
	if !raw.Valid {
		return map[string]string{}
	}
	trimmed := strings.TrimSpace(raw.String)
	if trimmed == "" || trimmed == "{}" {
		return map[string]string{}
	}
	out := map[string]string{}
	if err := json.Unmarshal([]byte(trimmed), &out); err != nil {
		return map[string]string{}
	}
	return out
}

// decryptConfigValues returns the plugin-facing view: ciphertext
// decrypted, everything else passed through. Server-side only — the
// result must never reach an API response.
func (s *Service) decryptConfigValues(cfg map[string]string) (map[string]string, error) {
	out := make(map[string]string, len(cfg))
	for key, value := range cfg {
		if !strings.HasPrefix(value, encPrefix) {
			out[key] = value
			continue
		}
		dec, err := s.encryption.Decrypt(strings.TrimPrefix(value, encPrefix))
		if err != nil {
			// Not ciphertext after all — a plaintext value that
			// happens to lead with the marker. Pass it through
			// rather than failing the whole login flow.
			out[key] = value
			continue
		}
		out[key] = dec
	}
	return out, nil
}

// redactConfigValues returns the API-facing view: every encrypted
// value dropped, everything else passed through. Dropping rather than
// blanking keeps the key out of the edit dialog's dirty-field
// detection, so an untouched secret is not resubmitted as "".
func redactConfigValues(cfg map[string]string) map[string]string {
	out := make(map[string]string, len(cfg))
	for key, value := range cfg {
		if strings.HasPrefix(value, encPrefix) {
			continue
		}
		out[key] = value
	}
	return out
}

// mergeConfig layers an incoming config map over the stored one. A key
// the caller omits keeps its stored value; a key the caller sends
// empty for a secret that already has a stored value keeps the stored
// value too — that is the "leave blank to keep the existing secret"
// contract the edit dialog relies on. Everything else overwrites.
func (s *Service) mergeConfig(providerType string, stored, incoming map[string]string) map[string]string {
	secrets := s.configSecretKeys(providerType)
	out := make(map[string]string, len(stored)+len(incoming))
	for key, value := range stored {
		out[key] = value
	}
	for key, value := range incoming {
		if value == "" && secrets[key] {
			if _, exists := out[key]; exists {
				continue
			}
		}
		out[key] = value
	}
	return out
}

// storedConfig reads just the config column for one provider, in its
// stored (still-encrypted) form. Used by Update to merge rather than
// clobber.
func (s *Service) storedConfig(ctx context.Context, id string) (map[string]string, error) {
	var raw sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT config FROM identity_providers WHERE id = ?`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying provider config: %w", err)
	}
	return decodeConfig(raw), nil
}

// configColumnMirror ties a plugin config key to the typed column that
// shadows it. The typed columns are still read by the login flow and
// the provider list, so Update keeps them in step with the config map
// for the keys the host knows.
type configColumnMirror struct {
	key     string
	column  string
	encrypt bool
}

// configColumnMirrors returns the mirrors to apply for this request,
// skipping any column the caller already set through its legacy typed
// field — writing the same column twice in one UPDATE would be
// ambiguous about which value wins.
func configColumnMirrors(input UpdateProviderInput) []configColumnMirror {
	claimed := map[string]bool{
		"client_id":     input.ClientID != nil,
		"client_secret": input.ClientSecret != nil,
		"tenant_id":     input.TenantID != nil,
		"domain":        input.Domain != nil,
		"issuer_url":    input.IssuerURL != nil,
		"metadata_url":  input.MetadataURL != nil,
		"entity_id":     input.EntityID != nil,
		"scopes":        input.Scopes != nil,
	}
	all := []configColumnMirror{
		{key: "client_id", column: "client_id"},
		{key: "client_secret", column: "client_secret", encrypt: true},
		{key: "tenant_id", column: "tenant_id"},
		{key: "domain", column: "domain"},
		{key: "issuer_url", column: "issuer_url"},
		{key: "metadata_url", column: "metadata_url"},
		{key: "entity_id", column: "entity_id"},
		{key: "scopes", column: "scopes"},
	}
	out := make([]configColumnMirror, 0, len(all))
	for _, mirror := range all {
		if claimed[mirror.key] {
			continue
		}
		out = append(out, mirror)
	}
	return out
}

// pickConfig returns the value at key from cfg when non-empty,
// otherwise the fallback. Used to bridge the new plugin-defined
// `Config` map and the pre-port typed fields on Create/Update
// request bodies.
func pickConfig(cfg map[string]string, key, fallback string) string {
	if v, ok := cfg[key]; ok && strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}

// pickConfigPtr is the *string variant — returns "" when missing so
// callers can choose between *string and string without an extra
// allocation.
func pickConfigPtr(cfg map[string]string, key string) string {
	if v, ok := cfg[key]; ok {
		return v
	}
	return ""
}

// pickConfigBool returns the bool value at key. Accepts "true",
// "1", "yes", "on" (case-insensitive) as truthy. Falls back when
// the key is missing or unparseable.
func pickConfigBool(cfg map[string]string, key string, fallback bool) bool {
	v, ok := cfg[key]
	if !ok {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	default:
		return fallback
	}
}

// extractQueryParam returns the value of a single URL query parameter
// from a redirect URL, or "" when the URL can't be parsed or the
// param is missing. Used by BeginAuth to recover the state token
// from a plugin-built redirect URL.
func extractQueryParam(rawURL, param string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Query().Get(param)
}

// relayStateFormRegexp matches the SAML HTTP-Post binding's hidden
// RelayState input: `<input ... name="RelayState" value="..."/>`.
// The value is captured into group 1.
var relayStateFormRegexp = regexp.MustCompile(`(?i)name=["']RelayState["'][^>]*value=["']([^"']+)["']`)

// extractFormInput returns the value of a single hidden input field
// from an auto-submitting SAML HTML form. Used by BeginAuth when the
// plugin returns HTMLForm instead of RedirectURL.
func extractFormInput(html, name string) string {
	if name != "RelayState" {
		return ""
	}
	matches := relayStateFormRegexp.FindStringSubmatch(html)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

// errUserNotProvisioned is the internal sentinel returned by
// findOrProvisionUser when auto-provisioning is disabled and no
// matching user exists. Defined in host_callbacks.go alongside the
// HostProvisioner that maps it to pluginruntime.ErrUserNotFound so
// plugins see the SDK-level error.

// closeRows releases a result set and logs a failed Close, which a deferred call cannot return.
func closeRows(rows *sql.Rows) {
	if err := rows.Close(); err != nil {
		slog.Warn("identity: closing rows", "error", err)
	}
}
