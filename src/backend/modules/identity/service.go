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
	"strings"
	"time"

	"github.com/rs/xid"
	"golang.org/x/oauth2"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/oidc"
)

const stateExpiry = 10 * time.Minute

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
		`SELECT id, name, provider_type, enabled, client_id, tenant_id, domain,
		        scopes, auto_provision, default_role_id, group_mapping_enabled,
		        group_mappings, auto_import_groups, created_at, updated_at
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
		`SELECT id, name, provider_type, enabled, client_id, tenant_id, domain,
		        scopes, auto_provision, default_role_id, group_mapping_enabled,
		        group_mappings, auto_import_groups, created_at, updated_at
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

// Create creates a new identity provider with encrypted client secret.
func (s *Service) Create(input CreateProviderInput) (*IdentityProvider, error) {
	encSecret, err := s.encryption.Encrypt(input.ClientSecret)
	if err != nil {
		return nil, fmt.Errorf("encrypting client secret: %w", err)
	}

	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	scopes := "openid profile email"
	if input.Scopes != nil {
		scopes = *input.Scopes
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
		 (id, name, provider_type, enabled, client_id, client_secret, tenant_id, domain,
		  scopes, auto_provision, default_role_id, group_mapping_enabled, group_mappings,
		  auto_import_groups, created_at, updated_at)
		 VALUES (?, ?, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, input.Name, input.ProviderType, input.ClientID, encSecret,
		input.TenantID, input.Domain, scopes, autoProvision, input.DefaultRoleID,
		groupMappingEnabled, string(mappingsJSON), autoImportGroups, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting identity provider: %w", err)
	}

	return s.ByID(id)
}

// Update partially updates an identity provider.
func (s *Service) Update(id string, input UpdateProviderInput) (*IdentityProvider, error) {
	// Check exists
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

// BuildAuthURL creates an OIDC state and returns the authorization URL.
// Also cleans up expired states opportunistically.
func (s *Service) BuildAuthURL(providerID, baseURL string) (string, error) {
	s.CleanupExpiredStates()
	// Load provider with secret
	var clientID, encSecret, providerType, scopes string
	var tenantID, domain sql.NullString
	var enabled bool

	err := s.db.QueryRow(
		`SELECT client_id, client_secret, provider_type, tenant_id, domain, scopes, enabled
		 FROM identity_providers WHERE id = ?`, providerID,
	).Scan(&clientID, &encSecret, &providerType, &tenantID, &domain, &scopes, &enabled)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("provider not found")
	}
	if err != nil {
		return "", fmt.Errorf("querying provider: %w", err)
	}
	if !enabled {
		return "", fmt.Errorf("provider is disabled")
	}

	clientSecret, err := s.encryption.Decrypt(encSecret)
	if err != nil {
		return "", fmt.Errorf("decrypting client secret: %w", err)
	}

	redirectURL := baseURL + "/api/identity-providers/callback"
	scopeList := strings.Fields(scopes)

	var cfg oidc.ProviderConfig
	switch providerType {
	case "entra_id":
		tid := ""
		if tenantID.Valid {
			tid = tenantID.String
		}
		cfg = oidc.BuildEntraConfig(clientID, clientSecret, tid, redirectURL, scopeList)
	case "google":
		cfg = oidc.BuildGoogleConfig(clientID, clientSecret, redirectURL, scopeList)
	default:
		return "", fmt.Errorf("unsupported provider type: %s", providerType)
	}

	// Generate state and nonce
	state, err := randomHex(32)
	if err != nil {
		return "", fmt.Errorf("generating state: %w", err)
	}
	nonce, err := randomHex(16)
	if err != nil {
		return "", fmt.Errorf("generating nonce: %w", err)
	}

	// Store state in DB
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

	authURL := cfg.OAuth2.AuthCodeURL(state, oauth2.SetAuthURLParam("nonce", nonce))
	return authURL, nil
}

// ExchangeAndProvision validates the OIDC callback, exchanges the code, fetches userinfo,
// and provisions/updates the user. Returns the session ID and user.
func (s *Service) ExchangeAndProvision(ctx context.Context, stateParam, code, baseURL string) (string, *auth.User, error) {
	// Validate state
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

	// Delete state (single-use)
	if _, delErr := s.db.Exec("DELETE FROM oidc_states WHERE id = ?", stateID); delErr != nil {
		slog.Error("failed to delete oidc state", "stateID", stateID, "error", delErr)
	}

	// Check expiry
	expires, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil || time.Now().UTC().After(expires) {
		return "", nil, fmt.Errorf("state expired")
	}

	// Load provider with secret
	var clientID, encSecret, providerType, scopes string
	var tenantIDN, domainN sql.NullString
	var enabled, autoProvision, groupMappingEnabled, autoImportGroups bool
	var defaultRoleID sql.NullString
	var groupMappingsJSON string

	err = s.db.QueryRow(
		`SELECT client_id, client_secret, provider_type, tenant_id, domain, scopes,
		        enabled, auto_provision, default_role_id, group_mapping_enabled, group_mappings,
		        auto_import_groups
		 FROM identity_providers WHERE id = ?`, providerID,
	).Scan(&clientID, &encSecret, &providerType, &tenantIDN, &domainN, &scopes,
		&enabled, &autoProvision, &defaultRoleID, &groupMappingEnabled, &groupMappingsJSON,
		&autoImportGroups)
	if err != nil {
		return "", nil, fmt.Errorf("querying provider: %w", err)
	}
	if !enabled {
		return "", nil, fmt.Errorf("provider is disabled")
	}

	clientSecret, err := s.encryption.Decrypt(encSecret)
	if err != nil {
		return "", nil, fmt.Errorf("decrypting client secret: %w", err)
	}

	redirectURL := baseURL + "/api/identity-providers/callback"
	scopeList := strings.Fields(scopes)

	var cfg oidc.ProviderConfig
	switch providerType {
	case "entra_id":
		tid := ""
		if tenantIDN.Valid {
			tid = tenantIDN.String
		}
		cfg = oidc.BuildEntraConfig(clientID, clientSecret, tid, redirectURL, scopeList)
	case "google":
		cfg = oidc.BuildGoogleConfig(clientID, clientSecret, redirectURL, scopeList)
	default:
		return "", nil, fmt.Errorf("unsupported provider type: %s", providerType)
	}

	// Exchange code for token
	exchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	token, err := cfg.OAuth2.Exchange(exchCtx, code)
	if err != nil {
		return "", nil, fmt.Errorf("exchanging code: %w", err)
	}

	// Fetch user info
	userInfo, err := oidc.FetchUserInfo(exchCtx, token, cfg.UserInfo)
	if err != nil {
		return "", nil, fmt.Errorf("fetching userinfo: %w", err)
	}

	if userInfo.Sub == "" {
		return "", nil, fmt.Errorf("userinfo missing sub claim")
	}

	// Find or create user by external_id + provider_id
	user, err := s.findOrProvisionUser(providerID, userInfo, autoProvision, defaultRoleID)
	if err != nil {
		return "", nil, fmt.Errorf("provisioning user: %w", err)
	}

	// Sync groups from provider claims
	if len(userInfo.Groups) > 0 {
		if autoImportGroups {
			s.autoImportGroups(user.ID, userInfo.Groups)
		}
		if groupMappingEnabled {
			s.applyGroupMappings(user.ID, groupMappingsJSON, userInfo.Groups)
		}
	}

	// Create session
	sessionID, err := s.authSvc.CreateSession(user.ID)
	if err != nil {
		return "", nil, fmt.Errorf("creating session: %w", err)
	}

	// Update last login
	if _, loginErr := s.db.Exec("UPDATE users SET last_login = ? WHERE id = ?",
		time.Now().UTC().Format(time.RFC3339), user.ID); loginErr != nil {
		slog.Error("failed to update last login", "userID", user.ID, "error", loginErr)
	}

	return sessionID, user, nil
}

// CleanupExpiredStates removes expired OIDC states.
func (s *Service) CleanupExpiredStates() {
	if _, err := s.db.Exec("DELETE FROM oidc_states WHERE expires_at < ?", time.Now().UTC().Format(time.RFC3339)); err != nil {
		slog.Error("failed to cleanup expired oidc states", "error", err)
	}
}

// findOrProvisionUser looks up a user by external_id or creates a new one.
func (s *Service) findOrProvisionUser(providerID string, info *oidc.UserInfo, autoProvision bool, defaultRoleID sql.NullString) (*auth.User, error) {
	// Check for existing user by external_id + provider
	var userID, username string
	var displayName, email sql.NullString

	err := s.db.QueryRow(
		`SELECT id, username, display_name, email FROM users
		 WHERE external_id = ? AND identity_provider_id = ?`,
		info.Sub, providerID,
	).Scan(&userID, &username, &displayName, &email)

	if err == nil {
		// Existing user — update display name and email if changed
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

	// Also check by email match (link existing local accounts)
	if info.Email != "" {
		err = s.db.QueryRow(
			`SELECT id, username, display_name, email FROM users WHERE email = ?`,
			info.Email,
		).Scan(&userID, &username, &displayName, &email)
		if err == nil {
			// Link existing user to this provider
			now := time.Now().UTC().Format(time.RFC3339)
			if _, linkErr := s.db.Exec(
				`UPDATE users SET external_id = ?, identity_provider_id = ?, updated_at = ? WHERE id = ?`,
				info.Sub, providerID, now, userID,
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

	// Create new user
	userID = xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	// Generate a username from email or sub
	username = info.Email
	if username == "" {
		subLen := min(len(info.Sub), 8)
		username = "oidc_" + info.Sub[:subLen]
	}

	// Generate a random password hash (user cannot log in locally with OIDC accounts)
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
		info.Sub, providerID, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting user: %w", err)
	}

	// Assign default role if configured
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
// and syncs the user's membership. Only touches oidc-sourced memberships.
func (s *Service) autoImportGroups(userID string, providerGroups []string) {
	now := time.Now().UTC().Format(time.RFC3339)

	// Build set of group IDs the user should belong to
	shouldBeIn := make(map[string]bool, len(providerGroups))

	for _, groupName := range providerGroups {
		// Find or create the McHarbor group by name
		var groupID string
		err := s.db.QueryRow(
			`SELECT id FROM groups WHERE name = ?`, groupName,
		).Scan(&groupID)

		if err == sql.ErrNoRows {
			// Create the group
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

		// Ensure membership with source=oidc
		gmID := xid.New().String()
		if _, gmErr := s.db.Exec(
			`INSERT OR IGNORE INTO group_members (id, group_id, user_id, source, created_at, updated_at)
			 VALUES (?, ?, ?, 'oidc', ?, ?)`,
			gmID, groupID, userID, now, now,
		); gmErr != nil {
			slog.Error("failed to assign auto-imported group", "userID", userID, "groupID", groupID, "error", gmErr)
		}
	}

	// Remove oidc-sourced memberships that are no longer in provider claims
	rows, err := s.db.Query(
		`SELECT group_id FROM group_members WHERE user_id = ? AND source = 'oidc'`, userID,
	)
	if err != nil {
		slog.Error("failed to query oidc group memberships", "userID", userID, "error", err)
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
				`DELETE FROM group_members WHERE group_id = ? AND user_id = ? AND source = 'oidc'`,
				groupID, userID,
			); rmErr != nil {
				slog.Error("failed to remove stale oidc group membership", "userID", userID, "groupID", groupID, "error", rmErr)
			}
		}
	}
}

// TestConnection verifies that the provider's OIDC credentials are valid.
func (s *Service) TestConnection(ctx context.Context, providerID string) error {
	var clientID, encSecret, providerType string
	var tenantID sql.NullString

	err := s.db.QueryRow(
		`SELECT client_id, client_secret, provider_type, tenant_id
		 FROM identity_providers WHERE id = ?`, providerID,
	).Scan(&clientID, &encSecret, &providerType, &tenantID)
	if err == sql.ErrNoRows {
		return fmt.Errorf("provider not found")
	}
	if err != nil {
		return fmt.Errorf("querying provider: %w", err)
	}

	clientSecret, err := s.encryption.Decrypt(encSecret)
	if err != nil {
		return fmt.Errorf("decrypting client secret: %w", err)
	}

	tid := ""
	if tenantID.Valid {
		tid = tenantID.String
	}

	return oidc.TestConnection(ctx, providerType, clientID, clientSecret, tid)
}

// FetchProviderGroups fetches groups from the provider's API (Entra ID Graph API or Google Admin SDK).
func (s *Service) FetchProviderGroups(ctx context.Context, providerID string) ([]ProviderGroupInfo, error) {
	var clientID, encSecret, providerType string
	var tenantID, domain sql.NullString

	err := s.db.QueryRow(
		`SELECT client_id, client_secret, provider_type, tenant_id, domain
		 FROM identity_providers WHERE id = ?`, providerID,
	).Scan(&clientID, &encSecret, &providerType, &tenantID, &domain)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("provider not found")
	}
	if err != nil {
		return nil, fmt.Errorf("querying provider: %w", err)
	}

	clientSecret, err := s.encryption.Decrypt(encSecret)
	if err != nil {
		return nil, fmt.Errorf("decrypting client secret: %w", err)
	}

	var groups []oidc.GroupInfo
	switch providerType {
	case "entra_id":
		tid := ""
		if tenantID.Valid {
			tid = tenantID.String
		}
		groups, err = oidc.FetchEntraGroups(ctx, clientID, clientSecret, tid)
	case "google":
		dom := ""
		if domain.Valid {
			dom = domain.String
		}
		groups, err = oidc.FetchGoogleGroups(ctx, clientID, clientSecret, dom)
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", providerType)
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
// It adds the user to mapped groups they belong to on the provider side,
// and removes them from mapped groups they no longer belong to.
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
			// User has this provider group — ensure membership
			gmID := xid.New().String()
			if _, gmErr := s.db.Exec(
				`INSERT OR IGNORE INTO group_members (id, group_id, user_id, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?)`,
				gmID, m.McharborGroupID, userID, now, now,
			); gmErr != nil {
				slog.Error("failed to assign group", "userID", userID, "groupID", m.McharborGroupID, "error", gmErr)
			}
		} else {
			// User no longer has this provider group — remove membership
			if _, rmErr := s.db.Exec(
				`DELETE FROM group_members WHERE group_id = ? AND user_id = ?`,
				m.McharborGroupID, userID,
			); rmErr != nil {
				slog.Error("failed to remove group membership", "userID", userID, "groupID", m.McharborGroupID, "error", rmErr)
			}
		}
	}
}

// scanProvider scans an IdentityProvider from a rows result (no client_secret).
func scanProvider(rows *sql.Rows) (IdentityProvider, error) {
	var p IdentityProvider
	var tenantID, domain, defaultRoleID sql.NullString
	var enabled, autoProvision, groupMappingEnabled, autoImportGroups bool
	var mappingsJSON string

	err := rows.Scan(
		&p.ID, &p.Name, &p.ProviderType, &enabled, &p.ClientID,
		&tenantID, &domain, &p.Scopes, &autoProvision, &defaultRoleID,
		&groupMappingEnabled, &mappingsJSON, &autoImportGroups, &p.CreatedAt, &p.UpdatedAt,
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
	var tenantID, domain, defaultRoleID sql.NullString
	var enabled, autoProvision, groupMappingEnabled, autoImportGroups bool
	var mappingsJSON string

	err := row.Scan(
		&p.ID, &p.Name, &p.ProviderType, &enabled, &p.ClientID,
		&tenantID, &domain, &p.Scopes, &autoProvision, &defaultRoleID,
		&groupMappingEnabled, &mappingsJSON, &autoImportGroups, &p.CreatedAt, &p.UpdatedAt,
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
