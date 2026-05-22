// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package api_keys

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

const keyPrefix = "mch_"

// Service handles API key operations.
type Service struct {
	db *sql.DB
}

// NewService creates a new API keys service.
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// List returns API keys. If userID is empty, returns all keys (admin).
func (s *Service) List(userID string) ([]APIKey, error) {
	var rows *sql.Rows
	var err error

	if userID == "" {
		rows, err = s.db.Query(
			`SELECT ak.id, ak.user_id, u.username, ak.name, ak.key_prefix, ak.scopes,
			        ak.expires_at, ak.last_used_at, ak.is_active, ak.created_at, ak.updated_at
			 FROM api_keys ak
			 INNER JOIN users u ON u.id = ak.user_id
			 ORDER BY ak.created_at DESC`,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT ak.id, ak.user_id, u.username, ak.name, ak.key_prefix, ak.scopes,
			        ak.expires_at, ak.last_used_at, ak.is_active, ak.created_at, ak.updated_at
			 FROM api_keys ak
			 INNER JOIN users u ON u.id = ak.user_id
			 WHERE ak.user_id = ?
			 ORDER BY ak.created_at DESC`,
			userID,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("querying api keys: %w", err)
	}
	defer rows.Close()

	var items []APIKey
	for rows.Next() {
		k, err := scanKey(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *k)
	}
	if items == nil {
		items = []APIKey{}
	}
	return items, rows.Err()
}

// Get returns a single API key.
func (s *Service) Get(id string) (*APIKey, error) {
	row := s.db.QueryRow(
		`SELECT ak.id, ak.user_id, u.username, ak.name, ak.key_prefix, ak.scopes,
		        ak.expires_at, ak.last_used_at, ak.is_active, ak.created_at, ak.updated_at
		 FROM api_keys ak
		 INNER JOIN users u ON u.id = ak.user_id
		 WHERE ak.id = ?`, id,
	)

	k, err := scanKeyRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying api key: %w", err)
	}
	return k, nil
}

// Create generates a new API key and returns it with the plaintext key.
func (s *Service) Create(userID string, input *CreateAPIKeyInput) (*CreateAPIKeyResult, error) {
	// Generate random key: mch_ + 48 hex chars (24 bytes)
	keyBytes := make([]byte, 24)
	if _, err := rand.Read(keyBytes); err != nil {
		return nil, fmt.Errorf("generating key bytes: %w", err)
	}
	plainKey := keyPrefix + hex.EncodeToString(keyBytes)

	// SHA-256 hash for storage
	hash := sha256.Sum256([]byte(plainKey))
	keyHash := hex.EncodeToString(hash[:])

	// Key prefix for display: first 12 chars + "..."
	displayPrefix := plainKey[:12] + "..."

	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	scopesJSON, err := json.Marshal(input.Scopes)
	if err != nil {
		return nil, fmt.Errorf("marshaling scopes: %w", err)
	}

	_, err = s.db.Exec(
		`INSERT INTO api_keys (id, user_id, name, key_hash, key_prefix, scopes, expires_at, is_active, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		id, userID, input.Name, keyHash, displayPrefix, string(scopesJSON), input.ExpiresAt, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting api key: %w", err)
	}

	key, err := s.Get(id)
	if err != nil {
		return nil, err
	}

	return &CreateAPIKeyResult{
		APIKey: *key,
		Key:    plainKey,
	}, nil
}

// Revoke deactivates an API key.
func (s *Service) Revoke(id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.Exec(
		"UPDATE api_keys SET is_active = 0, updated_at = ? WHERE id = ?", now, id,
	)
	if err != nil {
		return fmt.Errorf("revoking api key: %w", err)
	}
	if db.RowsAffected(result) == 0 {
		return fmt.Errorf("api key not found")
	}
	return nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanKey(s scannable) (*APIKey, error) {
	var k APIKey
	var scopesJSON string
	var expiresAt, lastUsedAt sql.NullString

	if err := s.Scan(&k.ID, &k.UserID, &k.Username, &k.Name, &k.KeyPrefix, &scopesJSON,
		&expiresAt, &lastUsedAt, &k.IsActive, &k.CreatedAt, &k.UpdatedAt); err != nil {
		return nil, fmt.Errorf("scanning api key: %w", err)
	}

	if err := json.Unmarshal([]byte(scopesJSON), &k.Scopes); err != nil {
		k.Scopes = []string{}
	}
	if expiresAt.Valid {
		k.ExpiresAt = &expiresAt.String
	}
	if lastUsedAt.Valid {
		k.LastUsedAt = &lastUsedAt.String
	}

	return &k, nil
}

func scanKeyRow(row *sql.Row) (*APIKey, error) {
	var k APIKey
	var scopesJSON string
	var expiresAt, lastUsedAt sql.NullString

	err := row.Scan(&k.ID, &k.UserID, &k.Username, &k.Name, &k.KeyPrefix, &scopesJSON,
		&expiresAt, &lastUsedAt, &k.IsActive, &k.CreatedAt, &k.UpdatedAt)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(scopesJSON), &k.Scopes); err != nil {
		k.Scopes = []string{}
	}
	if expiresAt.Valid {
		k.ExpiresAt = &expiresAt.String
	}
	if lastUsedAt.Valid {
		k.LastUsedAt = &lastUsedAt.String
	}

	return &k, nil
}
