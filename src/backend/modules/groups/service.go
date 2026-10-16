// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package groups

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// Service handles group operations.
type Service struct {
	db *sql.DB
}

// NewService creates a new groups service.
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// List returns all groups with member counts.
func (s *Service) List(ctx context.Context) ([]Group, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT g.id, g.name, g.description, g.is_system, g.created_at, g.updated_at,
		        (SELECT COUNT(*) FROM group_members gm WHERE gm.group_id = g.id) AS member_count,
		        (SELECT GROUP_CONCAT(name, char(31)) FROM (SELECT DISTINCT r.name AS name FROM group_roles gr JOIN roles r ON r.id = gr.role_id WHERE gr.group_id = g.id)) AS role_names
		 FROM groups g ORDER BY g.is_system DESC, g.name ASC LIMIT 1000`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying groups: %w", err)
	}
	defer closeRows(rows)

	var items []Group
	for rows.Next() {
		var g Group
		var desc, roleNames sql.NullString
		if err := rows.Scan(&g.ID, &g.Name, &desc, &g.IsSystem, &g.CreatedAt, &g.UpdatedAt, &g.MemberCount, &roleNames); err != nil {
			return nil, fmt.Errorf("scanning group: %w", err)
		}
		g.Description = desc.String
		g.RoleNames = splitGroupConcat(roleNames)
		items = append(items, g)
	}
	if items == nil {
		items = []Group{}
	}
	return items, rows.Err()
}

// Get returns a group with members and role assignments.
func (s *Service) Get(ctx context.Context, id string) (*Group, error) {
	var g Group
	var desc sql.NullString

	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, description, is_system, created_at, updated_at FROM groups WHERE id = ?`, id,
	).Scan(&g.ID, &g.Name, &desc, &g.IsSystem, &g.CreatedAt, &g.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying group: %w", err)
	}
	g.Description = desc.String

	// Load members
	members, err := s.listMembers(ctx, id)
	if err != nil {
		return nil, err
	}
	g.Members = members
	g.MemberCount = len(members)

	// Load role assignments
	roles, err := s.listRoles(ctx, id)
	if err != nil {
		return nil, err
	}
	g.Roles = roles

	return &g, nil
}

// Create creates a new group.
func (s *Service) Create(ctx context.Context, input *CreateGroupInput) (*Group, error) {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO groups (id, name, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		id, input.Name, input.Description, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting group: %w", err)
	}

	return s.Get(ctx, id)
}

// Update updates an existing group.
func (s *Service) Update(ctx context.Context, id string, input *UpdateGroupInput) (*Group, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	if input.Name != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE groups SET name = ?, updated_at = ? WHERE id = ?", *input.Name, now, id); err != nil {
			return nil, fmt.Errorf("updating group name: %w", err)
		}
	}
	if input.Description != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE groups SET description = ?, updated_at = ? WHERE id = ?", *input.Description, now, id); err != nil {
			return nil, fmt.Errorf("updating group description: %w", err)
		}
	}

	return s.Get(ctx, id)
}

// Delete removes a group and its memberships/role assignments (CASCADE).
func (s *Service) Delete(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM groups WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting group: %w", err)
	}
	if db.RowsAffected(result) == 0 {
		return fmt.Errorf("group not found")
	}
	return nil
}

// IsSystem checks if a group is a system group.
func (s *Service) IsSystem(ctx context.Context, id string) (bool, error) {
	var isSystem bool
	err := s.db.QueryRowContext(ctx, "SELECT is_system FROM groups WHERE id = ?", id).Scan(&isSystem)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking system group: %w", err)
	}
	return isSystem, nil
}

// NameExists checks if a group name is already taken.
func (s *Service) NameExists(ctx context.Context, name, excludeID string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM groups WHERE name = ? AND id != ?", name, excludeID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("checking group name: %w", err)
	}
	return count > 0, nil
}

// AddMember adds a user to a group.
func (s *Service) AddMember(ctx context.Context, groupID, userID string) error {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO group_members (id, group_id, user_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		id, groupID, userID, now, now,
	)
	if err != nil {
		return fmt.Errorf("adding group member: %w", err)
	}
	return nil
}

// RemoveMember removes a user from a group.
func (s *Service) RemoveMember(ctx context.Context, groupID, userID string) error {
	result, err := s.db.ExecContext(ctx,
		"DELETE FROM group_members WHERE group_id = ? AND user_id = ?", groupID, userID,
	)
	if err != nil {
		return fmt.Errorf("removing group member: %w", err)
	}
	if db.RowsAffected(result) == 0 {
		return fmt.Errorf("member not found")
	}
	return nil
}

// MemberExists checks if a user is already in a group.
func (s *Service) MemberExists(ctx context.Context, groupID, userID string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM group_members WHERE group_id = ? AND user_id = ?", groupID, userID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("checking group membership: %w", err)
	}
	return count > 0, nil
}

// AssignRole assigns a role to a group, optionally scoped to an environment or stack.
func (s *Service) AssignRole(ctx context.Context, groupID string, input AssignRoleInput) (string, error) {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO group_roles (id, group_id, role_id, environment_id, stack_name, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, groupID, input.RoleID, input.EnvironmentID, input.StackName, now, now,
	)
	if err != nil {
		return "", fmt.Errorf("assigning group role: %w", err)
	}
	return id, nil
}

// UnassignRole removes a role assignment from a group.
func (s *Service) UnassignRole(ctx context.Context, assignmentID string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM group_roles WHERE id = ?", assignmentID)
	if err != nil {
		return fmt.Errorf("unassigning group role: %w", err)
	}
	if db.RowsAffected(result) == 0 {
		return fmt.Errorf("role assignment not found")
	}
	return nil
}

func (s *Service) listMembers(ctx context.Context, groupID string) ([]GroupMember, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT gm.id, gm.user_id, u.username
		 FROM group_members gm
		 INNER JOIN users u ON u.id = gm.user_id
		 WHERE gm.group_id = ?
		 ORDER BY u.username ASC`,
		groupID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying group members: %w", err)
	}
	defer closeRows(rows)

	var members []GroupMember
	for rows.Next() {
		var m GroupMember
		if err := rows.Scan(&m.ID, &m.UserID, &m.Username); err != nil {
			return nil, fmt.Errorf("scanning group member: %w", err)
		}
		members = append(members, m)
	}
	if members == nil {
		members = []GroupMember{}
	}
	return members, rows.Err()
}

// splitGroupConcat splits a `GROUP_CONCAT(x, char(31))` result on the unit
// separator, returning an empty (never nil) slice for a NULL aggregate.
func splitGroupConcat(v sql.NullString) []string {
	if !v.Valid || v.String == "" {
		return []string{}
	}
	return strings.Split(v.String, "\x1f")
}

func (s *Service) listRoles(ctx context.Context, groupID string) ([]GroupRole, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT gr.id, gr.role_id, r.name, gr.environment_id, e.name, gr.stack_name
		 FROM group_roles gr
		 INNER JOIN roles r ON r.id = gr.role_id
		 LEFT JOIN environments e ON e.id = gr.environment_id
		 WHERE gr.group_id = ?
		 ORDER BY r.name ASC`,
		groupID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying group roles: %w", err)
	}
	defer closeRows(rows)

	var roles []GroupRole
	for rows.Next() {
		var gr GroupRole
		var envID, envName, stackName sql.NullString
		if err := rows.Scan(&gr.ID, &gr.RoleID, &gr.RoleName, &envID, &envName, &stackName); err != nil {
			return nil, fmt.Errorf("scanning group role: %w", err)
		}
		if envID.Valid {
			gr.EnvironmentID = &envID.String
		}
		if envName.Valid {
			gr.EnvironmentName = &envName.String
		}
		if stackName.Valid {
			gr.StackName = &stackName.String
		}
		roles = append(roles, gr)
	}
	if roles == nil {
		roles = []GroupRole{}
	}
	return roles, rows.Err()
}

// closeRows releases a result set and logs a failed Close, which a deferred call cannot return.
func closeRows(rows *sql.Rows) {
	if err := rows.Close(); err != nil {
		slog.Warn(".: closing rows", "error", err)
	}
}
