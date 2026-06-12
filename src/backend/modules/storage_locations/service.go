// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package storage_locations

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
)

// ErrLocalStorageProtected means local backup storage cannot be disabled, deleted, or changed to a different type.
var ErrLocalStorageProtected = errors.New("local backup storage is protected")

// Service handles storage location persistence.
type Service struct {
	db  *sql.DB
	enc *encryption.Service
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
}

// NewService creates a new storage location service.
func NewService(database *sql.DB, enc *encryption.Service) *Service {
	return &Service{db: database, enc: enc}
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
	defer rows.Close()

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
	if err == sql.ErrNoRows {
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

	enabled := input.Enabled
	if !enabled {
		enabled = true
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO storage_locations (
			id, name, location_type, enabled, host, port, base_path, region, bucket,
			endpoint, tenant_id, site_url, drive_id, share_name, domain, username,
			auth_method, tls_mode, passive_mode, password, private_key, passphrase,
			ca_certificate, client_certificate, client_key,
			access_key_id, secret_access_key, client_id, client_secret,
			refresh_token, token, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, input.Name, input.LocationType, enabled, nullStr(input.Host), nullInt(input.Port),
		nullStr(input.BasePath), nullStr(input.Region), nullStr(input.Bucket), nullStr(input.Endpoint),
		nullStr(input.TenantID), nullStr(input.SiteURL), nullStr(input.DriveID), nullStr(input.ShareName),
		nullStr(input.Domain), nullStr(input.Username), nullStr(input.AuthMethod), nullStr(input.TLSMode),
		input.PassiveMode, nullStr(secrets.password), nullStr(secrets.privateKey), nullStr(secrets.passphrase),
		nullStr(secrets.caCertificate), nullStr(secrets.clientCert), nullStr(secrets.clientKey),
		nullStr(secrets.accessKeyID), nullStr(secrets.secretAccessKey), nullStr(secrets.clientID),
		nullStr(secrets.clientSecret), nullStr(secrets.refreshToken), nullStr(secrets.token), now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting storage location: %w", err)
	}

	return s.ByID(ctx, id)
}

// Update applies partial updates to a storage location.
func (s *Service) Update(ctx context.Context, id string, input UpdateStorageLocationInput) (*StorageLocation, error) {
	var existsID, existingType string
	if err := s.db.QueryRowContext(ctx, "SELECT id, location_type FROM storage_locations WHERE id = ?", id).Scan(&existsID, &existingType); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("checking storage location existence: %w", err)
	}
	if existingType == "local" {
		if input.Enabled != nil && !*input.Enabled {
			return nil, ErrLocalStorageProtected
		}
		if input.LocationType != nil && *input.LocationType != "local" {
			return nil, ErrLocalStorageProtected
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

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
	if err := s.db.QueryRowContext(ctx, "SELECT location_type FROM storage_locations WHERE id = ?", id).Scan(&locationType); err == sql.ErrNoRows {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("checking storage location type: %w", err)
	}
	if locationType == "local" {
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
	if err == sql.ErrNoRows {
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
	var tenantID, clientID, clientSecret sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, location_type, tenant_id, client_id, client_secret
		FROM storage_locations
		WHERE id = ?`, locationID,
	).Scan(&item.LocationID, &item.LocationType, &tenantID, &clientID, &clientSecret)
	if err == sql.ErrNoRows {
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
		return "UPDATE storage_locations SET password = ?, updated_at = ? WHERE id = ?"
	case "private_key":
		return "UPDATE storage_locations SET private_key = ?, updated_at = ? WHERE id = ?"
	case "passphrase":
		return "UPDATE storage_locations SET passphrase = ?, updated_at = ? WHERE id = ?"
	case "ca_certificate":
		return "UPDATE storage_locations SET ca_certificate = ?, updated_at = ? WHERE id = ?"
	case "client_certificate":
		return "UPDATE storage_locations SET client_certificate = ?, updated_at = ? WHERE id = ?"
	case "client_key":
		return "UPDATE storage_locations SET client_key = ?, updated_at = ? WHERE id = ?"
	case "access_key_id":
		return "UPDATE storage_locations SET access_key_id = ?, updated_at = ? WHERE id = ?"
	case "secret_access_key":
		return "UPDATE storage_locations SET secret_access_key = ?, updated_at = ? WHERE id = ?"
	case "client_id":
		return "UPDATE storage_locations SET client_id = ?, updated_at = ? WHERE id = ?"
	case "client_secret":
		return "UPDATE storage_locations SET client_secret = ?, updated_at = ? WHERE id = ?"
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
