// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package roles

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
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

// ActiveUser is one active account considered by the role-impact preview.
// DisplayName and Email are nil when the column is NULL.
type ActiveUser struct {
	ID          string
	Username    string
	DisplayName *string
	Email       *string
	IsActive    bool
}

// ActiveUsers returns up to limit active users ordered by username, for the
// role-impact preview.
func (s *Service) ActiveUsers(ctx context.Context, limit int) ([]ActiveUser, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, username, display_name, email, is_active
		 FROM users
		 WHERE is_active = 1
		 ORDER BY username ASC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("querying active users: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("roles: closing rows failed", "error", cerr)
		}
	}()
	var users []ActiveUser
	for rows.Next() {
		var u ActiveUser
		if err := rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Email, &u.IsActive); err != nil {
			return nil, fmt.Errorf("scanning active user: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// List returns all roles.
func (s *Service) List(ctx context.Context) ([]Role, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT r.id, r.name, r.description, r.permissions, r.is_system, r.created_at, r.updated_at,
		        (SELECT COUNT(DISTINCT ur.user_id) FROM user_roles ur WHERE ur.role_id = r.id) AS user_count,
		        (SELECT COUNT(DISTINCT gr.group_id) FROM group_roles gr WHERE gr.role_id = r.id) AS group_count,
		        (SELECT COUNT(DISTINCT ar.api_key_id) FROM api_key_roles ar WHERE ar.role_id = r.id) AS api_key_count
		 FROM roles r ORDER BY r.is_system DESC, r.name ASC LIMIT 1000`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying roles: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("roles: closing rows failed", "error", cerr)
		}
	}()

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
func (s *Service) Get(ctx context.Context, id string) (*Role, error) {
	row := s.db.QueryRowContext(ctx,
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
func (s *Service) Create(ctx context.Context, input *CreateRoleInput) (*Role, error) {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	permsJSON, err := json.Marshal(input.Permissions)
	if err != nil {
		return nil, fmt.Errorf("encoding role permissions: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO roles (id, name, description, permissions, is_system, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 0, ?, ?)`,
		id, input.Name, input.Description, string(permsJSON), now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting role: %w", err)
	}

	return s.Get(ctx, id)
}

// Update updates an existing custom role.
func (s *Service) Update(ctx context.Context, id string, input *UpdateRoleInput) (*Role, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	if input.Name != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE roles SET name = ?, updated_at = ? WHERE id = ?", *input.Name, now, id); err != nil {
			return nil, fmt.Errorf("updating role name: %w", err)
		}
	}
	if input.Description != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE roles SET description = ?, updated_at = ? WHERE id = ?", *input.Description, now, id); err != nil {
			return nil, fmt.Errorf("updating role description: %w", err)
		}
	}
	if input.Permissions != nil {
		permsJSON, err := json.Marshal(input.Permissions)
		if err != nil {
			return nil, fmt.Errorf("encoding role permissions: %w", err)
		}
		if _, err := s.db.ExecContext(ctx, "UPDATE roles SET permissions = ?, updated_at = ? WHERE id = ?", string(permsJSON), now, id); err != nil {
			return nil, fmt.Errorf("updating role permissions: %w", err)
		}
	}

	return s.Get(ctx, id)
}

// Delete removes a custom role.
func (s *Service) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM roles WHERE id = ? AND is_system = 0", id)
	if err != nil {
		return fmt.Errorf("deleting role: %w", err)
	}
	return nil
}

// IsSystem checks if a role is a system role.
func (s *Service) IsSystem(ctx context.Context, id string) (bool, error) {
	var isSystem bool
	err := s.db.QueryRowContext(ctx, "SELECT is_system FROM roles WHERE id = ?", id).Scan(&isSystem)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking system role: %w", err)
	}
	return isSystem, nil
}

// NameExists checks if a role name is already taken (excluding a specific ID).
func (s *Service) NameExists(ctx context.Context, name, excludeID string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
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
	var userCount, groupCount, apiKeyCount int

	if err := s.Scan(&r.ID, &r.Name, &desc, &permsJSON, &r.IsSystem, &r.CreatedAt, &r.UpdatedAt,
		&userCount, &groupCount, &apiKeyCount); err != nil {
		return nil, fmt.Errorf("scanning role: %w", err)
	}
	r.UserCount = &userCount
	r.GroupCount = &groupCount
	r.APIKeyCount = &apiKeyCount

	r.Description = desc.String
	if err := json.Unmarshal([]byte(permsJSON), &r.Permissions); err != nil {
		r.Permissions = []string{}
	}

	return &r, nil
}
