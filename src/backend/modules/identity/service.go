// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package identity

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/crewjam/saml"
	"github.com/rs/xid"
	"golang.org/x/oauth2"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/oidc"
	"github.com/therealmcsparrow/mcharbor/core/samlx"
)

const stateExpiry = 10 * time.Minute

type authFlowResult struct {
	RedirectURL string
	HTMLForm    string
}

type externalUserInfo struct {
	Subject        string
	Email          string
	Name           string
	Groups         []string
	UsernamePrefix string
}

type runtimeProvider struct {
	ClientID              string
	EncryptedClientSecret string
	ProviderType          string
	Scopes                string
	TenantID              sql.NullString
	Domain                sql.NullString
	IssuerURL             sql.NullString
	MetadataURL           sql.NullString
	EntityID              sql.NullString
	EncryptedSPCert       sql.NullString
	EncryptedSPKey        sql.NullString
	Enabled               bool
	AutoProvision         bool
	DefaultRoleID         sql.NullString
	GroupMappingEnabled   bool
	GroupMappingsJSON     string
	AutoImportGroups      bool
}

// Service handles identity provider operations.
type Service struct {
	db         *sql.DB
	encryption *encryption.Service
	authSvc    *auth.Service
}

// NewService creates a new identity service.
func NewService(db *sql.DB, enc *encryption.Service, authSvc *auth.Service) *Service {
	return &Service{db: db, encryption: enc, authSvc: authSvc}
}

// List returns all identity providers with secrets redacted.
func (s *Service) List() ([]IdentityProvider, error) {
	rows, err := s.db.Query(
		`SELECT id, name, provider_type, enabled, client_id, tenant_id, domain, issuer_url,
		        metadata_url, entity_id, scopes, auto_provision, default_role_id,
		        group_mapping_enabled, group_mappings, auto_import_groups, created_at, updated_at
		 FROM identity_providers ORDER BY created_at DESC LIMIT 1000`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying identity providers: %w", err)
	}
	defer rows.Close()

	var providers []IdentityProvider
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		providers = append(providers, p)
	}
	if providers == nil {
		providers = []IdentityProvider{}
	}
	return providers, rows.Err()
}

// ByID returns a single identity provider by ID (secret redacted).
func (s *Service) ByID(id string) (*IdentityProvider, error) {
	row := s.db.QueryRow(
		`SELECT id, name, provider_type, enabled, client_id, tenant_id, domain, issuer_url,
		        metadata_url, entity_id, scopes, auto_provision, default_role_id,
		        group_mapping_enabled, group_mappings, auto_import_groups, created_at, updated_at
		 FROM identity_providers WHERE id = ?`, id,
	)

	p, err := scanProviderRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying identity provider: %w", err)
	}
	return &p, nil
}

// Create creates a new identity provider with encrypted secrets.
func (s *Service) Create(input CreateProviderInput) (*IdentityProvider, error) {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	clientID := strings.TrimSpace(input.ClientID)
	clientSecret := input.ClientSecret
	scopes := "openid profile email"
	metadataURL := input.MetadataURL
	entityID := input.EntityID
	var encryptedSPCert any
	var encryptedSPKey any

	if input.ProviderType == "saml_2_0" {
		clientID = ""
		clientSecret = ""
		scopes = ""

		certPEM, keyPEM, err := samlx.GenerateCredentials(input.Name)
		if err != nil {
			return nil, fmt.Errorf("generating service provider credentials: %w", err)
		}

		encCert, err := s.encryption.Encrypt(certPEM)
		if err != nil {
			return nil, fmt.Errorf("encrypting service provider certificate: %w", err)
		}
		encKey, err := s.encryption.Encrypt(keyPEM)
		if err != nil {
			return nil, fmt.Errorf("encrypting service provider private key: %w", err)
		}

		encryptedSPCert = encCert
		encryptedSPKey = encKey
	} else if input.Scopes != nil {
		scopes = *input.Scopes
	}

	encSecret, err := s.encryption.Encrypt(clientSecret)
	if err != nil {
		return nil, fmt.Errorf("encrypting client secret: %w", err)
	}

	autoProvision := true
	if input.AutoProvision != nil {
		autoProvision = *input.AutoProvision
	}
	groupMappingEnabled := false
	if input.GroupMappingEnabled != nil {
		groupMappingEnabled = *input.GroupMappingEnabled
	}
	autoImportGroups := false
	if input.AutoImportGroups != nil {
		autoImportGroups = *input.AutoImportGroups
	}

	mappingsJSON, err := json.Marshal(input.GroupMappings)
	if err != nil {
		return nil, fmt.Errorf("marshaling group mappings: %w", err)
	}
	if input.GroupMappings == nil {
		mappingsJSON = []byte("[]")
	}

	_, err = s.db.Exec(
		`INSERT INTO identity_providers
		 (id, name, provider_type, enabled, client_id, client_secret, tenant_id, domain, issuer_url,
		  metadata_url, entity_id, sp_certificate, sp_private_key, scopes, auto_provision,
		  default_role_id, group_mapping_enabled, group_mappings, auto_import_groups, created_at, updated_at)
		 VALUES (?, ?, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, input.Name, input.ProviderType, clientID, encSecret, input.TenantID, input.Domain,
		input.IssuerURL, metadataURL, entityID, encryptedSPCert, encryptedSPKey, scopes,
		autoProvision, input.DefaultRoleID, groupMappingEnabled, string(mappingsJSON),
		autoImportGroups, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting identity provider: %w", err)
	}

	return s.ByID(id)
}

// Update partially updates an identity provider.
func (s *Service) Update(id string, input UpdateProviderInput) (*IdentityProvider, error) {
	existing, err := s.ByID(id)
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
		sets = append(sets, "client_secret = ?")
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

	args = append(args, id)
	query := "UPDATE identity_providers SET " + strings.Join(sets, ", ") + " WHERE id = ?"
	if _, err := s.db.Exec(query, args...); err != nil {
		return nil, fmt.Errorf("updating identity provider: %w", err)
	}

	return s.ByID(id)
}

// Delete removes an identity provider.
func (s *Service) Delete(id string) error {
	result, err := s.db.Exec("DELETE FROM identity_providers WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting identity provider: %w", err)
	}
	if db.RowsAffected(result) == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// AllEnabled returns enabled providers for the login page (minimal info only).
func (s *Service) AllEnabled() ([]EnabledProvider, error) {
	rows, err := s.db.Query(
		`SELECT id, name, provider_type FROM identity_providers WHERE enabled = 1 ORDER BY name LIMIT 1000`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying enabled providers: %w", err)
	}
	defer rows.Close()

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

// BeginAuth creates the provider-specific authentication flow response.
func (s *Service) BeginAuth(ctx context.Context, providerID, baseURL string) (*authFlowResult, error) {
	s.CleanupExpiredStates()

	provider, err := s.loadRuntimeProvider(providerID)
	if err != nil {
		return nil, err
	}
	if !provider.Enabled {
		return nil, fmt.Errorf("provider is disabled")
	}

	switch provider.ProviderType {
	case "entra_id", "google", "generic_oidc":
		authURL, err := s.buildOIDCAuthURL(ctx, providerID, baseURL, provider)
		if err != nil {
			return nil, err
		}
		return &authFlowResult{RedirectURL: authURL}, nil
	case "saml_2_0":
		return s.buildSAMLAuthFlow(ctx, providerID, baseURL, provider)
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", provider.ProviderType)
	}
}

// ExchangeAndProvision validates the OIDC callback, exchanges the code, fetches userinfo,
// and provisions/updates the user. Returns the session ID and user.
func (s *Service) ExchangeAndProvision(ctx context.Context, stateParam, code, baseURL string) (string, *auth.User, error) {
	var stateID, providerID, nonce, expiresAt string
	err := s.db.QueryRow(
		`SELECT id, provider_id, nonce, expires_at FROM oidc_states WHERE state = ?`, stateParam,
	).Scan(&stateID, &providerID, &nonce, &expiresAt)
	if err == sql.ErrNoRows {
		return "", nil, fmt.Errorf("invalid state")
	}
	if err != nil {
		return "", nil, fmt.Errorf("querying oidc state: %w", err)
	}

	if _, delErr := s.db.Exec("DELETE FROM oidc_states WHERE id = ?", stateID); delErr != nil {
		slog.Error("failed to delete oidc state", "stateID", stateID, "error", delErr)
	}

	expires, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil || time.Now().UTC().After(expires) {
		return "", nil, fmt.Errorf("state expired")
	}

	provider, err := s.loadRuntimeProvider(providerID)
	if err != nil {
		return "", nil, err
	}
	if !provider.Enabled {
		return "", nil, fmt.Errorf("provider is disabled")
	}

	clientSecret, err := s.encryption.Decrypt(provider.EncryptedClientSecret)
	if err != nil {
		return "", nil, fmt.Errorf("decrypting client secret: %w", err)
	}

	redirectURL := baseURL + "/api/identity-providers/callback"
	scopeList := strings.Fields(provider.Scopes)

	var cfg oidc.ProviderConfig
	switch provider.ProviderType {
	case "entra_id":
		cfg = oidc.BuildEntraConfig(provider.ClientID, clientSecret, provider.nullTenantID(), redirectURL, scopeList)
	case "google":
		cfg = oidc.BuildGoogleConfig(provider.ClientID, clientSecret, redirectURL, scopeList)
	case "generic_oidc":
		if !provider.IssuerURL.Valid || provider.IssuerURL.String == "" {
			return "", nil, fmt.Errorf("issuer URL is required for generic OIDC providers")
		}
		cfg, err = oidc.BuildGenericConfig(ctx, provider.ClientID, clientSecret, provider.IssuerURL.String, redirectURL, scopeList)
		if err != nil {
			return "", nil, fmt.Errorf("building generic oidc config: %w", err)
		}
	default:
		return "", nil, fmt.Errorf("unsupported provider type: %s", provider.ProviderType)
	}

	exchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	token, err := cfg.OAuth2.Exchange(exchCtx, code)
	if err != nil {
		return "", nil, fmt.Errorf("exchanging code: %w", err)
	}

	userInfo, err := oidc.FetchUserInfo(exchCtx, token, cfg.UserInfo)
	if err != nil {
		return "", nil, fmt.Errorf("fetching userinfo: %w", err)
	}
	if userInfo.Sub == "" {
		return "", nil, fmt.Errorf("userinfo missing sub claim")
	}

	user, err := s.findOrProvisionUser(providerID, externalUserInfo{
		Subject:        userInfo.Sub,
		Email:          userInfo.Email,
		Name:           userInfo.Name,
		Groups:         userInfo.Groups,
		UsernamePrefix: "oidc_",
	}, provider.AutoProvision, provider.DefaultRoleID)
	if err != nil {
		return "", nil, fmt.Errorf("provisioning user: %w", err)
	}

	if len(userInfo.Groups) > 0 {
		if provider.AutoImportGroups {
			s.autoImportGroups(user.ID, userInfo.Groups, "oidc")
		}
		if provider.GroupMappingEnabled {
			s.applyGroupMappings(user.ID, provider.GroupMappingsJSON, userInfo.Groups)
		}
	}

	return s.createSessionForUser(user)
}

// HandleSAMLACS validates the SAML response and provisions or links the user.
func (s *Service) HandleSAMLACS(ctx context.Context, providerID, baseURL string, r *http.Request) (string, *auth.User, error) {
	if err := r.ParseForm(); err != nil {
		return "", nil, fmt.Errorf("parsing saml response form: %w", err)
	}

	provider, err := s.loadRuntimeProvider(providerID)
	if err != nil {
		return "", nil, err
	}
	if provider.ProviderType != "saml_2_0" {
		return "", nil, fmt.Errorf("provider is not a saml provider")
	}
	if !provider.Enabled {
		return "", nil, fmt.Errorf("provider is disabled")
	}

	relayState := strings.TrimSpace(r.Form.Get("RelayState"))
	if relayState == "" {
		return "", nil, fmt.Errorf("relay state is required")
	}

	requestID, err := s.consumeSAMLRequest(providerID, relayState)
	if err != nil {
		return "", nil, err
	}

	sp, err := s.buildSAMLServiceProvider(ctx, baseURL, providerID, provider)
	if err != nil {
		return "", nil, err
	}

	assertion, err := sp.ParseResponse(r, []string{requestID})
	if err != nil {
		return "", nil, fmt.Errorf("parsing saml response: %w", err)
	}

	info := externalUserInfoFromSAML(assertion)
	if info.Subject == "" {
		return "", nil, fmt.Errorf("saml assertion missing subject")
	}

	user, err := s.findOrProvisionUser(providerID, info, provider.AutoProvision, provider.DefaultRoleID)
	if err != nil {
		return "", nil, fmt.Errorf("provisioning user: %w", err)
	}

	if len(info.Groups) > 0 {
		if provider.AutoImportGroups {
			s.autoImportGroups(user.ID, info.Groups, "saml")
		}
		if provider.GroupMappingEnabled {
			s.applyGroupMappings(user.ID, provider.GroupMappingsJSON, info.Groups)
		}
	}

	return s.createSessionForUser(user)
}

// SAMLMetadata returns this service provider's metadata document.
func (s *Service) SAMLMetadata(ctx context.Context, providerID, baseURL string) ([]byte, error) {
	provider, err := s.loadRuntimeProvider(providerID)
	if err != nil {
		return nil, err
	}
	if provider.ProviderType != "saml_2_0" {
		return nil, fmt.Errorf("provider is not a saml provider")
	}

	sp, err := s.buildSAMLServiceProvider(ctx, baseURL, providerID, provider)
	if err != nil {
		return nil, err
	}
	return samlx.MetadataXML(sp)
}

// CleanupExpiredStates removes expired external-auth state.
func (s *Service) CleanupExpiredStates() {
	now := time.Now().UTC().Format(time.RFC3339)

	if _, err := s.db.Exec("DELETE FROM oidc_states WHERE expires_at < ?", now); err != nil {
		slog.Error("failed to cleanup expired oidc states", "error", err)
	}
	if _, err := s.db.Exec("DELETE FROM saml_requests WHERE expires_at < ?", now); err != nil {
		slog.Error("failed to cleanup expired saml requests", "error", err)
	}
}

// TestConnection verifies that the provider's configuration is valid.
func (s *Service) TestConnection(ctx context.Context, providerID string) error {
	provider, err := s.loadRuntimeProvider(providerID)
	if err != nil {
		return err
	}

	if provider.ProviderType == "saml_2_0" {
		if !provider.MetadataURL.Valid || provider.MetadataURL.String == "" {
			return fmt.Errorf("metadata url is required for saml providers")
		}
		return samlx.TestConnection(ctx, provider.MetadataURL.String)
	}

	clientSecret, err := s.encryption.Decrypt(provider.EncryptedClientSecret)
	if err != nil {
		return fmt.Errorf("decrypting client secret: %w", err)
	}

	return oidc.TestConnection(
		ctx,
		provider.ProviderType,
		provider.ClientID,
		clientSecret,
		provider.nullTenantID(),
		provider.nullIssuerURL(),
	)
}

// FetchProviderGroups fetches groups from the provider's API (Entra ID Graph API or Google Admin SDK).
func (s *Service) FetchProviderGroups(ctx context.Context, providerID string) ([]ProviderGroupInfo, error) {
	provider, err := s.loadRuntimeProvider(providerID)
	if err != nil {
		return nil, err
	}

	clientSecret, err := s.encryption.Decrypt(provider.EncryptedClientSecret)
	if err != nil {
		return nil, fmt.Errorf("decrypting client secret: %w", err)
	}

	var groups []oidc.GroupInfo
	switch provider.ProviderType {
	case "entra_id":
		groups, err = oidc.FetchEntraGroups(ctx, provider.ClientID, clientSecret, provider.nullTenantID())
	case "google":
		groups, err = oidc.FetchGoogleGroups(ctx, provider.ClientID, clientSecret, provider.nullDomain())
	case "generic_oidc":
		return nil, fmt.Errorf("fetching groups is not supported for generic OIDC providers")
	case "saml_2_0":
		return nil, fmt.Errorf("fetching groups is not supported for SAML 2.0 providers")
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", provider.ProviderType)
	}
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
func (s *Service) applyGroupMappings(userID, mappingsJSON string, providerGroups []string) {
	var mappings []GroupMapping
	if err := json.Unmarshal([]byte(mappingsJSON), &mappings); err != nil {
		slog.Warn("failed to unmarshal group mappings", "userID", userID, "error", err)
		return
	}

	providerSet := make(map[string]bool, len(providerGroups))
	for _, g := range providerGroups {
		providerSet[g] = true
	}

	now := time.Now().UTC().Format(time.RFC3339)
	for _, m := range mappings {
		if providerSet[m.ProviderGroup] {
			gmID := xid.New().String()
			if _, gmErr := s.db.Exec(
				`INSERT OR IGNORE INTO group_members (id, group_id, user_id, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?)`,
				gmID, m.McharborGroupID, userID, now, now,
			); gmErr != nil {
				slog.Error("failed to assign group", "userID", userID, "groupID", m.McharborGroupID, "error", gmErr)
			}
		} else {
			if _, rmErr := s.db.Exec(
				`DELETE FROM group_members WHERE group_id = ? AND user_id = ?`,
				m.McharborGroupID, userID,
			); rmErr != nil {
				slog.Error("failed to remove group membership", "userID", userID, "groupID", m.McharborGroupID, "error", rmErr)
			}
		}
	}
}

func (s *Service) buildOIDCAuthURL(ctx context.Context, providerID, baseURL string, provider runtimeProvider) (string, error) {
	clientSecret, err := s.encryption.Decrypt(provider.EncryptedClientSecret)
	if err != nil {
		return "", fmt.Errorf("decrypting client secret: %w", err)
	}

	redirectURL := baseURL + "/api/identity-providers/callback"
	scopeList := strings.Fields(provider.Scopes)

	var cfg oidc.ProviderConfig
	switch provider.ProviderType {
	case "entra_id":
		cfg = oidc.BuildEntraConfig(provider.ClientID, clientSecret, provider.nullTenantID(), redirectURL, scopeList)
	case "google":
		cfg = oidc.BuildGoogleConfig(provider.ClientID, clientSecret, redirectURL, scopeList)
	case "generic_oidc":
		if !provider.IssuerURL.Valid || provider.IssuerURL.String == "" {
			return "", fmt.Errorf("issuer URL is required for generic OIDC providers")
		}
		cfg, err = oidc.BuildGenericConfig(ctx, provider.ClientID, clientSecret, provider.IssuerURL.String, redirectURL, scopeList)
		if err != nil {
			return "", fmt.Errorf("building generic oidc config: %w", err)
		}
	default:
		return "", fmt.Errorf("unsupported provider type: %s", provider.ProviderType)
	}

	state, err := randomHex(32)
	if err != nil {
		return "", fmt.Errorf("generating state: %w", err)
	}
	nonce, err := randomHex(16)
	if err != nil {
		return "", fmt.Errorf("generating nonce: %w", err)
	}

	stateID := xid.New().String()
	now := time.Now().UTC()
	expiresAt := now.Add(stateExpiry).Format(time.RFC3339)

	_, err = s.db.Exec(
		`INSERT INTO oidc_states (id, provider_id, state, nonce, redirect_url, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		stateID, providerID, state, nonce, redirectURL, expiresAt, now.Format(time.RFC3339),
	)
	if err != nil {
		return "", fmt.Errorf("inserting oidc state: %w", err)
	}

	return cfg.OAuth2.AuthCodeURL(state, oauth2.SetAuthURLParam("nonce", nonce)), nil
}

func (s *Service) buildSAMLAuthFlow(ctx context.Context, providerID, baseURL string, provider runtimeProvider) (*authFlowResult, error) {
	sp, err := s.buildSAMLServiceProvider(ctx, baseURL, providerID, provider)
	if err != nil {
		return nil, err
	}

	relayState, err := randomHex(16)
	if err != nil {
		return nil, fmt.Errorf("generating relay state: %w", err)
	}

	if destination := sp.GetSSOBindingLocation(saml.HTTPRedirectBinding); destination != "" {
		req, err := sp.MakeAuthenticationRequest(destination, saml.HTTPRedirectBinding, saml.HTTPPostBinding)
		if err != nil {
			return nil, fmt.Errorf("creating redirect authn request: %w", err)
		}
		if err := s.storeSAMLRequest(providerID, req.ID, relayState); err != nil {
			return nil, err
		}
		redirectURL, err := req.Redirect(relayState, sp)
		if err != nil {
			return nil, fmt.Errorf("building redirect authn request: %w", err)
		}
		return &authFlowResult{RedirectURL: redirectURL.String()}, nil
	}

	if destination := sp.GetSSOBindingLocation(saml.HTTPPostBinding); destination != "" {
		req, err := sp.MakeAuthenticationRequest(destination, saml.HTTPPostBinding, saml.HTTPPostBinding)
		if err != nil {
			return nil, fmt.Errorf("creating post authn request: %w", err)
		}
		if err := s.storeSAMLRequest(providerID, req.ID, relayState); err != nil {
			return nil, err
		}
		return &authFlowResult{HTMLForm: string(req.Post(relayState))}, nil
	}

	return nil, fmt.Errorf("identity provider metadata does not expose a supported SSO binding")
}

func (s *Service) buildSAMLServiceProvider(ctx context.Context, baseURL, providerID string, provider runtimeProvider) (*saml.ServiceProvider, error) {
	if !provider.MetadataURL.Valid || provider.MetadataURL.String == "" {
		return nil, fmt.Errorf("metadata url is required for saml providers")
	}

	certPEM, err := decryptOptionalString(s.encryption, provider.EncryptedSPCert)
	if err != nil {
		return nil, fmt.Errorf("decrypting service provider certificate: %w", err)
	}
	keyPEM, err := decryptOptionalString(s.encryption, provider.EncryptedSPKey)
	if err != nil {
		return nil, fmt.Errorf("decrypting service provider private key: %w", err)
	}
	if certPEM == "" || keyPEM == "" {
		return nil, fmt.Errorf("service provider credentials are missing")
	}

	entityID := ""
	if provider.EntityID.Valid {
		entityID = provider.EntityID.String
	}

	spCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	sp, err := samlx.BuildServiceProvider(spCtx, baseURL, providerID, provider.MetadataURL.String, entityID, certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("building saml service provider: %w", err)
	}
	return sp, nil
}

func (s *Service) storeSAMLRequest(providerID, requestID, relayState string) error {
	now := time.Now().UTC()
	expiresAt := now.Add(stateExpiry).Format(time.RFC3339)

	_, err := s.db.Exec(
		`INSERT INTO saml_requests (id, provider_id, request_id, relay_state, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		xid.New().String(), providerID, requestID, relayState, expiresAt, now.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("inserting saml request: %w", err)
	}
	return nil
}

func (s *Service) consumeSAMLRequest(providerID, relayState string) (string, error) {
	var requestID, expiresAt string
	err := s.db.QueryRow(
		`SELECT request_id, expires_at FROM saml_requests WHERE provider_id = ? AND relay_state = ?`,
		providerID, relayState,
	).Scan(&requestID, &expiresAt)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("invalid relay state")
	}
	if err != nil {
		return "", fmt.Errorf("querying saml request: %w", err)
	}

	if _, delErr := s.db.Exec(
		`DELETE FROM saml_requests WHERE provider_id = ? AND relay_state = ?`,
		providerID, relayState,
	); delErr != nil {
		slog.Error("failed to delete saml request", "providerID", providerID, "relayState", relayState, "error", delErr)
	}

	expires, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil || time.Now().UTC().After(expires) {
		return "", fmt.Errorf("saml request expired")
	}

	return requestID, nil
}

func (s *Service) createSessionForUser(user *auth.User) (string, *auth.User, error) {
	sessionID, err := s.authSvc.CreateSession(user.ID)
	if err != nil {
		return "", nil, fmt.Errorf("creating session: %w", err)
	}

	if _, loginErr := s.db.Exec(
		"UPDATE users SET last_login = ? WHERE id = ?",
		time.Now().UTC().Format(time.RFC3339),
		user.ID,
	); loginErr != nil {
		slog.Error("failed to update last login", "userID", user.ID, "error", loginErr)
	}

	return sessionID, user, nil
}

func (s *Service) loadRuntimeProvider(providerID string) (runtimeProvider, error) {
	var provider runtimeProvider
	err := s.db.QueryRow(
		`SELECT client_id, client_secret, provider_type, tenant_id, domain, issuer_url, metadata_url,
		        entity_id, sp_certificate, sp_private_key, scopes, enabled, auto_provision,
		        default_role_id, group_mapping_enabled, group_mappings, auto_import_groups
		 FROM identity_providers WHERE id = ?`,
		providerID,
	).Scan(
		&provider.ClientID,
		&provider.EncryptedClientSecret,
		&provider.ProviderType,
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
	)
	if err == sql.ErrNoRows {
		return provider, fmt.Errorf("provider not found")
	}
	if err != nil {
		return provider, fmt.Errorf("querying provider: %w", err)
	}
	return provider, nil
}

// findOrProvisionUser looks up a user by external_id or creates a new one.
func (s *Service) findOrProvisionUser(providerID string, info externalUserInfo, autoProvision bool, defaultRoleID sql.NullString) (*auth.User, error) {
	var userID, username string
	var displayName, email sql.NullString

	err := s.db.QueryRow(
		`SELECT id, username, display_name, email FROM users
		 WHERE external_id = ? AND identity_provider_id = ?`,
		info.Subject, providerID,
	).Scan(&userID, &username, &displayName, &email)

	if err == nil {
		now := time.Now().UTC().Format(time.RFC3339)
		if _, updErr := s.db.Exec(
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
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("querying user by external_id: %w", err)
	}

	if info.Email != "" {
		err = s.db.QueryRow(
			`SELECT id, username, display_name, email FROM users WHERE email = ?`,
			info.Email,
		).Scan(&userID, &username, &displayName, &email)
		if err == nil {
			now := time.Now().UTC().Format(time.RFC3339)
			if _, linkErr := s.db.Exec(
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
		if err != sql.ErrNoRows {
			return nil, fmt.Errorf("querying user by email: %w", err)
		}
	}

	if !autoProvision {
		return nil, fmt.Errorf("user not found and auto-provisioning is disabled")
	}

	userID = xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	username = info.Email
	if username == "" {
		subLen := min(len(info.Subject), 8)
		prefix := info.UsernamePrefix
		if prefix == "" {
			prefix = "idp_"
		}
		username = prefix + info.Subject[:subLen]
	}

	randomPass := make([]byte, 32)
	if _, err := rand.Read(randomPass); err != nil {
		return nil, fmt.Errorf("generating random password: %w", err)
	}
	passwordHash, err := auth.HashPassword(hex.EncodeToString(randomPass))
	if err != nil {
		return nil, fmt.Errorf("hashing random password: %w", err)
	}

	_, err = s.db.Exec(
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
		if _, roleErr := s.db.Exec(
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

// autoImportGroups automatically creates McHarbor groups from provider claims
// and syncs the user's membership for the given identity source.
func (s *Service) autoImportGroups(userID string, providerGroups []string, source string) {
	now := time.Now().UTC().Format(time.RFC3339)
	shouldBeIn := make(map[string]bool, len(providerGroups))

	for _, groupName := range providerGroups {
		var groupID string
		err := s.db.QueryRow(`SELECT id FROM groups WHERE name = ?`, groupName).Scan(&groupID)

		if err == sql.ErrNoRows {
			groupID = xid.New().String()
			if _, createErr := s.db.Exec(
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
		if _, gmErr := s.db.Exec(
			`INSERT OR IGNORE INTO group_members (id, group_id, user_id, source, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			gmID, groupID, userID, source, now, now,
		); gmErr != nil {
			slog.Error("failed to assign auto-imported group", "userID", userID, "groupID", groupID, "error", gmErr)
		}
	}

	rows, err := s.db.Query(
		`SELECT group_id FROM group_members WHERE user_id = ? AND source = ?`,
		userID, source,
	)
	if err != nil {
		slog.Error("failed to query identity group memberships", "userID", userID, "source", source, "error", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var groupID string
		if err := rows.Scan(&groupID); err != nil {
			slog.Error("failed to scan group membership", "error", err)
			continue
		}
		if !shouldBeIn[groupID] {
			if _, rmErr := s.db.Exec(
				`DELETE FROM group_members WHERE group_id = ? AND user_id = ? AND source = ?`,
				groupID, userID, source,
			); rmErr != nil {
				slog.Error("failed to remove stale identity group membership", "userID", userID, "groupID", groupID, "source", source, "error", rmErr)
			}
		}
	}
}

// scanProvider scans an IdentityProvider from a rows result (no client_secret).
func scanProvider(rows *sql.Rows) (IdentityProvider, error) {
	var p IdentityProvider
	var tenantID, domain, issuerURL, metadataURL, entityID, defaultRoleID sql.NullString
	var enabled, autoProvision, groupMappingEnabled, autoImportGroups bool
	var mappingsJSON string

	err := rows.Scan(
		&p.ID, &p.Name, &p.ProviderType, &enabled, &p.ClientID,
		&tenantID, &domain, &issuerURL, &metadataURL, &entityID, &p.Scopes, &autoProvision,
		&defaultRoleID, &groupMappingEnabled, &mappingsJSON, &autoImportGroups, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return p, fmt.Errorf("scanning identity provider: %w", err)
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

	if err := json.Unmarshal([]byte(mappingsJSON), &p.GroupMappings); err != nil {
		p.GroupMappings = []GroupMapping{}
	}
	if p.GroupMappings == nil {
		p.GroupMappings = []GroupMapping{}
	}

	return p, nil
}

// scanProviderRow scans a single row.
func scanProviderRow(row *sql.Row) (IdentityProvider, error) {
	var p IdentityProvider
	var tenantID, domain, issuerURL, metadataURL, entityID, defaultRoleID sql.NullString
	var enabled, autoProvision, groupMappingEnabled, autoImportGroups bool
	var mappingsJSON string

	err := row.Scan(
		&p.ID, &p.Name, &p.ProviderType, &enabled, &p.ClientID,
		&tenantID, &domain, &issuerURL, &metadataURL, &entityID, &p.Scopes, &autoProvision,
		&defaultRoleID, &groupMappingEnabled, &mappingsJSON, &autoImportGroups, &p.CreatedAt, &p.UpdatedAt,
	)
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

	if err := json.Unmarshal([]byte(mappingsJSON), &p.GroupMappings); err != nil {
		p.GroupMappings = []GroupMapping{}
	}
	if p.GroupMappings == nil {
		p.GroupMappings = []GroupMapping{}
	}

	return p, nil
}

func decryptOptionalString(enc *encryption.Service, value sql.NullString) (string, error) {
	if !value.Valid || value.String == "" {
		return "", nil
	}
	return enc.Decrypt(value.String)
}

func externalUserInfoFromSAML(assertion *saml.Assertion) externalUserInfo {
	info := externalUserInfo{
		UsernamePrefix: "saml_",
	}
	if assertion == nil {
		return info
	}

	if assertion.Subject != nil && assertion.Subject.NameID != nil {
		info.Subject = strings.TrimSpace(assertion.Subject.NameID.Value)
		if strings.Contains(info.Subject, "@") {
			info.Email = info.Subject
		}
	}

	var givenName string
	var surname string
	groupSet := make(map[string]struct{})

	for _, statement := range assertion.AttributeStatements {
		for _, attribute := range statement.Attributes {
			values := attributeValues(attribute.Values)
			if len(values) == 0 {
				continue
			}

			name := strings.ToLower(strings.TrimSpace(attribute.Name))
			friendlyName := strings.ToLower(strings.TrimSpace(attribute.FriendlyName))

			switch {
			case matchesAttribute(name, friendlyName, "email", "mail", "emailaddress"):
				if info.Email == "" {
					info.Email = values[0]
				}
			case matchesAttribute(name, friendlyName, "name", "displayname", "cn", "commonname"):
				if info.Name == "" {
					info.Name = values[0]
				}
			case matchesAttribute(name, friendlyName, "givenname", "firstname", "first_name"):
				givenName = values[0]
			case matchesAttribute(name, friendlyName, "surname", "lastname", "familyname", "last_name"):
				surname = values[0]
			case matchesAttribute(name, friendlyName, "groups", "memberof", "roles", "role"):
				for _, value := range values {
					groupSet[value] = struct{}{}
				}
			}
		}
	}

	if info.Name == "" {
		switch {
		case givenName != "" && surname != "":
			info.Name = givenName + " " + surname
		case givenName != "":
			info.Name = givenName
		case info.Email != "":
			info.Name = info.Email
		}
	}

	for group := range groupSet {
		info.Groups = append(info.Groups, group)
	}

	return info
}

func attributeValues(values []saml.AttributeValue) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value.Value)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func matchesAttribute(name, friendlyName string, aliases ...string) bool {
	for _, alias := range aliases {
		if name == alias || friendlyName == alias {
			return true
		}
	}
	return false
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

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func nullableStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
