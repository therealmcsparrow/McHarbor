// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package registry

import (
	"database/sql"
	"fmt"
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
func (s *Service) List(page, perPage int) ([]Registry, int64, error) {
	var total int64
	if err := s.db.QueryRow("SELECT COUNT(*) FROM registries").Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting registries: %w", err)
	}

	offset := (page - 1) * perPage
	rows, err := s.db.Query(
		`SELECT id, name, url, username, is_default, created_at, updated_at
		 FROM registries ORDER BY name ASC LIMIT ? OFFSET ?`,
		perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("listing registries: %w", err)
	}
	defer rows.Close()

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
func (s *Service) ByID(id string) (*Registry, error) {
	var reg Registry
	var username sql.NullString
	var isDefault sql.NullBool

	err := s.db.QueryRow(
		`SELECT id, name, url, username, is_default, created_at, updated_at
		 FROM registries WHERE id = ?`, id,
	).Scan(&reg.ID, &reg.Name, &reg.URL, &username, &isDefault, &reg.CreatedAt, &reg.UpdatedAt)

	if err == sql.ErrNoRows {
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
func (s *Service) Create(input CreateRegistryInput) (*Registry, error) {
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

	_, err := s.db.Exec(
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
func (s *Service) Update(id string, input UpdateRegistryInput) (*Registry, error) {
	// Verify existence
	var existsID string
	if err := s.db.QueryRow("SELECT id FROM registries WHERE id = ?", id).Scan(&existsID); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("checking registry existence: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	if input.Name != nil {
		if _, err := s.db.Exec("UPDATE registries SET name = ?, updated_at = ? WHERE id = ?", *input.Name, now, id); err != nil {
			return nil, fmt.Errorf("updating registry name: %w", err)
		}
	}
	if input.URL != nil {
		if _, err := s.db.Exec("UPDATE registries SET url = ?, updated_at = ? WHERE id = ?", *input.URL, now, id); err != nil {
			return nil, fmt.Errorf("updating registry url: %w", err)
		}
	}
	if input.Username != nil {
		if _, err := s.db.Exec("UPDATE registries SET username = ?, updated_at = ? WHERE id = ?", *input.Username, now, id); err != nil {
			return nil, fmt.Errorf("updating registry username: %w", err)
		}
	}
	if input.Password != nil {
		encrypted, err := s.enc.Encrypt(*input.Password)
		if err != nil {
			return nil, fmt.Errorf("encrypting registry password: %w", err)
		}
		if _, err := s.db.Exec("UPDATE registries SET password = ?, updated_at = ? WHERE id = ?", encrypted, now, id); err != nil {
			return nil, fmt.Errorf("updating registry password: %w", err)
		}
	}
	if input.IsDefault != nil {
		if *input.IsDefault {
			// Clear all other defaults first
			if _, err := s.db.Exec("UPDATE registries SET is_default = 0, updated_at = ?", now); err != nil {
				return nil, fmt.Errorf("clearing default registries: %w", err)
			}
		}
		if _, err := s.db.Exec("UPDATE registries SET is_default = ?, updated_at = ? WHERE id = ?", *input.IsDefault, now, id); err != nil {
			return nil, fmt.Errorf("updating registry default flag: %w", err)
		}
	}

	return s.ByID(id)
}

// Delete removes a registry by ID. Returns true if a row was deleted.
func (s *Service) Delete(id string) (bool, error) {
	result, err := s.db.Exec("DELETE FROM registries WHERE id = ?", id)
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
func (s *Service) TestConnection(id string) (*ConnectionTestResult, error) {
	var regURL, username, encPassword sql.NullString
	err := s.db.QueryRow(
		"SELECT url, username, password FROM registries WHERE id = ?", id,
	).Scan(&regURL, &username, &encPassword)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("looking up registry %s for connection test: %w", id, err)
	}

	testURL := regURL.String + "/v2/"
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(testURL)
	if err != nil {
		return &ConnectionTestResult{Reachable: false, StatusCode: 0}, nil
	}
	defer resp.Body.Close()

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
