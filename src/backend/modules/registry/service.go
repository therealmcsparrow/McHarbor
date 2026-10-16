// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package registry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
)

// Service handles registry CRUD operations against the database.
type Service struct {
	db  *sql.DB
	enc *encryption.Service
}

// NewService creates a new registry service.
func NewService(db *sql.DB, enc *encryption.Service) *Service {
	return &Service{db: db, enc: enc}
}

// List returns a paginated list of registries. Passwords are omitted.
func (s *Service) List(ctx context.Context, page, perPage int) ([]Registry, int64, error) {
	var total int64
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM registries").Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting registries: %w", err)
	}

	offset := (page - 1) * perPage
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, url, username, is_default, created_at, updated_at
		 FROM registries ORDER BY name ASC LIMIT ? OFFSET ?`,
		perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("listing registries: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing registry rows failed", "error", err)
		}
	}()

	var items []Registry
	for rows.Next() {
		reg, err := scanRegistry(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning registry row: %w", err)
		}
		items = append(items, reg)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating registry rows: %w", err)
	}

	if items == nil {
		items = []Registry{}
	}

	return items, total, nil
}

// ByID returns a single registry by ID. Password is omitted.
func (s *Service) ByID(ctx context.Context, id string) (*Registry, error) {
	var reg Registry
	var username sql.NullString
	var isDefault sql.NullBool

	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, url, username, is_default, created_at, updated_at
		 FROM registries WHERE id = ?`, id,
	).Scan(&reg.ID, &reg.Name, &reg.URL, &username, &isDefault, &reg.CreatedAt, &reg.UpdatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting registry %s: %w", id, err)
	}

	reg.Username = username.String
	reg.IsDefault = isDefault.Bool

	return &reg, nil
}

// Create inserts a new registry. The password is encrypted before storage.
func (s *Service) Create(ctx context.Context, input CreateRegistryInput) (*Registry, error) {
	encPassword := ""
	if input.Password != "" {
		encrypted, err := s.enc.Encrypt(input.Password)
		if err != nil {
			return nil, fmt.Errorf("encrypting registry password: %w", err)
		}
		encPassword = encrypted
	}

	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.ExecContext(ctx,
		// password_secret_id stays NULL; the start-up adoption pass fills it.
		`INSERT INTO registries (id, name, url, username, password, is_default, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 0, ?, ?)`,
		id, input.Name, input.URL, input.Username, encPassword, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting registry: %w", err)
	}

	return &Registry{
		ID:        id,
		Name:      input.Name,
		URL:       input.URL,
		Username:  input.Username,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// Update applies partial updates to an existing registry. Returns the
// updated registry or nil if the ID was not found.
func (s *Service) Update(ctx context.Context, id string, input UpdateRegistryInput) (*Registry, error) {
	// Verify existence
	var existsID string
	if err := s.db.QueryRowContext(ctx, "SELECT id FROM registries WHERE id = ?", id).Scan(&existsID); errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("checking registry existence: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	if input.Name != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE registries SET name = ?, updated_at = ? WHERE id = ?", *input.Name, now, id); err != nil {
			return nil, fmt.Errorf("updating registry name: %w", err)
		}
	}
	if input.URL != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE registries SET url = ?, updated_at = ? WHERE id = ?", *input.URL, now, id); err != nil {
			return nil, fmt.Errorf("updating registry url: %w", err)
		}
	}
	if input.Username != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE registries SET username = ?, updated_at = ? WHERE id = ?", *input.Username, now, id); err != nil {
			return nil, fmt.Errorf("updating registry username: %w", err)
		}
	}
	if input.Password != nil {
		encrypted, err := s.enc.Encrypt(*input.Password)
		if err != nil {
			return nil, fmt.Errorf("encrypting registry password: %w", err)
		}
		// Clearing password_secret_id keeps the vault copy from going stale:
		// the read-through falls back to the column just written and the next
		// boot re-adopts it.
		if _, err := s.db.ExecContext(ctx, "UPDATE registries SET password = ?, password_secret_id = NULL, updated_at = ? WHERE id = ?", encrypted, now, id); err != nil {
			return nil, fmt.Errorf("updating registry password: %w", err)
		}
	}
	if input.IsDefault != nil {
		if *input.IsDefault {
			// Clear all other defaults first
			if _, err := s.db.ExecContext(ctx, "UPDATE registries SET is_default = 0, updated_at = ?", now); err != nil {
				return nil, fmt.Errorf("clearing default registries: %w", err)
			}
		}
		if _, err := s.db.ExecContext(ctx, "UPDATE registries SET is_default = ?, updated_at = ? WHERE id = ?", *input.IsDefault, now, id); err != nil {
			return nil, fmt.Errorf("updating registry default flag: %w", err)
		}
	}

	return s.ByID(ctx, id)
}

// Delete removes a registry by ID. Returns true if a row was deleted.
func (s *Service) Delete(ctx context.Context, id string) (bool, error) {
	result, err := s.db.ExecContext(ctx, "DELETE FROM registries WHERE id = ?", id)
	if err != nil {
		return false, fmt.Errorf("deleting registry %s: %w", id, err)
	}

	return db.RowsAffected(result) > 0, nil
}

// ConnectionTestResult holds the outcome of a registry connection test.
type ConnectionTestResult struct {
	Reachable  bool `json:"success"`
	StatusCode int  `json:"statusCode"`
}

// TestConnection checks whether the registry's /v2/ endpoint is reachable.
// Returns nil result with no error when the registry ID is not found.
func (s *Service) TestConnection(ctx context.Context, id string) (*ConnectionTestResult, error) {
	var regURL, username, encPassword sql.NullString
	err := s.db.QueryRowContext(ctx,
		"SELECT url, username, password FROM registries WHERE id = ?", id,
	).Scan(&regURL, &username, &encPassword)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("looking up registry %s for connection test: %w", id, err)
	}

	testURL := regURL.String + "/v2/"
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, testURL, nil)
	if err != nil {
		return &ConnectionTestResult{Reachable: false, StatusCode: 0}, nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return &ConnectionTestResult{Reachable: false, StatusCode: 0}, nil
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Debug("closing registry test response body failed", "error", err)
		}
	}()

	// 200 or 401 means the registry is reachable
	reachable := resp.StatusCode == 200 || resp.StatusCode == 401

	return &ConnectionTestResult{
		Reachable:  reachable,
		StatusCode: resp.StatusCode,
	}, nil
}

// scanRegistry scans a row from the registries table (without password).
func scanRegistry(rows *sql.Rows) (Registry, error) {
	var reg Registry
	var username sql.NullString
	var isDefault sql.NullBool

	if err := rows.Scan(&reg.ID, &reg.Name, &reg.URL, &username, &isDefault,
		&reg.CreatedAt, &reg.UpdatedAt); err != nil {
		return Registry{}, err
	}

	reg.Username = username.String
	reg.IsDefault = isDefault.Bool

	return reg, nil
}
