// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package storage_locations

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/secrets"
	"github.com/therealmcsparrow/mcharbor/core/storageconfig"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// ErrLocalStorageProtected means the built-in local location cannot be disabled, deleted, or changed to a
// different type; every backup falls back to it, so it must always exist.
var ErrLocalStorageProtected = errors.New("local backup storage is protected")

// DefaultLocalLocationID is the built-in location migrations 050/051 seed. Only it is protected: an extra
// local location an operator adds is an ordinary location and can be disabled or deleted.
const DefaultLocalLocationID = "default-local-backup"

// Service handles storage location persistence.
type Service struct {
	db  *sql.DB
	enc *encryption.Service
	// pluginRuntime is the host's loaded plugin runtime, set by
	// SetPluginRuntime after both modules finish wiring. Held as
	// `any` to avoid an import cycle between modules/storage_locations
	// and the plugin runtime; plugin_test_dispatch.go does the
	// type assertion to plugin.StorageAdapterSource.
	pluginRuntime any
	// vault reads the nine operator-typed credentials through the secrets
	// vault once Phase 5b's backfill has adopted them, and falls back to the
	// legacy columns until then. The OAuth columns beside them —
	// access_token, refresh_token, token — are deliberately not adopted and
	// keep their own path: they rotate hourly, and versioning them would
	// write secret_versions rows continuously for values no human reads.
	vault *secrets.Service
}

type storageOAuthState struct {
	State      string
	LocationID string
	Provider   string
	ExpiresAt  string
}

type storageOAuthCredentials struct {
	LocationID   string
	LocationType string
	TenantID     string
	ClientID     string
	ClientSecret string
	// Config is the decrypted per-row plugin config map.
	Config map[string]string
}

// NewService creates a new storage location service.
func NewService(database *sql.DB, enc *encryption.Service) *Service {
	svc := &Service{db: database, enc: enc}
	if database != nil && enc != nil {
		svc.vault = secrets.NewService(database, enc, nil)
	}
	return svc
}

// readAdopted reads one adopted storage credential through the vault, falling
// back to the legacy column. Excluded columns must not call it.
func (s *Service) readAdopted(ctx context.Context, secretID sql.NullString, legacy string) (string, error) {
	if s.vault == nil {
		return s.enc.Decrypt(legacy)
	}
	return s.vault.ReadThrough(ctx, secretID, legacy)
}

// SetPluginRuntime injects the loaded plugin runtime into the service.
// The runtime is held as `any` to avoid the import cycle; the
// dispatcher does the type assertion to plugin.StorageAdapterSource.
func (s *Service) SetPluginRuntime(runtime any) {
	s.pluginRuntime = runtime
}

// List returns all configured storage locations.
func (s *Service) List(ctx context.Context) ([]StorageLocation, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, location_type, enabled, host, port, base_path, region, bucket,
		       endpoint, tenant_id, site_url, drive_id, share_name, domain, username,
		       auth_method, tls_mode, passive_mode,
		       created_at, updated_at
		FROM storage_locations
		ORDER BY name ASC
		LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing storage locations: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()

	items := []StorageLocation{}
	for rows.Next() {
		item, err := scanStorageLocation(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning storage location: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating storage locations: %w", err)
	}

	return items, nil
}

// ByID returns a single storage location.
func (s *Service) ByID(ctx context.Context, id string) (*StorageLocation, error) {
	var item StorageLocation
	var enabled sql.NullBool
	var host, basePath, region, bucket, endpoint, tenantID, siteURL sql.NullString
	var driveID, shareName, domain, username sql.NullString
	var authMethod, tlsMode sql.NullString
	var passiveMode sql.NullBool
	var port sql.NullInt64

	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, location_type, enabled, host, port, base_path, region, bucket,
		       endpoint, tenant_id, site_url, drive_id, share_name, domain, username,
		       auth_method, tls_mode, passive_mode,
		       created_at, updated_at
		FROM storage_locations
		WHERE id = ?`, id,
	).Scan(&item.ID, &item.Name, &item.LocationType, &enabled, &host, &port, &basePath,
		&region, &bucket, &endpoint, &tenantID, &siteURL, &driveID, &shareName,
		&domain, &username, &authMethod, &tlsMode, &passiveMode, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting storage location %s: %w", id, err)
	}

	item.Enabled = enabled.Bool
	item.Host = host.String
	item.Port = int(port.Int64)
	item.BasePath = basePath.String
	item.Region = region.String
	item.Bucket = bucket.String
	item.Endpoint = endpoint.String
	item.TenantID = tenantID.String
	item.SiteURL = siteURL.String
	item.DriveID = driveID.String
	item.ShareName = shareName.String
	item.Domain = domain.String
	item.Username = username.String
	item.AuthMethod = authMethod.String
	item.TLSMode = tlsMode.String
	item.PassiveMode = passiveMode.Bool

	return &item, nil
}

// Create inserts a new storage location, encrypting credential fields.
func (s *Service) Create(ctx context.Context, input CreateStorageLocationInput) (*StorageLocation, error) {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	applyConfigToCreateInput(&input)

	configValue, err := s.encodeConfig(input.Config)
	if err != nil {
		return nil, err
	}

	secrets, err := s.encryptSecrets(storageSecrets{
		password:        input.Password,
		privateKey:      input.PrivateKey,
		passphrase:      input.Passphrase,
		caCertificate:   input.CACertificate,
		clientCert:      input.ClientCert,
		clientKey:       input.ClientKey,
		accessKeyID:     input.AccessKeyID,
		secretAccessKey: input.SecretAccessKey,
		clientID:        input.ClientID,
		clientSecret:    input.ClientSecret,
		refreshToken:    input.RefreshToken,
		token:           input.Token,
	})
	if err != nil {
		return nil, err
	}

	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO storage_locations (
			id, name, location_type, enabled, host, port, base_path, region, bucket,
			endpoint, tenant_id, site_url, drive_id, share_name, domain, username,
			auth_method, tls_mode, passive_mode, password, private_key, passphrase,
			ca_certificate, client_certificate, client_key,
			access_key_id, secret_access_key, client_id, client_secret,
			refresh_token, token, config, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, input.Name, input.LocationType, enabled, nullStr(input.Host), nullInt(input.Port),
		nullStr(input.BasePath), nullStr(input.Region), nullStr(input.Bucket), nullStr(input.Endpoint),
		nullStr(input.TenantID), nullStr(input.SiteURL), nullStr(input.DriveID), nullStr(input.ShareName),
		nullStr(input.Domain), nullStr(input.Username), nullStr(input.AuthMethod), nullStr(input.TLSMode),
		input.PassiveMode, nullStr(secrets.password), nullStr(secrets.privateKey), nullStr(secrets.passphrase),
		nullStr(secrets.caCertificate), nullStr(secrets.clientCert), nullStr(secrets.clientKey),
		nullStr(secrets.accessKeyID), nullStr(secrets.secretAccessKey), nullStr(secrets.clientID),
		nullStr(secrets.clientSecret), nullStr(secrets.refreshToken), nullStr(secrets.token), configValue, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting storage location: %w", err)
	}

	return s.ByID(ctx, id)
}

// Update applies partial updates to a storage location.
func (s *Service) Update(ctx context.Context, id string, input UpdateStorageLocationInput) (*StorageLocation, error) {
	var existsID, existingType string
	if err := s.db.QueryRowContext(ctx, "SELECT id, location_type FROM storage_locations WHERE id = ?", id).Scan(&existsID, &existingType); errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("checking storage location existence: %w", err)
	}
	if existsID == DefaultLocalLocationID {
		if input.Enabled != nil && !*input.Enabled {
			return nil, ErrLocalStorageProtected
		}
		if input.LocationType != nil && *input.LocationType != "local" {
			return nil, ErrLocalStorageProtected
		}
	}

	applyConfigToUpdateInput(&input)

	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer closeutil.Rollback(tx)

	if input.Config != nil {
		configValue, err := s.encodeConfig(input.Config)
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE storage_locations SET config = ?, updated_at = ? WHERE id = ?", configValue, now, id); err != nil {
			return nil, fmt.Errorf("updating storage location config: %w", err)
		}
	}

	if err := updateText(ctx, tx, id, "name", input.Name, now); err != nil {
		return nil, err
	}
	if err := updateText(ctx, tx, id, "location_type", input.LocationType, now); err != nil {
		return nil, err
	}
	if input.Enabled != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE storage_locations SET enabled = ?, updated_at = ? WHERE id = ?", *input.Enabled, now, id); err != nil {
			return nil, fmt.Errorf("updating storage location enabled: %w", err)
		}
	}
	textFields := []struct {
		column string
		value  *string
	}{
		{"host", input.Host}, {"base_path", input.BasePath}, {"region", input.Region},
		{"bucket", input.Bucket}, {"endpoint", input.Endpoint}, {"tenant_id", input.TenantID},
		{"site_url", input.SiteURL}, {"drive_id", input.DriveID}, {"share_name", input.ShareName},
		{"domain", input.Domain}, {"username", input.Username}, {"auth_method", input.AuthMethod},
		{"tls_mode", input.TLSMode},
	}
	for _, field := range textFields {
		if err := updateText(ctx, tx, id, field.column, field.value, now); err != nil {
			return nil, err
		}
	}
	if input.Port != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE storage_locations SET port = ?, updated_at = ? WHERE id = ?", nullInt(*input.Port), now, id); err != nil {
			return nil, fmt.Errorf("updating storage location port: %w", err)
		}
	}
	if input.PassiveMode != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE storage_locations SET passive_mode = ?, updated_at = ? WHERE id = ?", *input.PassiveMode, now, id); err != nil {
			return nil, fmt.Errorf("updating storage location passive mode: %w", err)
		}
	}

	secretFields := []struct {
		column string
		value  *string
		label  string
	}{
		{"password", input.Password, "password"},
		{"private_key", input.PrivateKey, "private key"},
		{"passphrase", input.Passphrase, "passphrase"},
		{"ca_certificate", input.CACertificate, "ca certificate"},
		{"client_certificate", input.ClientCert, "client certificate"},
		{"client_key", input.ClientKey, "client key"},
		{"access_key_id", input.AccessKeyID, "access key id"},
		{"secret_access_key", input.SecretAccessKey, "secret access key"},
		{"client_id", input.ClientID, "client id"},
		{"client_secret", input.ClientSecret, "client secret"},
		{"refresh_token", input.RefreshToken, "refresh token"},
		{"token", input.Token, "token"},
	}
	for _, field := range secretFields {
		if field.value == nil || *field.value == "" {
			continue
		}
		encrypted, err := s.enc.Encrypt(*field.value)
		if err != nil {
			return nil, fmt.Errorf("encrypting %s: %w", field.label, err)
		}
		if err := updateSecret(ctx, tx, id, field.column, encrypted, now); err != nil {
			return nil, fmt.Errorf("updating storage location %s: %w", field.label, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	return s.ByID(ctx, id)
}

// Delete removes a storage location.
func (s *Service) Delete(ctx context.Context, id string) (bool, error) {
	var locationType string
	if err := s.db.QueryRowContext(ctx, "SELECT location_type FROM storage_locations WHERE id = ?", id).Scan(&locationType); errors.Is(err, sql.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("checking storage location type: %w", err)
	}
	if id == DefaultLocalLocationID {
		return false, ErrLocalStorageProtected
	}

	result, err := s.db.ExecContext(ctx, "DELETE FROM storage_locations WHERE id = ?", id)
	if err != nil {
		return false, fmt.Errorf("deleting storage location %s: %w", id, err)
	}

	return db.RowsAffected(result) > 0, nil
}

// CreateOAuthState creates a short-lived state value for delegated provider consent.
func (s *Service) CreateOAuthState(ctx context.Context, locationID, provider string) (*storageOAuthState, error) {
	state, err := randomState()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	item := &storageOAuthState{
		State:      state,
		LocationID: locationID,
		Provider:   provider,
		ExpiresAt:  now.Add(10 * time.Minute).Format(time.RFC3339),
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO storage_location_oauth_states (state, storage_location_id, provider, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		item.State, item.LocationID, item.Provider, item.ExpiresAt, now.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("creating storage oauth state: %w", err)
	}

	return item, nil
}

// OAuthState returns a pending consent state if it exists and has not expired.
func (s *Service) OAuthState(ctx context.Context, state string) (*storageOAuthState, error) {
	var item storageOAuthState
	err := s.db.QueryRowContext(ctx, `
		SELECT state, storage_location_id, provider, expires_at
		FROM storage_location_oauth_states
		WHERE state = ?`, state,
	).Scan(&item.State, &item.LocationID, &item.Provider, &item.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting storage oauth state: %w", err)
	}
	expiresAt, err := time.Parse(time.RFC3339, item.ExpiresAt)
	if err != nil || time.Now().UTC().After(expiresAt) {
		if deleteErr := s.DeleteOAuthState(ctx, state); deleteErr != nil {
			return nil, deleteErr
		}
		return nil, nil
	}
	return &item, nil
}

// DeleteOAuthState removes a consent state after use or expiry.
func (s *Service) DeleteOAuthState(ctx context.Context, state string) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM storage_location_oauth_states WHERE state = ?", state); err != nil {
		return fmt.Errorf("deleting storage oauth state: %w", err)
	}
	return nil
}

// OAuthCredentials decrypts the client credentials needed to start consent.
func (s *Service) OAuthCredentials(ctx context.Context, locationID string) (*storageOAuthCredentials, error) {
	var item storageOAuthCredentials
	var tenantID, clientID, clientSecret, configJSON sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, location_type, tenant_id, client_id, client_secret, config
		FROM storage_locations
		WHERE id = ?`, locationID,
	).Scan(&item.LocationID, &item.LocationType, &tenantID, &clientID, &clientSecret, &configJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting storage oauth credentials: %w", err)
	}
	if tenantID.Valid {
		item.TenantID = tenantID.String
	}
	if clientID.Valid {
		decrypted, err := s.enc.Decrypt(clientID.String)
		if err != nil {
			return nil, fmt.Errorf("decrypting client id: %w", err)
		}
		item.ClientID = decrypted
	}
	if clientSecret.Valid {
		decrypted, err := s.enc.Decrypt(clientSecret.String)
		if err != nil {
			return nil, fmt.Errorf("decrypting client secret: %w", err)
		}
		item.ClientSecret = decrypted
	}
	cfg, err := DecodeConfig(s.enc, configJSON.String)
	if err != nil {
		return nil, err
	}
	item.Config = cfg
	return &item, nil
}

// StoreOAuthToken stores tokens returned by provider consent.
func (s *Service) StoreOAuthToken(ctx context.Context, locationID, accessToken, refreshToken string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if accessToken != "" {
		encrypted, err := s.enc.Encrypt(accessToken)
		if err != nil {
			return fmt.Errorf("encrypting access token: %w", err)
		}
		if _, err := s.db.ExecContext(ctx, "UPDATE storage_locations SET token = ?, updated_at = ? WHERE id = ?", encrypted, now, locationID); err != nil {
			return fmt.Errorf("updating storage access token: %w", err)
		}
	}
	if refreshToken != "" {
		encrypted, err := s.enc.Encrypt(refreshToken)
		if err != nil {
			return fmt.Errorf("encrypting refresh token: %w", err)
		}
		if _, err := s.db.ExecContext(ctx, "UPDATE storage_locations SET refresh_token = ?, updated_at = ? WHERE id = ?", encrypted, now, locationID); err != nil {
			return fmt.Errorf("updating storage refresh token: %w", err)
		}
	}
	return nil
}

// DecryptedSecrets returns the decrypted credential columns for a
// single storage location. The endpoint is permission-gated by
// the route handler (settings.manage) so only operators who can
// edit the destination can see the secrets. The plugin's ConfigForm
// relies on this to populate fields on edit — the host's GET
// response deliberately omits credentials by design, so without
// this dedicated endpoint the operator would have to re-enter
// every credential every time they edited a working destination.
//
// The transport is HTTPS. The secrets stay encrypted at rest in
// the database; the AES-256-GCM layer decrypts at the moment of
// the API response.
func (s *Service) DecryptedSecrets(ctx context.Context, locationID string) (*StorageLocationSecrets, error) {
	if s.enc == nil {
		return nil, fmt.Errorf("storage credential encryption service is not configured")
	}

	var clientID, clientSecret, refreshToken, token sql.NullString
	var username, password, privateKey, passphrase sql.NullString
	var accessKeyID, secretAccessKey sql.NullString
	var configJSON sql.NullString
	// The adopted columns' vault pointers. `username`, `client_id`,
	// `refresh_token` and `token` have none: the first two are not
	// credentials and the last two are machine-rotated (§5.11).
	var clientSecretID, passwordID, privateKeyID, passphraseID sql.NullString
	var accessKeyIDID, secretAccessKeyID sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT client_id, client_secret, refresh_token, token,
		       username, password, private_key, passphrase,
		       access_key_id, secret_access_key, config,
		       client_secret_secret_id, password_secret_id, private_key_secret_id,
		       passphrase_secret_id, access_key_id_secret_id, secret_access_key_secret_id
		FROM storage_locations
		WHERE id = ?`, locationID,
	).Scan(
		&clientID, &clientSecret, &refreshToken, &token,
		&username, &password, &privateKey, &passphrase,
		&accessKeyID, &secretAccessKey, &configJSON,
		&clientSecretID, &passwordID, &privateKeyID,
		&passphraseID, &accessKeyIDID, &secretAccessKeyID,
	)
	if err != nil {
		return nil, fmt.Errorf("loading storage location secrets: %w", err)
	}

	out := &StorageLocationSecrets{}

	if clientID.Valid && strings.TrimSpace(clientID.String) != "" {
		d, err := s.enc.Decrypt(clientID.String)
		if err != nil {
			return nil, fmt.Errorf("decrypting client id: %w", err)
		}
		out.ClientID = d
	}
	if (clientSecret.Valid && strings.TrimSpace(clientSecret.String) != "") || clientSecretID.String != "" {
		d, err := s.readAdopted(ctx, clientSecretID, clientSecret.String)
		if err != nil {
			return nil, fmt.Errorf("decrypting client secret: %w", err)
		}
		out.ClientSecret = d
	}
	if refreshToken.Valid && strings.TrimSpace(refreshToken.String) != "" {
		d, err := s.enc.Decrypt(refreshToken.String)
		if err != nil {
			return nil, fmt.Errorf("decrypting refresh token: %w", err)
		}
		out.RefreshToken = d
	}
	if token.Valid && strings.TrimSpace(token.String) != "" {
		d, err := s.enc.Decrypt(token.String)
		if err != nil {
			return nil, fmt.Errorf("decrypting access token: %w", err)
		}
		out.Token = d
	}
	if username.Valid && strings.TrimSpace(username.String) != "" {
		d, err := s.enc.Decrypt(username.String)
		if err != nil {
			return nil, fmt.Errorf("decrypting username: %w", err)
		}
		out.Username = d
	}
	if (password.Valid && strings.TrimSpace(password.String) != "") || passwordID.String != "" {
		d, err := s.readAdopted(ctx, passwordID, password.String)
		if err != nil {
			return nil, fmt.Errorf("decrypting password: %w", err)
		}
		out.Password = d
	}
	if (privateKey.Valid && strings.TrimSpace(privateKey.String) != "") || privateKeyID.String != "" {
		d, err := s.readAdopted(ctx, privateKeyID, privateKey.String)
		if err != nil {
			return nil, fmt.Errorf("decrypting private key: %w", err)
		}
		out.PrivateKey = d
	}
	if (passphrase.Valid && strings.TrimSpace(passphrase.String) != "") || passphraseID.String != "" {
		d, err := s.readAdopted(ctx, passphraseID, passphrase.String)
		if err != nil {
			return nil, fmt.Errorf("decrypting passphrase: %w", err)
		}
		out.Passphrase = d
	}
	if (accessKeyID.Valid && strings.TrimSpace(accessKeyID.String) != "") || accessKeyIDID.String != "" {
		d, err := s.readAdopted(ctx, accessKeyIDID, accessKeyID.String)
		if err != nil {
			return nil, fmt.Errorf("decrypting access key id: %w", err)
		}
		out.AccessKeyID = d
	}
	if (secretAccessKey.Valid && strings.TrimSpace(secretAccessKey.String) != "") || secretAccessKeyID.String != "" {
		d, err := s.readAdopted(ctx, secretAccessKeyID, secretAccessKey.String)
		if err != nil {
			return nil, fmt.Errorf("decrypting secret access key: %w", err)
		}
		out.SecretAccessKey = d
	}

	// The per-row Config JSON map carries plugin-specific fields
	// (tenant_id, drive_id, upn, site_id, ...). Decode and inline so
	// the dialog can feed the plugin's ConfigForm without a second pass.
	extra, err := DecodeConfig(s.enc, configJSON.String)
	if err != nil {
		return nil, err
	}
	if len(extra) > 0 {
		out.Extra = extra
	}

	return out, nil
}

type storageSecrets struct {
	password        string
	privateKey      string
	passphrase      string
	caCertificate   string
	clientCert      string
	clientKey       string
	accessKeyID     string
	secretAccessKey string
	clientID        string
	clientSecret    string
	refreshToken    string
	token           string
}

func (s *Service) encryptSecrets(input storageSecrets) (storageSecrets, error) {
	values := []*string{
		&input.password, &input.privateKey, &input.passphrase, &input.caCertificate,
		&input.clientCert, &input.clientKey, &input.accessKeyID, &input.secretAccessKey, &input.clientID,
		&input.clientSecret, &input.refreshToken, &input.token,
	}
	labels := []string{
		"password", "private key", "passphrase", "ca certificate", "client certificate",
		"client key", "access key id", "secret access key", "client id", "client secret",
		"refresh token", "token",
	}
	for i, value := range values {
		if *value == "" {
			continue
		}
		encrypted, err := s.enc.Encrypt(*value)
		if err != nil {
			return storageSecrets{}, fmt.Errorf("encrypting %s: %w", labels[i], err)
		}
		*value = encrypted
	}
	return input, nil
}

func scanStorageLocation(rows *sql.Rows) (StorageLocation, error) {
	var item StorageLocation
	var enabled sql.NullBool
	var host, basePath, region, bucket, endpoint, tenantID, siteURL sql.NullString
	var driveID, shareName, domain, username sql.NullString
	var authMethod, tlsMode sql.NullString
	var port sql.NullInt64
	var passiveMode sql.NullBool

	if err := rows.Scan(&item.ID, &item.Name, &item.LocationType, &enabled, &host, &port,
		&basePath, &region, &bucket, &endpoint, &tenantID, &siteURL, &driveID,
		&shareName, &domain, &username, &authMethod, &tlsMode, &passiveMode,
		&item.CreatedAt, &item.UpdatedAt); err != nil {
		return StorageLocation{}, err
	}

	item.Enabled = enabled.Bool
	item.Host = host.String
	item.Port = int(port.Int64)
	item.BasePath = basePath.String
	item.Region = region.String
	item.Bucket = bucket.String
	item.Endpoint = endpoint.String
	item.TenantID = tenantID.String
	item.SiteURL = siteURL.String
	item.DriveID = driveID.String
	item.ShareName = shareName.String
	item.Domain = domain.String
	item.Username = username.String
	item.AuthMethod = authMethod.String
	item.TLSMode = tlsMode.String
	item.PassiveMode = passiveMode.Bool
	return item, nil
}

func updateText(ctx context.Context, tx *sql.Tx, id, column string, value *string, now string) error {
	if value == nil {
		return nil
	}
	stmt := storageLocationTextUpdateStatement(column)
	if stmt == "" {
		return fmt.Errorf("unsupported storage location text column %s", column)
	}
	if _, err := tx.ExecContext(ctx, stmt, nullStr(*value), now, id); err != nil {
		return fmt.Errorf("updating storage location %s: %w", column, err)
	}
	return nil
}

func updateSecret(ctx context.Context, tx *sql.Tx, id, column, encrypted, now string) error {
	stmt := storageLocationSecretUpdateStatement(column)
	if stmt == "" {
		return fmt.Errorf("unsupported storage location secret column %s", column)
	}
	_, err := tx.ExecContext(ctx, stmt, encrypted, now, id)
	return err
}

func storageLocationTextUpdateStatement(column string) string {
	switch column {
	case "name":
		return "UPDATE storage_locations SET name = ?, updated_at = ? WHERE id = ?"
	case "location_type":
		return "UPDATE storage_locations SET location_type = ?, updated_at = ? WHERE id = ?"
	case "host":
		return "UPDATE storage_locations SET host = ?, updated_at = ? WHERE id = ?"
	case "base_path":
		return "UPDATE storage_locations SET base_path = ?, updated_at = ? WHERE id = ?"
	case "region":
		return "UPDATE storage_locations SET region = ?, updated_at = ? WHERE id = ?"
	case "bucket":
		return "UPDATE storage_locations SET bucket = ?, updated_at = ? WHERE id = ?"
	case "endpoint":
		return "UPDATE storage_locations SET endpoint = ?, updated_at = ? WHERE id = ?"
	case "tenant_id":
		return "UPDATE storage_locations SET tenant_id = ?, updated_at = ? WHERE id = ?"
	case "site_url":
		return "UPDATE storage_locations SET site_url = ?, updated_at = ? WHERE id = ?"
	case "drive_id":
		return "UPDATE storage_locations SET drive_id = ?, updated_at = ? WHERE id = ?"
	case "share_name":
		return "UPDATE storage_locations SET share_name = ?, updated_at = ? WHERE id = ?"
	case "domain":
		return "UPDATE storage_locations SET domain = ?, updated_at = ? WHERE id = ?"
	case "username":
		return "UPDATE storage_locations SET username = ?, updated_at = ? WHERE id = ?"
	case "auth_method":
		return "UPDATE storage_locations SET auth_method = ?, updated_at = ? WHERE id = ?"
	case "tls_mode":
		return "UPDATE storage_locations SET tls_mode = ?, updated_at = ? WHERE id = ?"
	default:
		return ""
	}
}

func storageLocationSecretUpdateStatement(column string) string {
	switch column {
	case "password":
		// The pointer is cleared alongside the value so an adopted copy can
		// never shadow a freshly typed credential; the next boot re-adopts it.
		return "UPDATE storage_locations SET password = ?, password_secret_id = NULL, updated_at = ? WHERE id = ?"
	case "private_key":
		// The pointer is cleared alongside the value so an adopted copy can
		// never shadow a freshly typed credential; the next boot re-adopts it.
		return "UPDATE storage_locations SET private_key = ?, private_key_secret_id = NULL, updated_at = ? WHERE id = ?"
	case "passphrase":
		// The pointer is cleared alongside the value so an adopted copy can
		// never shadow a freshly typed credential; the next boot re-adopts it.
		return "UPDATE storage_locations SET passphrase = ?, passphrase_secret_id = NULL, updated_at = ? WHERE id = ?"
	case "ca_certificate":
		// The pointer is cleared alongside the value so an adopted copy can
		// never shadow a freshly typed credential; the next boot re-adopts it.
		return "UPDATE storage_locations SET ca_certificate = ?, ca_certificate_secret_id = NULL, updated_at = ? WHERE id = ?"
	case "client_certificate":
		// The pointer is cleared alongside the value so an adopted copy can
		// never shadow a freshly typed credential; the next boot re-adopts it.
		return "UPDATE storage_locations SET client_certificate = ?, client_certificate_secret_id = NULL, updated_at = ? WHERE id = ?"
	case "client_key":
		// The pointer is cleared alongside the value so an adopted copy can
		// never shadow a freshly typed credential; the next boot re-adopts it.
		return "UPDATE storage_locations SET client_key = ?, client_key_secret_id = NULL, updated_at = ? WHERE id = ?"
	case "access_key_id":
		// The pointer is cleared alongside the value so an adopted copy can
		// never shadow a freshly typed credential; the next boot re-adopts it.
		return "UPDATE storage_locations SET access_key_id = ?, access_key_id_secret_id = NULL, updated_at = ? WHERE id = ?"
	case "secret_access_key":
		// The pointer is cleared alongside the value so an adopted copy can
		// never shadow a freshly typed credential; the next boot re-adopts it.
		return "UPDATE storage_locations SET secret_access_key = ?, secret_access_key_secret_id = NULL, updated_at = ? WHERE id = ?"
	case "client_id":
		return "UPDATE storage_locations SET client_id = ?, updated_at = ? WHERE id = ?"
	case "client_secret":
		// The pointer is cleared alongside the value so an adopted copy can
		// never shadow a freshly typed credential; the next boot re-adopts it.
		return "UPDATE storage_locations SET client_secret = ?, client_secret_secret_id = NULL, updated_at = ? WHERE id = ?"
	case "refresh_token":
		return "UPDATE storage_locations SET refresh_token = ?, updated_at = ? WHERE id = ?"
	case "token":
		return "UPDATE storage_locations SET token = ?, updated_at = ? WHERE id = ?"
	default:
		return ""
	}
}

func nullStr(value string) sql.NullString {
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func nullInt(value int) sql.NullInt64 {
	if value == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(value), Valid: true}
}

func randomState() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating storage oauth state: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// encodeConfig serialises the plugin config map and encrypts it for
// the storage_locations.config column. Plugin forms routinely put
// client secrets and tokens in this map, so it is never stored as
// plaintext. An empty map is stored as the literal "{}" the column
// defaults to.
func (s *Service) encodeConfig(cfg map[string]string) (string, error) {
	clean := make(map[string]string, len(cfg))
	for k, v := range cfg {
		if k = strings.TrimSpace(k); k == "" || strings.TrimSpace(v) == "" {
			continue
		}
		clean[k] = v
	}
	if len(clean) == 0 {
		return "{}", nil
	}
	raw, err := json.Marshal(clean)
	if err != nil {
		return "", fmt.Errorf("encoding storage location config: %w", err)
	}
	if s.enc == nil {
		return "", fmt.Errorf("storage credential encryption service is not configured")
	}
	encrypted, err := s.enc.Encrypt(string(raw))
	if err != nil {
		return "", fmt.Errorf("encrypting storage location config: %w", err)
	}
	return encrypted, nil
}

// DecodeConfig turns the storage_locations.config column back into a map.
// The decoding itself lives in core/encryption so other modules can read
// such a column without importing this one.
func DecodeConfig(enc *encryption.Service, raw string) (map[string]string, error) {
	return storageconfig.Decode(enc, raw)
}

// configColumnKeys maps the snake_case keys plugin ConfigForms emit to
// the first-class columns the host relies on (OAuth readiness checks
// client_id / client_secret, the list view shows tenant_id, ...).
type configColumnBinding struct {
	key string
	get func(*CreateStorageLocationInput) *string
	ptr func(*UpdateStorageLocationInput) **string
}

var configColumnBindings = []configColumnBinding{
	{"host", func(i *CreateStorageLocationInput) *string { return &i.Host }, func(i *UpdateStorageLocationInput) **string { return &i.Host }},
	{"base_path", func(i *CreateStorageLocationInput) *string { return &i.BasePath }, func(i *UpdateStorageLocationInput) **string { return &i.BasePath }},
	{"region", func(i *CreateStorageLocationInput) *string { return &i.Region }, func(i *UpdateStorageLocationInput) **string { return &i.Region }},
	{"bucket", func(i *CreateStorageLocationInput) *string { return &i.Bucket }, func(i *UpdateStorageLocationInput) **string { return &i.Bucket }},
	{"endpoint", func(i *CreateStorageLocationInput) *string { return &i.Endpoint }, func(i *UpdateStorageLocationInput) **string { return &i.Endpoint }},
	{"tenant_id", func(i *CreateStorageLocationInput) *string { return &i.TenantID }, func(i *UpdateStorageLocationInput) **string { return &i.TenantID }},
	{"site_url", func(i *CreateStorageLocationInput) *string { return &i.SiteURL }, func(i *UpdateStorageLocationInput) **string { return &i.SiteURL }},
	{"drive_id", func(i *CreateStorageLocationInput) *string { return &i.DriveID }, func(i *UpdateStorageLocationInput) **string { return &i.DriveID }},
	{"share_name", func(i *CreateStorageLocationInput) *string { return &i.ShareName }, func(i *UpdateStorageLocationInput) **string { return &i.ShareName }},
	{"domain", func(i *CreateStorageLocationInput) *string { return &i.Domain }, func(i *UpdateStorageLocationInput) **string { return &i.Domain }},
	{"username", func(i *CreateStorageLocationInput) *string { return &i.Username }, func(i *UpdateStorageLocationInput) **string { return &i.Username }},
	{"password", func(i *CreateStorageLocationInput) *string { return &i.Password }, func(i *UpdateStorageLocationInput) **string { return &i.Password }},
	{"private_key", func(i *CreateStorageLocationInput) *string { return &i.PrivateKey }, func(i *UpdateStorageLocationInput) **string { return &i.PrivateKey }},
	{"passphrase", func(i *CreateStorageLocationInput) *string { return &i.Passphrase }, func(i *UpdateStorageLocationInput) **string { return &i.Passphrase }},
	{"access_key_id", func(i *CreateStorageLocationInput) *string { return &i.AccessKeyID }, func(i *UpdateStorageLocationInput) **string { return &i.AccessKeyID }},
	{"secret_access_key", func(i *CreateStorageLocationInput) *string { return &i.SecretAccessKey }, func(i *UpdateStorageLocationInput) **string { return &i.SecretAccessKey }},
	{"client_id", func(i *CreateStorageLocationInput) *string { return &i.ClientID }, func(i *UpdateStorageLocationInput) **string { return &i.ClientID }},
	{"client_secret", func(i *CreateStorageLocationInput) *string { return &i.ClientSecret }, func(i *UpdateStorageLocationInput) **string { return &i.ClientSecret }},
	{"refresh_token", func(i *CreateStorageLocationInput) *string { return &i.RefreshToken }, func(i *UpdateStorageLocationInput) **string { return &i.RefreshToken }},
	{"token", func(i *CreateStorageLocationInput) *string { return &i.Token }, func(i *UpdateStorageLocationInput) **string { return &i.Token }},
}

// applyConfigToCreateInput mirrors well-known snake_case keys from a
// plugin's config map into the first-class columns. Explicit
// top-level values win over the config map.
func applyConfigToCreateInput(in *CreateStorageLocationInput) {
	if len(in.Config) == 0 {
		return
	}
	for _, b := range configColumnBindings {
		dst := b.get(in)
		if *dst == "" {
			if v := strings.TrimSpace(in.Config[b.key]); v != "" {
				*dst = v
			}
		}
	}
	if in.Port == 0 {
		if p, err := strconv.Atoi(strings.TrimSpace(in.Config["port"])); err == nil && p > 0 {
			in.Port = p
		}
	}
}

// applyConfigToUpdateInput is the Update twin of applyConfigToCreateInput.
func applyConfigToUpdateInput(in *UpdateStorageLocationInput) {
	if len(in.Config) == 0 {
		return
	}
	for _, b := range configColumnBindings {
		dst := b.ptr(in)
		if *dst == nil {
			if v := strings.TrimSpace(in.Config[b.key]); v != "" {
				value := v
				*dst = &value
			}
		}
	}
	if in.Port == nil {
		if p, err := strconv.Atoi(strings.TrimSpace(in.Config["port"])); err == nil && p > 0 {
			in.Port = &p
		}
	}
}
