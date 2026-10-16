// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_providers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/secrets"
)

// Service handles AI provider connection business logic. It owns the
// CRUD surface for named provider instances (mirrors storage_locations
// and communication_channels) and the plugin-first dispatch bridge
// that turns a connection row into a live Test call against the
// registered ai-provider-* plugin.
type Service struct {
	db  *sql.DB
	enc *encryption.Service

	// pluginRuntime is the host's plugin runtime (or nil when no
	// plugins are loaded).
	pluginRuntime any
	// vault reads this module's adopted credential through the secrets vault
	// once Phase 5b's start-up backfill has linked it, and falls back to the
	// legacy column until then. Built here rather than injected: every
	// construction site already supplies the database and the encryption
	// service, and a service built without it would quietly read the legacy
	// column forever.
	vault *secrets.Service
}

// ErrConnectionNotFound reports that no AI provider connection matches the requested ID.
var ErrConnectionNotFound = errors.New("ai provider connection not found")

// ErrPluginNotLoaded reports that no plugin is registered for the connection's provider type.
var ErrPluginNotLoaded = errors.New("ai provider plugin not loaded")

// ErrInvalidConnection reports that a connection failed validation before being stored.
var ErrInvalidConnection = errors.New("invalid AI provider connection")

var providerTypePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

// aiAdapterSource is the slice of the plugin runtime the dispatcher
// needs. Spec is `any` — the picker UI fetches it via
// /plugins/loader/ai-adapters, the dispatcher only uses the client.
type aiAdapterSource interface {
	AIAdapter(providerType string) (plugin.AIAdapterClient, plugin.AIAdapterSpec, bool)
}

// Alias — not a mirror — so the type assertion cannot drift out of sync
// with the SDK.
type pluginAITestClient = plugin.AIAdapterClient

// NewService creates a new AI provider connection service.
// pluginRuntime may be nil when no plugins are loaded; the Test
// dispatcher then returns ErrPluginNotLoaded so the admin gets a clear
// message instead of a generic 500.
func NewService(database *sql.DB, enc *encryption.Service, pluginRuntime any) *Service {
	svc := &Service{db: database, enc: enc, pluginRuntime: pluginRuntime}
	if database != nil && enc != nil {
		svc.vault = secrets.NewService(database, enc, nil)
	}
	return svc
}

// readAPIKey reads a connection's API key through the vault when Phase 5b has
// adopted it, falling back to the legacy api_key column until then.
func (s *Service) readAPIKey(ctx context.Context, secretID sql.NullString, legacy string) (string, error) {
	if s.vault == nil {
		return s.enc.Decrypt(legacy)
	}
	return s.vault.ReadThrough(ctx, secretID, legacy)
}

// List returns all AI provider connections (secrets excluded).
func (s *Service) List(ctx context.Context) ([]AIProviderConnection, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+connectionColumns+`
		 FROM ai_provider_connections ORDER BY name ASC LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing AI provider connections: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()

	var items []AIProviderConnection
	for rows.Next() {
		conn, err := scanConnection(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning AI provider connection row: %w", err)
		}
		items = append(items, conn)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating AI provider connection rows: %w", err)
	}

	if items == nil {
		items = []AIProviderConnection{}
	}
	return items, nil
}

// ByID returns a single AI provider connection by ID, or nil if not found.
func (s *Service) ByID(ctx context.Context, id string) (*AIProviderConnection, error) {
	conn, err := scanConnection(s.db.QueryRowContext(ctx,
		`SELECT `+connectionColumns+`
		 FROM ai_provider_connections WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting AI provider connection %s: %w", id, err)
	}
	return &conn, nil
}

// Default returns the row whose is_default = 1, or nil if none is set.
// Used by the chat composer's resolver.
func (s *Service) Default(ctx context.Context) (*AIProviderConnection, error) {
	conn, err := scanConnection(s.db.QueryRowContext(ctx,
		`SELECT `+connectionColumns+`
		 FROM ai_provider_connections WHERE is_default = 1 LIMIT 1`))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting default AI provider connection: %w", err)
	}
	return &conn, nil
}

// Create inserts a new AI provider connection, encrypting the API key
// before insert. If isDefault is true, clears any existing default
// within the same transaction.
func (s *Service) Create(ctx context.Context, input CreateAIProviderInput) (*AIProviderConnection, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.ProviderType = strings.TrimSpace(input.ProviderType)
	input.BaseURL = strings.TrimSpace(input.BaseURL)
	input.DefaultModel = strings.TrimSpace(input.DefaultModel)
	if input.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidConnection)
	}
	if len(input.Name) > 120 {
		return nil, fmt.Errorf("%w: name exceeds 120 characters", ErrInvalidConnection)
	}
	if !providerTypePattern.MatchString(input.ProviderType) {
		return nil, fmt.Errorf("%w: providerType is required", ErrInvalidConnection)
	}
	if _, ok := s.lookupAIAdapter(input.ProviderType); !ok {
		return nil, ErrPluginNotLoaded
	}
	if err := validateBaseURL(input.BaseURL); err != nil {
		return nil, err
	}
	authMethod, err := normalizeAuthMethod(input.AuthMethod)
	if err != nil {
		return nil, err
	}
	oauthCfg, err := normalizeOAuthInput(authMethod, input.OAuth, s.hasCompleteOAuthDefaults(input.ProviderType))
	if err != nil {
		return nil, err
	}

	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	var encAPIKey string
	if input.APIKey != "" {
		encAPIKey, err = s.enc.Encrypt(input.APIKey)
		if err != nil {
			return nil, fmt.Errorf("encrypting API key: %w", err)
		}
	}
	var encClientSecret string
	if oauthCfg != nil && oauthCfg.ClientSecret != "" {
		encClientSecret, err = s.enc.Encrypt(oauthCfg.ClientSecret)
		if err != nil {
			return nil, fmt.Errorf("encrypting OAuth client secret: %w", err)
		}
	}

	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	if input.IsDefault && !enabled {
		return nil, fmt.Errorf("%w: default connection must be enabled", ErrInvalidConnection)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer db.RollbackTx(tx)

	if input.IsDefault {
		if _, err := tx.ExecContext(ctx,
			`UPDATE ai_provider_connections SET is_default = 0, updated_at = ? WHERE is_default = 1`,
			now); err != nil {
			return nil, fmt.Errorf("clearing existing default: %w", err)
		}
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO ai_provider_connections (id, name, provider_type, is_default, enabled,
		 base_url, default_model, api_key, auth_method,
		 oauth_client_id, oauth_client_secret, oauth_device_url, oauth_token_url, oauth_scopes,
		 created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, input.Name, input.ProviderType, input.IsDefault, enabled,
		nullStr(input.BaseURL), nullStr(input.DefaultModel),
		nullStr(encAPIKey), authMethod,
		nullStr(oauthField(oauthCfg, func(o *AIProviderOAuthInput) string { return o.ClientID })),
		nullStr(encClientSecret),
		nullStr(oauthField(oauthCfg, func(o *AIProviderOAuthInput) string { return o.DeviceURL })),
		nullStr(oauthField(oauthCfg, func(o *AIProviderOAuthInput) string { return o.TokenURL })),
		nullStr(oauthField(oauthCfg, func(o *AIProviderOAuthInput) string { return o.Scopes })),
		now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting AI provider connection: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	created := &AIProviderConnection{
		ID:           id,
		Name:         input.Name,
		ProviderType: input.ProviderType,
		IsDefault:    input.IsDefault,
		Enabled:      enabled,
		BaseURL:      input.BaseURL,
		DefaultModel: input.DefaultModel,
		HasAPIKey:    input.APIKey != "",
		AuthMethod:   authMethod,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if oauthCfg != nil {
		created.OAuth = &AIProviderOAuth{
			ClientID:        oauthCfg.ClientID,
			DeviceURL:       oauthCfg.DeviceURL,
			TokenURL:        oauthCfg.TokenURL,
			Scopes:          oauthCfg.Scopes,
			HasClientSecret: encClientSecret != "",
		}
	}
	return created, nil
}

// Update applies partial updates to an AI provider connection.
// Empty-string *APIKey means "do not touch"; non-empty triggers
// re-encryption. Sets/clears is_default atomically.
func (s *Service) Update(ctx context.Context, id string, input UpdateAIProviderInput) (*AIProviderConnection, error) {
	if input.Name != nil {
		trimmed := strings.TrimSpace(*input.Name)
		if trimmed == "" || len(trimmed) > 120 {
			return nil, fmt.Errorf("%w: name is invalid", ErrInvalidConnection)
		}
		input.Name = &trimmed
	}
	if input.BaseURL != nil {
		trimmed := strings.TrimSpace(*input.BaseURL)
		if err := validateBaseURL(trimmed); err != nil {
			return nil, err
		}
		input.BaseURL = &trimmed
	}
	if input.DefaultModel != nil {
		trimmed := strings.TrimSpace(*input.DefaultModel)
		input.DefaultModel = &trimmed
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer db.RollbackTx(tx)

	var existsID string
	var currentDefault, currentEnabled bool
	if err := tx.QueryRowContext(ctx,
		`SELECT id, is_default, enabled FROM ai_provider_connections WHERE id = ?`, id,
	).Scan(&existsID, &currentDefault, &currentEnabled); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("checking AI provider connection existence: %w", err)
	}
	willBeDefault := currentDefault
	if input.IsDefault != nil {
		willBeDefault = *input.IsDefault
	}
	willBeEnabled := currentEnabled
	if input.Enabled != nil {
		willBeEnabled = *input.Enabled
	}
	if willBeDefault && !willBeEnabled {
		return nil, fmt.Errorf("%w: default connection must be enabled", ErrInvalidConnection)
	}
	if currentDefault && input.IsDefault != nil && !*input.IsDefault {
		return nil, fmt.Errorf("%w: select another default before clearing this connection", ErrInvalidConnection)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	if input.Name != nil {
		if _, err := tx.ExecContext(ctx,
			`UPDATE ai_provider_connections SET name = ?, updated_at = ? WHERE id = ?`,
			*input.Name, now, id); err != nil {
			return nil, fmt.Errorf("updating AI provider connection name: %w", err)
		}
	}
	if input.Enabled != nil {
		result, err := tx.ExecContext(ctx,
			`UPDATE ai_provider_connections SET enabled = ?, updated_at = ?
			 WHERE id = ? AND (? = 1 OR is_default = 0)`,
			*input.Enabled, now, id, *input.Enabled)
		if err != nil {
			return nil, fmt.Errorf("updating AI provider connection enabled: %w", err)
		}
		if db.RowsAffected(result) == 0 {
			return nil, fmt.Errorf("%w: default connection must be enabled", ErrInvalidConnection)
		}
	}
	if input.BaseURL != nil {
		if _, err := tx.ExecContext(ctx,
			`UPDATE ai_provider_connections SET base_url = ?, updated_at = ? WHERE id = ?`,
			*input.BaseURL, now, id); err != nil {
			return nil, fmt.Errorf("updating AI provider connection base url: %w", err)
		}
	}
	if input.DefaultModel != nil {
		if _, err := tx.ExecContext(ctx,
			`UPDATE ai_provider_connections SET default_model = ?, updated_at = ? WHERE id = ?`,
			*input.DefaultModel, now, id); err != nil {
			return nil, fmt.Errorf("updating AI provider connection default model: %w", err)
		}
	}
	if input.APIKey != nil && *input.APIKey != "" {
		enc, err := s.enc.Encrypt(*input.APIKey)
		if err != nil {
			return nil, fmt.Errorf("encrypting API key: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE ai_provider_connections SET api_key = ?, api_key_secret_id = NULL, updated_at = ? WHERE id = ?`,
			enc, now, id); err != nil {
			return nil, fmt.Errorf("updating AI provider connection API key: %w", err)
		}
	}
	if input.AuthMethod != nil {
		authMethod, err := normalizeAuthMethod(*input.AuthMethod)
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE ai_provider_connections SET auth_method = ?, updated_at = ? WHERE id = ?`,
			authMethod, now, id); err != nil {
			return nil, fmt.Errorf("updating AI provider connection auth method: %w", err)
		}
	}
	if input.OAuth != nil {
		// The endpoint config is always stored, whichever auth method
		// is active: an operator who switches to API-key auth and back
		// should not have to retype two URLs. The credential itself is
		// still only usable while auth_method = 'oauth'.
		cfg := *input.OAuth
		if _, err := tx.ExecContext(ctx,
			`UPDATE ai_provider_connections SET
			   oauth_client_id = ?, oauth_device_url = ?, oauth_token_url = ?,
			   oauth_scopes = ?, updated_at = ?
			 WHERE id = ?`,
			nullStr(strings.TrimSpace(cfg.ClientID)), nullStr(strings.TrimSpace(cfg.DeviceURL)),
			nullStr(strings.TrimSpace(cfg.TokenURL)), nullStr(strings.TrimSpace(cfg.Scopes)),
			now, id); err != nil {
			return nil, fmt.Errorf("updating AI provider OAuth config: %w", err)
		}
		// Empty client secret means "leave the stored one alone", the
		// same convention the API key field uses.
		if strings.TrimSpace(cfg.ClientSecret) != "" {
			enc, err := s.enc.Encrypt(strings.TrimSpace(cfg.ClientSecret))
			if err != nil {
				return nil, fmt.Errorf("encrypting OAuth client secret: %w", err)
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE ai_provider_connections SET oauth_client_secret = ?, oauth_client_secret_secret_id = NULL, updated_at = ? WHERE id = ?`,
				enc, now, id); err != nil {
				return nil, fmt.Errorf("updating OAuth client secret: %w", err)
			}
		}
	}
	if input.IsDefault != nil {
		if *input.IsDefault {
			var providerType string
			if err := tx.QueryRowContext(ctx,
				`SELECT provider_type FROM ai_provider_connections WHERE id = ?`, id,
			).Scan(&providerType); err != nil {
				return nil, fmt.Errorf("reading AI provider type: %w", err)
			}
			if _, ok := s.lookupAIAdapter(providerType); !ok {
				return nil, ErrPluginNotLoaded
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE ai_provider_connections SET is_default = 0, updated_at = ? WHERE is_default = 1`,
				now); err != nil {
				return nil, fmt.Errorf("clearing existing default: %w", err)
			}
			result, err := tx.ExecContext(ctx,
				`UPDATE ai_provider_connections SET is_default = 1, updated_at = ?
				 WHERE id = ? AND enabled = 1`, now, id)
			if err != nil {
				return nil, fmt.Errorf("setting new default: %w", err)
			}
			if db.RowsAffected(result) == 0 {
				return nil, fmt.Errorf("%w: default connection must be enabled", ErrInvalidConnection)
			}
		} else {
			if _, err := tx.ExecContext(ctx,
				`UPDATE ai_provider_connections SET is_default = 0, updated_at = ? WHERE id = ?`,
				now, id); err != nil {
				return nil, fmt.Errorf("clearing default flag: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	return s.ByID(ctx, id)
}

// Delete removes an AI provider connection by ID. Returns true if a
// row was deleted.
func (s *Service) Delete(ctx context.Context, id string) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("beginning transaction: %w", err)
	}
	defer db.RollbackTx(tx)

	var wasDefault bool
	if err := tx.QueryRowContext(ctx,
		`SELECT is_default FROM ai_provider_connections WHERE id = ?`, id,
	).Scan(&wasDefault); errors.Is(err, sql.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("reading AI provider connection %s: %w", id, err)
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM ai_provider_connections WHERE id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("deleting AI provider connection %s: %w", id, err)
	}
	if wasDefault {
		now := time.Now().UTC().Format(time.RFC3339)
		if _, err := tx.ExecContext(ctx,
			`UPDATE ai_provider_connections SET is_default = 1, updated_at = ?
			 WHERE id = (
			   SELECT id FROM ai_provider_connections
			   WHERE enabled = 1 ORDER BY created_at ASC LIMIT 1
			 )`, now); err != nil {
			return false, fmt.Errorf("selecting replacement default: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("committing transaction: %w", err)
	}
	return db.RowsAffected(result) > 0, nil
}

// SetDefault sets the given connection as the default, clearing any
// existing default atomically.
func (s *Service) SetDefault(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer db.RollbackTx(tx)

	var existsID, providerType string
	var enabled bool
	if err := tx.QueryRowContext(ctx,
		`SELECT id, provider_type, enabled FROM ai_provider_connections WHERE id = ?`, id,
	).Scan(&existsID, &providerType, &enabled); err == sql.ErrNoRows {
		return ErrConnectionNotFound
	} else if err != nil {
		return fmt.Errorf("checking AI provider connection existence: %w", err)
	}
	if _, ok := s.lookupAIAdapter(providerType); !ok {
		return ErrPluginNotLoaded
	}
	if !enabled {
		return fmt.Errorf("%w: default connection must be enabled", ErrInvalidConnection)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	if _, err := tx.ExecContext(ctx,
		`UPDATE ai_provider_connections SET is_default = 0, updated_at = ? WHERE is_default = 1`,
		now); err != nil {
		return fmt.Errorf("clearing existing default: %w", err)
	}
	result, err := tx.ExecContext(ctx,
		`UPDATE ai_provider_connections SET is_default = 1, updated_at = ?
		 WHERE id = ? AND enabled = 1`, now, id)
	if err != nil {
		return fmt.Errorf("setting new default: %w", err)
	}
	if db.RowsAffected(result) == 0 {
		return fmt.Errorf("%w: default connection must be enabled", ErrInvalidConnection)
	}
	return tx.Commit()
}

// TestConnection moved to test.go so the per-step reporter lives next
// to the storage_locations and communications equivalents. Returns a
// *TestConnectionResult with load → decrypt → plugin → send steps.

// PlaintextConfig returns the decrypted config map the plugin receives
// for a connection. Both the chat dispatch (ai_assistant) and the test
// endpoint go through this helper so the secret-decryption policy
// stays in one place.
func (s *Service) PlaintextConfig(ctx context.Context, row *AIProviderConnection) (map[string]string, error) {
	if row == nil {
		return nil, fmt.Errorf("nil connection")
	}

	authMethod := row.AuthMethod
	if authMethod == "" {
		authMethod = AuthMethodAPIKey
	}

	cfg := map[string]string{
		"provider_type": row.ProviderType,
		"name":          row.Name,
		"auth_method":   authMethod,
	}
	if row.BaseURL != "" {
		cfg["base_url"] = row.BaseURL
	}
	if row.DefaultModel != "" {
		cfg["default_model"] = row.DefaultModel
	}

	// OAuth connections carry a bearer token instead of a key. It is
	// published under both "access_token" and "api_key": adapters that
	// know about OAuth can read the explicit field, and every adapter
	// written before this flow existed keeps working unchanged, because
	// the providers this grant applies to accept the OAuth token in the
	// same Authorization header as a personal key.
	if authMethod == AuthMethodOAuth {
		token, err := s.oauthAccessToken(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		cfg["access_token"] = token
		cfg["api_key"] = token
		return cfg, nil
	}

	// Fetch and decrypt the API key. We use a focused query rather
	// than reading the whole row so the caller can pass a partially
	// populated struct (e.g. the one returned by ByID, which does not
	// include the secret column) without us having to maintain two
	// scan paths.
	var encKey, keySecretID sql.NullString
	if err := s.db.QueryRowContext(ctx,
		`SELECT api_key, api_key_secret_id FROM ai_provider_connections WHERE id = ?`, row.ID,
	).Scan(&encKey, &keySecretID); err != nil {
		return nil, fmt.Errorf("reading API key for connection %s: %w", row.ID, err)
	}
	if (encKey.Valid && encKey.String != "") || keySecretID.String != "" {
		// Read through the vault once Phase 5b has adopted this key; fall back
		// to the legacy api_key column until then. The OAuth access and
		// refresh tokens beside it are deliberately NOT adopted — they rotate
		// hourly — so oauth.go keeps decrypting them directly.
		plain, err := s.readAPIKey(ctx, keySecretID, encKey.String)
		if err != nil {
			return nil, fmt.Errorf("decrypting API key: %w", err)
		}
		cfg["api_key"] = plain
	}
	return cfg, nil
}

// Resolve returns the connection row plus its decrypted plugin config
// map for the chat dispatch (ai_assistant). An empty id resolves the
// connection marked is_default=1. Returns ErrConnectionNotFound when
// nothing matches — callers translate that into the chat error toast.
func (s *Service) Resolve(ctx context.Context, id string) (*AIProviderConnection, map[string]string, error) {
	var row *AIProviderConnection
	var err error
	if id == "" {
		row, err = s.Default(ctx)
	} else {
		row, err = s.ByID(ctx, id)
	}
	if err != nil {
		return nil, nil, err
	}
	if row == nil {
		return nil, nil, ErrConnectionNotFound
	}
	if !row.Enabled {
		return nil, nil, fmt.Errorf("connection %s is disabled", row.ID)
	}
	cfg, err := s.PlaintextConfig(ctx, row)
	if err != nil {
		return nil, nil, err
	}
	return row, cfg, nil
}

// lookupAIAdapter asks the plugin runtime (if non-nil) whether an
// AIAdapterClient is registered for the given provider_type. Returns
// the client and a boolean reporting whether the lookup succeeded.
func (s *Service) lookupAIAdapter(providerType string) (pluginAITestClient, bool) {
	if s.pluginRuntime == nil {
		return nil, false
	}
	source, ok := s.pluginRuntime.(aiAdapterSource)
	if !ok {
		return nil, false
	}
	cli, _, ok := source.AIAdapter(providerType)
	if !ok || cli == nil {
		return nil, false
	}
	return cli, true
}

// aiSpecsSource is the slice of the plugin runtime that lists every
// registered AI adapter spec (loader.Runtime.AIAdapterSpecs). The
// merged-OAuth-defaults endpoint uses it to collect each plugin's
// declared OAuthDefaults; the dispatcher does not need it.
type aiSpecsSource interface {
	AIAdapterSpecs() map[string]plugin.AIAdapterSpec
}

// providerOAuthDefaults returns the OAuthDefaults a plugin declared for
// one provider type, or nil when no plugin is loaded, the type is
// unknown, or the plugin declared none. Used at sign-in time to fill an
// operator connection whose OAuth fields were left blank.
func (s *Service) providerOAuthDefaults(providerType string) *plugin.OAuthDefaults {
	if s.pluginRuntime == nil {
		return nil
	}
	source, ok := s.pluginRuntime.(aiAdapterSource)
	if !ok {
		return nil
	}
	_, spec, ok := source.AIAdapter(providerType)
	if !ok {
		return nil
	}
	return spec.OAuthDefaults
}

// hasCompleteOAuthDefaults reports whether a plugin ships a client ID
// and both endpoints (its own, or the host's published endpoints) for a
// provider type — i.e. whether "zero-config" sign-in is possible, so
// Create may accept an OAuth connection with no operator-entered config.
func (s *Service) hasCompleteOAuthDefaults(providerType string) bool {
	def := s.providerOAuthDefaults(providerType)
	if def == nil || strings.TrimSpace(def.ClientID) == "" {
		return false
	}
	deviceURL := strings.TrimSpace(def.DeviceURL)
	tokenURL := strings.TrimSpace(def.TokenURL)
	if host, ok := oauthEndpointDefaults[providerType]; ok {
		if deviceURL == "" {
			deviceURL = host.DeviceURL
		}
		if tokenURL == "" {
			tokenURL = host.TokenURL
		}
	}
	return deviceURL != "" && tokenURL != ""
}

// MergedOAuthDefaults builds the map GET /ai-providers/oauth-defaults
// returns: the host's published endpoint prefills, overlaid with every
// plugin's declared OAuthDefaults. A provider type whose merged config
// is complete (client ID + both endpoints) is flagged PluginProvided so
// the create form can offer a one-click sign-in and hide the config
// fields. No secret is ever included — only the non-secret client ID
// and endpoints, which travel in the operator's browser regardless.
func (s *Service) MergedOAuthDefaults() map[string]AIProviderOAuthDefault {
	out := make(map[string]AIProviderOAuthDefault)
	for providerType, cfg := range oauthEndpointDefaults {
		out[providerType] = AIProviderOAuthDefault{
			DeviceURL: cfg.DeviceURL,
			TokenURL:  cfg.TokenURL,
			Scopes:    cfg.Scopes,
		}
	}

	source, ok := s.pluginRuntime.(aiSpecsSource)
	if !ok {
		return out
	}
	for providerType, spec := range source.AIAdapterSpecs() {
		def := spec.OAuthDefaults
		if def == nil {
			continue
		}
		merged := out[providerType]
		if v := strings.TrimSpace(def.ClientID); v != "" {
			merged.ClientID = v
		}
		if v := strings.TrimSpace(def.DeviceURL); v != "" {
			merged.DeviceURL = v
		}
		if v := strings.TrimSpace(def.TokenURL); v != "" {
			merged.TokenURL = v
		}
		if v := strings.TrimSpace(def.Scopes); v != "" {
			merged.Scopes = v
		}
		merged.PluginProvided = merged.ClientID != "" && merged.DeviceURL != "" && merged.TokenURL != ""
		out[providerType] = merged
	}
	return out
}

// normalizeAuthMethod validates the requested credential style. An
// empty value keeps the historical default so a client that predates
// the OAuth flow -- or a script posting the old body shape -- still
// creates a working API-key connection.
func normalizeAuthMethod(raw string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "", AuthMethodAPIKey:
		return AuthMethodAPIKey, nil
	case AuthMethodOAuth:
		return AuthMethodOAuth, nil
	default:
		return "", fmt.Errorf("%w: authMethod must be %q or %q", ErrInvalidConnection, AuthMethodAPIKey, AuthMethodOAuth)
	}
}

// normalizeOAuthInput trims the device-flow config and, for an OAuth
// connection, insists on the three fields the flow cannot run without.
// The check happens at create time rather than at sign-in time so the
// operator finds out in the form, not two clicks later.
// pluginProvides is true when a plugin ships a complete OAuthDefaults
// for this provider type. The operator may then create an OAuth
// connection with no config at all — the sign-in flow overlays the
// plugin's client ID and endpoints at use time (applyOAuthDefaults),
// so the required-field checks below are skipped.
func normalizeOAuthInput(authMethod string, in *AIProviderOAuthInput, pluginProvides bool) (*AIProviderOAuthInput, error) {
	if in == nil {
		if authMethod == AuthMethodOAuth && !pluginProvides {
			return nil, fmt.Errorf("%w: OAuth configuration is required for sign-in connections", ErrInvalidConnection)
		}
		return nil, nil
	}

	cfg := AIProviderOAuthInput{
		ClientID:     strings.TrimSpace(in.ClientID),
		ClientSecret: strings.TrimSpace(in.ClientSecret),
		DeviceURL:    strings.TrimSpace(in.DeviceURL),
		TokenURL:     strings.TrimSpace(in.TokenURL),
		Scopes:       strings.TrimSpace(in.Scopes),
	}
	if authMethod == AuthMethodOAuth && !pluginProvides {
		if cfg.ClientID == "" {
			return nil, fmt.Errorf("%w: OAuth client ID is required", ErrInvalidConnection)
		}
		if err := validateOAuthEndpoint(cfg.DeviceURL); err != nil {
			return nil, fmt.Errorf("%w: device authorization URL is invalid", ErrInvalidConnection)
		}
		if err := validateOAuthEndpoint(cfg.TokenURL); err != nil {
			return nil, fmt.Errorf("%w: token URL is invalid", ErrInvalidConnection)
		}
	} else if authMethod == AuthMethodOAuth {
		// Plugin-backed: only validate endpoints the operator actually
		// typed (an override); blank fields are filled from the plugin.
		if cfg.DeviceURL != "" {
			if err := validateOAuthEndpoint(cfg.DeviceURL); err != nil {
				return nil, fmt.Errorf("%w: device authorization URL is invalid", ErrInvalidConnection)
			}
		}
		if cfg.TokenURL != "" {
			if err := validateOAuthEndpoint(cfg.TokenURL); err != nil {
				return nil, fmt.Errorf("%w: token URL is invalid", ErrInvalidConnection)
			}
		}
	}
	return &cfg, nil
}

// oauthField reads one field off an optional config without repeating
// a nil check at every INSERT placeholder.
func oauthField(cfg *AIProviderOAuthInput, pick func(*AIProviderOAuthInput) string) string {
	if cfg == nil {
		return ""
	}
	return pick(cfg)
}

func nullStr(str string) sql.NullString {
	if str == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: str, Valid: true}
}

func validateBaseURL(raw string) error {
	if raw == "" {
		return nil
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%w: base URL must be an absolute HTTP or HTTPS URL", ErrInvalidConnection)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%w: base URL must not contain credentials, a query, or a fragment", ErrInvalidConnection)
	}
	return nil
}

// connectionColumns is the projection every read path shares. Secret
// columns are collapsed to presence flags in SQL, so no ciphertext --
// API key, client secret, access token or in-flight device code --
// leaves the database on a list or get. Only PlaintextConfig and the
// OAuth flow read the real values.
const connectionColumns = `id, name, provider_type, is_default, enabled, base_url, default_model,
	        CASE WHEN COALESCE(api_key, '') <> '' THEN 1 ELSE 0 END,
	        COALESCE(auth_method, 'api_key'),
	        oauth_client_id,
	        CASE WHEN COALESCE(oauth_client_secret, '') <> '' THEN '1' ELSE '' END,
	        oauth_device_url, oauth_token_url, oauth_scopes,
	        CASE WHEN COALESCE(oauth_access_token, '') <> '' THEN '1' ELSE '' END,
	        oauth_expires_at, oauth_account_label,
	        CASE WHEN COALESCE(oauth_pending_device_code, '') <> '' THEN '1' ELSE '' END,
	        oauth_pending_user_code, oauth_pending_verification_uri,
	        oauth_pending_expires_at, oauth_pending_interval,
	        created_at, updated_at`

// rowScanner is satisfied by both *sql.Row and *sql.Rows, so the list
// and the single-row reads share one scan path instead of drifting
// apart every time a column is added.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanConnection(sc rowScanner) (AIProviderConnection, error) {
	var conn AIProviderConnection
	var baseURL, defaultModel sql.NullString
	var isDefault, enabled sql.NullBool
	var clientID, deviceURL, tokenURL, scopes sql.NullString
	var expiresAt, accountLabel sql.NullString
	// oa collects the OAuth half. clientSecret / accessToken /
	// pendingDeviceCode receive the SQL presence flags above, not the
	// ciphertext -- view() only tests them for emptiness.
	var oa oauthRow

	if err := sc.Scan(&conn.ID, &conn.Name, &conn.ProviderType, &isDefault, &enabled,
		&baseURL, &defaultModel, &conn.HasAPIKey, &conn.AuthMethod,
		&clientID, &oa.clientSecret, &deviceURL, &tokenURL, &scopes,
		&oa.accessToken, &expiresAt, &accountLabel,
		&oa.pendingDeviceCode, &oa.pendingUserCode, &oa.pendingVerificationURI,
		&oa.pendingExpiresAt, &oa.pendingInterval,
		&conn.CreatedAt, &conn.UpdatedAt); err != nil {
		return AIProviderConnection{}, err
	}

	conn.IsDefault = isDefault.Bool
	conn.Enabled = enabled.Bool
	conn.BaseURL = baseURL.String
	conn.DefaultModel = defaultModel.String
	if conn.AuthMethod == "" {
		conn.AuthMethod = AuthMethodAPIKey
	}

	oa.clientID = clientID.String
	oa.deviceURL = deviceURL.String
	oa.tokenURL = tokenURL.String
	oa.scopes = scopes.String
	oa.expiresAt = expiresAt.String
	oa.accountLabel = accountLabel.String

	// Surface the OAuth block whenever there is something to show --
	// an operator who switches a configured connection back to API-key
	// auth should still see (and be able to reuse) their endpoints.
	if conn.AuthMethod == AuthMethodOAuth || oa.clientID != "" || oa.accessToken != "" {
		conn.OAuth = oa.view()
	}
	return conn, nil
}
