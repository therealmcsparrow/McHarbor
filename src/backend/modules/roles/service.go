// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package roles

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/xid"
)

// Service handles role operations.
type Service struct {
	db *sql.DB
}

// NewService creates a new roles service.
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// List returns all roles.
func (s *Service) List() ([]Role, error) {
	rows, err := s.db.Query(
		`SELECT id, name, description, permissions, is_system, created_at, updated_at
		 FROM roles ORDER BY is_system DESC, name ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying roles: %w", err)
	}
	defer rows.Close()

	var items []Role
	for rows.Next() {
		r, err := scanRole(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *r)
	}
	if items == nil {
		items = []Role{}
	}
	return items, rows.Err()
}

// Get returns a single role by ID.
func (s *Service) Get(id string) (*Role, error) {
	row := s.db.QueryRow(
		`SELECT id, name, description, permissions, is_system, created_at, updated_at
		 FROM roles WHERE id = ?`, id,
	)

	var r Role
	var desc sql.NullString
	var permsJSON string

	err := row.Scan(&r.ID, &r.Name, &desc, &permsJSON, &r.IsSystem, &r.CreatedAt, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scanning role: %w", err)
	}

	r.Description = desc.String
	if err := json.Unmarshal([]byte(permsJSON), &r.Permissions); err != nil {
		r.Permissions = []string{}
	}

	return &r, nil
}

// Create creates a new custom role.
func (s *Service) Create(input *CreateRoleInput) (*Role, error) {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	permsJSON, err := json.Marshal(input.Permissions)
	if err != nil {
		return nil, fmt.Errorf("marshaling permissions: %w", err)
	}

	_, err = s.db.Exec(
		`INSERT INTO roles (id, name, description, permissions, is_system, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 0, ?, ?)`,
		id, input.Name, input.Description, string(permsJSON), now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting role: %w", err)
	}

	return s.Get(id)
}

// Update updates an existing custom role.
func (s *Service) Update(id string, input *UpdateRoleInput) (*Role, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	if input.Name != nil {
		if _, err := s.db.Exec("UPDATE roles SET name = ?, updated_at = ? WHERE id = ?", *input.Name, now, id); err != nil {
			return nil, fmt.Errorf("updating role name: %w", err)
		}
	}
	if input.Description != nil {
		if _, err := s.db.Exec("UPDATE roles SET description = ?, updated_at = ? WHERE id = ?", *input.Description, now, id); err != nil {
			return nil, fmt.Errorf("updating role description: %w", err)
		}
	}
	if input.Permissions != nil {
		permsJSON, err := json.Marshal(input.Permissions)
		if err != nil {
			return nil, fmt.Errorf("marshaling permissions: %w", err)
		}
		if _, err := s.db.Exec("UPDATE roles SET permissions = ?, updated_at = ? WHERE id = ?", string(permsJSON), now, id); err != nil {
			return nil, fmt.Errorf("updating role permissions: %w", err)
		}
	}

	return s.Get(id)
}

// Delete removes a custom role.
func (s *Service) Delete(id string) error {
	_, err := s.db.Exec("DELETE FROM roles WHERE id = ? AND is_system = 0", id)
	if err != nil {
		return fmt.Errorf("deleting role: %w", err)
	}
	return nil
}

// IsSystem checks if a role is a system role.
func (s *Service) IsSystem(id string) (bool, error) {
	var isSystem bool
	err := s.db.QueryRow("SELECT is_system FROM roles WHERE id = ?", id).Scan(&isSystem)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking system role: %w", err)
	}
	return isSystem, nil
}

// NameExists checks if a role name is already taken (excluding a specific ID).
func (s *Service) NameExists(name, excludeID string) (bool, error) {
	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM roles WHERE name = ? AND id != ?", name, excludeID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("checking role name: %w", err)
	}
	return count > 0, nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanRole(s scannable) (*Role, error) {
	var r Role
	var desc sql.NullString
	var permsJSON string

	if err := s.Scan(&r.ID, &r.Name, &desc, &permsJSON, &r.IsSystem, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, fmt.Errorf("scanning role: %w", err)
	}

	r.Description = desc.String
	if err := json.Unmarshal([]byte(permsJSON), &r.Permissions); err != nil {
		r.Permissions = []string{}
	}

	return &r, nil
}
