// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scim

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// Service owns CRUD for SCIM endpoints. Tokens are encrypted with
// the shared encryption.Service and the StableHash is used for O(1)
// lookup during inbound bearer-token auth.
type Service struct {
	db  *sql.DB
	enc *encryption.Service
}

// NewService builds a SCIM service.
func NewService(db *sql.DB, enc *encryption.Service) *Service {
	return &Service{db: db, enc: enc}
}

// ErrEndpointNotFound is returned by lookups when no row matches.
var ErrEndpointNotFound = errors.New("scim endpoint not found")

// NewBearerToken returns a freshly generated 32-byte bearer token
// formatted as `scim_<64-hex-chars>`. The plaintext is intended to
// be shown to the admin exactly once; storage uses the returned
// hash + ciphertext via Service.hashAndEncrypt.
func NewBearerToken(enc *encryption.Service) (plaintext, hash, encrypted string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", "", fmt.Errorf("generating bearer token: %w", err)
	}
	plaintext = "scim_" + hex.EncodeToString(b)
	if enc == nil {
		return plaintext, "", "", nil
	}
	hash = enc.StableHash(plaintext)
	encrypted, err = enc.Encrypt(plaintext)
	if err != nil {
		return "", "", "", fmt.Errorf("encrypting bearer token: %w", err)
	}
	return plaintext, hash, encrypted, nil
}

// CreateEndpoint inserts a new SCIM endpoint. If input.BearerToken is
// empty, a fresh token is generated. The plaintext is returned so the
// caller can include it in the response once.
func (s *Service) CreateEndpoint(ctx context.Context, in EndpointInput, createdBy string) (*Endpoint, string, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, "", errors.New("endpoint name is required")
	}
	if strings.TrimSpace(in.BaseURL) == "" {
		return nil, "", errors.New("endpoint baseUrl is required")
	}
	if in.AuthType == "" {
		in.AuthType = "bearer"
	}
	if in.PollSchedule == "" {
		in.PollSchedule = "*/15 * * * *"
	}

	plaintext := strings.TrimSpace(in.BearerToken)
	returnPlaintext := ""
	var hash, enc string
	if plaintext == "" && in.AuthType == "bearer" {
		generated, generatedHash, generatedEnc, err := NewBearerToken(s.enc)
		if err != nil {
			return nil, "", err
		}
		hash = generatedHash
		enc = generatedEnc
		returnPlaintext = generated
	} else {
		var err error
		hash, enc, err = s.hashAndEncrypt(plaintext)
		if err != nil {
			return nil, "", err
		}
	}

	id, err := newID()
	if err != nil {
		return nil, "", err
	}
	enabledInt := 0
	if in.Enabled {
		enabledInt = 1
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO scim_endpoints
		    (id, name, base_url, auth_type, encrypted_token, token_hash,
		     username, poll_schedule, enabled, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, in.Name, in.BaseURL, in.AuthType, enc, hash,
		in.Username, in.PollSchedule, enabledInt, createdBy,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, "", errors.New("endpoint name already taken")
		}
		return nil, "", fmt.Errorf("inserting scim endpoint: %w", err)
	}

	out, err := s.EndpointByID(ctx, id)
	if err != nil {
		return nil, "", err
	}
	return out, returnPlaintext, nil
}

// UpdateEndpoint modifies an existing endpoint. Tokens are only
// rewritten when input.BearerToken is non-empty; a blank field means
// "leave the existing token untouched".
func (s *Service) UpdateEndpoint(ctx context.Context, id string, in EndpointInput) (*Endpoint, error) {
	current, err := s.getRaw(ctx, id)
	if err != nil {
		return nil, err
	}

	enabledInt := 0
	if in.Enabled {
		enabledInt = 1
	}

	if in.BearerToken != "" {
		hash, enc, err := s.hashAndEncrypt(in.BearerToken)
		if err != nil {
			return nil, err
		}
		current.TokenHash = hash
		current.EncryptedToken = enc
	}

	_, err = s.db.ExecContext(ctx, `
		UPDATE scim_endpoints
		   SET base_url = ?, auth_type = ?, encrypted_token = ?, token_hash = ?,
		       username = ?, poll_schedule = ?, enabled = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		in.BaseURL, in.AuthType, current.EncryptedToken, current.TokenHash,
		in.Username, in.PollSchedule, enabledInt, id,
	)
	if err != nil {
		return nil, fmt.Errorf("updating scim endpoint: %w", err)
	}

	return s.EndpointByID(ctx, id)
}

// DeleteEndpoint removes an endpoint and its mappings/sync log (CASCADE).
func (s *Service) DeleteEndpoint(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM scim_endpoints WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting scim endpoint: %w", err)
	}
	if db.RowsAffected(res) == 0 {
		return ErrEndpointNotFound
	}
	return nil
}

// ListEndpoints returns every configured endpoint.
func (s *Service) ListEndpoints(ctx context.Context) ([]Endpoint, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, base_url, auth_type, username, poll_schedule,
		       enabled, COALESCE(last_sync_at, ''), last_sync_status,
		       last_sync_error, users_count, groups_count, created_at, updated_at,
		       CASE WHEN token_hash != '' THEN 1 ELSE 0 END, COALESCE(token_rotated_at, '')
		  FROM scim_endpoints
		 ORDER BY name ASC
		 LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing scim endpoints: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	var out []Endpoint
	for rows.Next() {
		var e Endpoint
		var enabled, hasToken int
		if err := rows.Scan(&e.ID, &e.Name, &e.BaseURL, &e.AuthType, &e.Username, &e.PollSchedule,
			&enabled, &e.LastSyncAt, &e.LastSyncStatus, &e.LastSyncError,
			&e.UsersCount, &e.GroupsCount, &e.CreatedAt, &e.UpdatedAt, &hasToken, &e.TokenRotatedAt); err != nil {
			return nil, err
		}
		e.Enabled = enabled == 1
		e.HasToken = hasToken == 1
		out = append(out, e)
	}
	return out, rows.Err()
}

// EndpointByID returns a single endpoint by id.
func (s *Service) EndpointByID(ctx context.Context, id string) (*Endpoint, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, base_url, auth_type, username, poll_schedule,
		       enabled, COALESCE(last_sync_at, ''), last_sync_status,
		       last_sync_error, users_count, groups_count, created_at, updated_at,
		       CASE WHEN token_hash != '' THEN 1 ELSE 0 END, COALESCE(token_rotated_at, '')
		  FROM scim_endpoints
		 WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("loading scim endpoint: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, ErrEndpointNotFound
	}
	var e Endpoint
	var enabled, hasToken int
	if err := rows.Scan(&e.ID, &e.Name, &e.BaseURL, &e.AuthType, &e.Username, &e.PollSchedule,
		&enabled, &e.LastSyncAt, &e.LastSyncStatus, &e.LastSyncError,
		&e.UsersCount, &e.GroupsCount, &e.CreatedAt, &e.UpdatedAt, &hasToken, &e.TokenRotatedAt); err != nil {
		return nil, err
	}
	e.Enabled = enabled == 1
	e.HasToken = hasToken == 1
	return &e, nil
}

// getRaw loads the encrypted token + hash so the service can rewrite
// them on update without going through the outward model.
type rawEndpoint struct {
	ID             string
	EncryptedToken string
	TokenHash      string
}

func (s *Service) getRaw(ctx context.Context, id string) (*rawEndpoint, error) {
	var r rawEndpoint
	err := s.db.QueryRowContext(ctx,
		`SELECT id, encrypted_token, token_hash FROM scim_endpoints WHERE id = ?`, id,
	).Scan(&r.ID, &r.EncryptedToken, &r.TokenHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrEndpointNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading raw scim endpoint: %w", err)
	}
	return &r, nil
}

// hashAndEncrypt accepts a plaintext token. Empty plaintext yields
// empty hash + empty ciphertext (used for endpoints with no auth).
func (s *Service) hashAndEncrypt(plaintext string) (hash, enc string, err error) {
	if plaintext == "" {
		return "", "", nil
	}
	hash = s.enc.StableHash(plaintext)
	enc, err = s.enc.Encrypt(plaintext)
	return hash, enc, err
}

// LookupByBearerToken returns the endpoint whose token_hash matches
// the supplied bearer token (verified constant-time after decrypt).
// Returns ErrEndpointNotFound when the token is not recognised.
func (s *Service) LookupByBearerToken(ctx context.Context, bearer string) (*Endpoint, error) {
	if bearer == "" {
		return nil, ErrEndpointNotFound
	}
	hash := s.enc.StableHash(bearer)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, encrypted_token, token_hash FROM scim_endpoints
		 WHERE token_hash = ? AND enabled = 1`, hash)
	if err != nil {
		return nil, fmt.Errorf("looking up scim endpoint: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	for rows.Next() {
		var id, enc, storedHash string
		if err := rows.Scan(&id, &enc, &storedHash); err != nil {
			return nil, err
		}
		dec, err := s.enc.Decrypt(enc)
		if err != nil {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(dec), []byte(bearer)) != 1 {
			continue
		}
		return s.EndpointByID(ctx, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nil, ErrEndpointNotFound
}

// ErrTokenRotationUnsupported is returned when an endpoint does not use bearer
// auth, so there is no McHarbor-issued token to rotate.
var ErrTokenRotationUnsupported = errors.New("scim endpoint does not use a bearer token")

// RotateToken replaces an endpoint's bearer token with a freshly generated one
// in a single UPDATE, so the old token stops authenticating the moment the new
// one exists. The plaintext is returned once for the caller to show.
func (s *Service) RotateToken(ctx context.Context, endpointID string) (string, *Endpoint, error) {
	ep, err := s.EndpointByID(ctx, endpointID)
	if err != nil {
		return "", nil, err
	}
	if ep.AuthType != "bearer" {
		return "", nil, ErrTokenRotationUnsupported
	}
	plaintext, hash, encrypted, err := NewBearerToken(s.enc)
	if err != nil {
		return "", nil, err
	}
	if hash == "" || encrypted == "" {
		return "", nil, errors.New("encryption is not configured")
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE scim_endpoints
		   SET encrypted_token = ?, token_hash = ?, token_rotated_at = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		encrypted, hash, time.Now().UTC().Format(time.RFC3339), endpointID)
	if err != nil {
		return "", nil, fmt.Errorf("rotating scim token: %w", err)
	}
	if db.RowsAffected(res) == 0 {
		return "", nil, ErrEndpointNotFound
	}
	updated, err := s.EndpointByID(ctx, endpointID)
	if err != nil {
		return "", nil, err
	}
	return plaintext, updated, nil
}

// DueEndpoints returns the ids of endpoints that should be polled
// on the supplied tick. An endpoint is due when its cron expression
// matches `now.Truncate(time.Minute)` OR when next_sync_at is set
// (the admin's manual sync-now request).
func (s *Service) DueEndpoints(ctx context.Context, now time.Time) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, COALESCE(poll_schedule, ''), COALESCE(next_sync_at, '')
		  FROM scim_endpoints
		 WHERE enabled = 1
		 LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing due endpoints: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	var due []string
	for rows.Next() {
		var id, schedule, nextSync string
		if err := rows.Scan(&id, &schedule, &nextSync); err != nil {
			return nil, err
		}
		if nextSync != "" {
			due = append(due, id)
			continue
		}
		if cronMatches(schedule, now.Truncate(time.Minute)) {
			due = append(due, id)
		}
	}
	return due, rows.Err()
}

// DecryptToken returns the plaintext bearer token for an endpoint.
// Used by the poller when pulling from the IdP.
func (s *Service) DecryptToken(ctx context.Context, endpointID string) (string, error) {
	var enc string
	err := s.db.QueryRowContext(ctx,
		`SELECT encrypted_token FROM scim_endpoints WHERE id = ?`, endpointID,
	).Scan(&enc)
	if err != nil {
		return "", fmt.Errorf("loading encrypted token: %w", err)
	}
	if enc == "" {
		return "", nil
	}
	return s.enc.Decrypt(enc)
}

// RequestSyncNow sets next_sync_at to an epoch-zero sentinel so the
// poller treats the endpoint as overdue on its next tick. The
// caller does not block on the actual pull — it merely enqueues a
// request. Returns ErrEndpointNotFound when the id is unknown.
func (s *Service) RequestSyncNow(ctx context.Context, endpointID string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE scim_endpoints
		   SET next_sync_at = '1970-01-01 00:00:00',
		       updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`, endpointID)
	if err != nil {
		return fmt.Errorf("requesting scim sync: %w", err)
	}
	if db.RowsAffected(res) == 0 {
		return ErrEndpointNotFound
	}
	return nil
}

// ConsumeForceRequest atomically reads and clears next_sync_at for
// the supplied endpoint. The poller calls this before each tick so
// force requests only fire once.
func (s *Service) ConsumeForceRequest(ctx context.Context, endpointID string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE scim_endpoints
		   SET next_sync_at = NULL
		 WHERE id = ? AND next_sync_at IS NOT NULL`, endpointID)
	if err != nil {
		return false, fmt.Errorf("consuming scim force request: %w", err)
	}
	return db.RowsAffected(res) > 0, nil
}

// SyncHistory returns the most-recent sync log rows for an endpoint.
func (s *Service) SyncHistory(ctx context.Context, endpointID string, limit int) ([]SyncLogEntry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, endpoint_id, direction, started_at, COALESCE(finished_at, ''),
		       status, users_created, users_updated, users_deleted,
		       groups_created, groups_updated, groups_deleted, error,
		       users_seen, users_linked, users_unmatched, users_disabled, users_skipped,
		       groups_seen, dry_run, run_trigger, item_errors
		  FROM scim_sync_log
		 WHERE endpoint_id = ?
		 ORDER BY started_at DESC, id DESC
		 LIMIT ?`, endpointID, limit)
	if err != nil {
		return nil, fmt.Errorf("loading sync history: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	var out []SyncLogEntry
	for rows.Next() {
		var e SyncLogEntry
		var dryRun int
		var notes string
		if err := rows.Scan(&e.ID, &e.EndpointID, &e.Direction, &e.StartedAt, &e.FinishedAt,
			&e.Status, &e.UsersCreated, &e.UsersUpdated, &e.UsersDeleted,
			&e.GroupsCreated, &e.GroupsUpdated, &e.GroupsDeleted, &e.Error,
			&e.UsersSeen, &e.UsersLinked, &e.UsersUnmatched, &e.UsersDisabled, &e.UsersSkipped,
			&e.GroupsSeen, &dryRun, &e.Trigger, &notes); err != nil {
			return nil, err
		}
		e.DryRun = dryRun == 1
		e.Notes = []string{}
		if notes != "" {
			if err := json.Unmarshal([]byte(notes), &e.Notes); err != nil {
				e.Notes = []string{notes}
			}
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ExtractBearerToken pulls the bearer token from the Authorization
// header. Returns "" when no header is present.
func ExtractBearerToken(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if auth == "" {
		return ""
	}
	const prefix = "Bearer "
	if len(auth) < len(prefix) {
		return ""
	}
	if !strings.EqualFold(auth[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(auth[len(prefix):])
}

// isUniqueViolation reports whether the SQL error is a unique-index
// violation. SQLite/Postgres/MySQL differ but the message contains
// "UNIQUE" or "duplicate" reliably enough.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "duplicate")
}

func newID() (string, error) {
	return newXID()
}
