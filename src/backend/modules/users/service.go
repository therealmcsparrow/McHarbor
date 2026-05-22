// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package users

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

var (
	// ErrUserNotFound is returned when a user does not exist.
	ErrUserNotFound = errors.New("user not found")

	// ErrRoleAssignmentNotFound is returned when a role assignment does not exist.
	ErrRoleAssignmentNotFound = errors.New("role assignment not found")
)

// Service handles user business logic and database operations.
type Service struct {
	db   *sql.DB
	rbac *rbac.Service
}

// NewService creates a new users service.
func NewService(db *sql.DB, rbac *rbac.Service) *Service {
	return &Service{db: db, rbac: rbac}
}

// List returns a paginated list of users and the total count.
func (s *Service) List(page, perPage int) ([]User, int64, error) {
	var total int64
	if err := s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting users: %w", err)
	}

	offset := (page - 1) * perPage
	rows, err := s.db.Query(
		`SELECT id, username, display_name, email, role, is_active, last_login, created_at, updated_at
		 FROM users ORDER BY username ASC LIMIT ? OFFSET ?`,
		perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("listing users: %w", err)
	}
	defer rows.Close()

	var items []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning user row: %w", err)
		}
		items = append(items, *u)
	}

	if items == nil {
		items = []User{}
	}

	return items, total, nil
}

// ByID returns a single user by ID, or nil if not found.
func (s *Service) ByID(id string) (*User, error) {
	row := s.db.QueryRow(
		`SELECT id, username, display_name, email, role, is_active, last_login, created_at, updated_at
		 FROM users WHERE id = ?`, id,
	)

	u, err := scanUserRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetching user %s: %w", id, err)
	}

	return u, nil
}

// Exists checks whether a user with the given ID exists.
func (s *Service) Exists(id string) (bool, error) {
	var existsID string
	err := s.db.QueryRow("SELECT id FROM users WHERE id = ?", id).Scan(&existsID)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking user existence %s: %w", id, err)
	}
	return true, nil
}

// Update applies partial updates to a user record.
func (s *Service) Update(id string, input UpdateUserInput) (*User, error) {
	exists, err := s.Exists(id)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrUserNotFound
	}

	now := time.Now().UTC().Format(time.RFC3339)

	if input.DisplayName != nil {
		if _, err := s.db.Exec("UPDATE users SET display_name = ?, updated_at = ? WHERE id = ?", *input.DisplayName, now, id); err != nil {
			return nil, fmt.Errorf("updating display name: %w", err)
		}
	}
	if input.Email != nil {
		if _, err := s.db.Exec("UPDATE users SET email = ?, updated_at = ? WHERE id = ?", *input.Email, now, id); err != nil {
			return nil, fmt.Errorf("updating email: %w", err)
		}
	}
	if input.Role != nil {
		if _, err := s.db.Exec("UPDATE users SET role = ?, updated_at = ? WHERE id = ?", *input.Role, now, id); err != nil {
			return nil, fmt.Errorf("updating role: %w", err)
		}
	}
	if input.IsActive != nil {
		if _, err := s.db.Exec("UPDATE users SET is_active = ?, updated_at = ? WHERE id = ?", *input.IsActive, now, id); err != nil {
			return nil, fmt.Errorf("updating is_active: %w", err)
		}
	}

	return s.ByID(id)
}

// Delete removes a user and their sessions. Returns ErrUserNotFound if the user does not exist.
func (s *Service) Delete(id string) error {
	result, err := s.db.Exec("DELETE FROM users WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting user %s: %w", id, err)
	}

	if db.RowsAffected(result) == 0 {
		return ErrUserNotFound
	}

	// Clean up sessions for the deleted user
	if _, err := s.db.Exec("DELETE FROM sessions WHERE user_id = ?", id); err != nil {
		return fmt.Errorf("deleting sessions for user %s: %w", id, err)
	}

	return nil
}

// PasswordHash returns the stored password hash for a user.
// Returns ErrUserNotFound if the user does not exist.
func (s *Service) PasswordHash(id string) (string, error) {
	var hash string
	err := s.db.QueryRow("SELECT password_hash FROM users WHERE id = ?", id).Scan(&hash)
	if err == sql.ErrNoRows {
		return "", ErrUserNotFound
	}
	if err != nil {
		return "", fmt.Errorf("fetching password hash for user %s: %w", id, err)
	}
	return hash, nil
}

// ChangePassword hashes the new password and stores it.
func (s *Service) ChangePassword(id, newPassword string) error {
	newHash, err := auth.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec("UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?", newHash, now, id); err != nil {
		return fmt.Errorf("updating password for user %s: %w", id, err)
	}

	return nil
}

// ListGroups returns all group memberships for a user.
func (s *Service) ListGroups(userID string) ([]UserGroup, error) {
	rows, err := s.db.Query(
		`SELECT gm.id, g.id, g.name, g.is_system,
		        (SELECT COUNT(*) FROM group_members WHERE group_id = g.id) AS member_count
		 FROM group_members gm
		 INNER JOIN groups g ON g.id = gm.group_id
		 WHERE gm.user_id = ?
		 ORDER BY g.name ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing groups for user %s: %w", userID, err)
	}
	defer rows.Close()

	var items []UserGroup
	for rows.Next() {
		var ug UserGroup
		if err := rows.Scan(&ug.ID, &ug.GroupID, &ug.GroupName, &ug.IsSystem, &ug.MemberCount); err != nil {
			return nil, fmt.Errorf("scanning group row: %w", err)
		}
		items = append(items, ug)
	}
	if items == nil {
		items = []UserGroup{}
	}

	return items, nil
}

// ListRoles returns all role assignments for a user.
func (s *Service) ListRoles(userID string) ([]UserRole, error) {
	rows, err := s.db.Query(
		`SELECT ur.id, ur.role_id, ro.name, ur.environment_id, e.name, ur.stack_name
		 FROM user_roles ur
		 INNER JOIN roles ro ON ro.id = ur.role_id
		 LEFT JOIN environments e ON e.id = ur.environment_id
		 WHERE ur.user_id = ?
		 ORDER BY ro.name ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing roles for user %s: %w", userID, err)
	}
	defer rows.Close()

	var items []UserRole
	for rows.Next() {
		var ur UserRole
		var envID, envName, stackName sql.NullString
		if err := rows.Scan(&ur.ID, &ur.RoleID, &ur.RoleName, &envID, &envName, &stackName); err != nil {
			return nil, fmt.Errorf("scanning role row: %w", err)
		}
		if envID.Valid {
			ur.EnvironmentID = &envID.String
		}
		if envName.Valid {
			ur.EnvironmentName = &envName.String
		}
		if stackName.Valid {
			ur.StackName = &stackName.String
		}
		items = append(items, ur)
	}
	if items == nil {
		items = []UserRole{}
	}

	return items, nil
}

// AssignRole creates a role assignment for a user and invalidates the RBAC cache.
func (s *Service) AssignRole(userID string, input AssignRoleInput) error {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.Exec(
		`INSERT INTO user_roles (id, user_id, role_id, environment_id, stack_name, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, userID, input.RoleID, input.EnvironmentID, input.StackName, now, now,
	)
	if err != nil {
		return fmt.Errorf("assigning role to user %s: %w", userID, err)
	}

	if s.rbac != nil {
		s.rbac.InvalidateCache(userID)
	}

	return nil
}

// UnassignRole removes a role assignment and invalidates the RBAC cache.
// Returns ErrRoleAssignmentNotFound if no matching assignment exists.
func (s *Service) UnassignRole(userID, assignmentID string) error {
	result, err := s.db.Exec(
		"DELETE FROM user_roles WHERE id = ? AND user_id = ?", assignmentID, userID,
	)
	if err != nil {
		return fmt.Errorf("unassigning role from user %s: %w", userID, err)
	}

	if db.RowsAffected(result) == 0 {
		return ErrRoleAssignmentNotFound
	}

	if s.rbac != nil {
		s.rbac.InvalidateCache(userID)
	}

	return nil
}

// scanUser scans a User from a sql.Rows iterator.
func scanUser(rows *sql.Rows) (*User, error) {
	var u User
	var displayName, email, role, lastLogin sql.NullString
	var isActive sql.NullBool

	if err := rows.Scan(&u.ID, &u.Username, &displayName, &email, &role, &isActive,
		&lastLogin, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}

	applyNullables(&u, displayName, email, role, isActive, lastLogin)
	return &u, nil
}

// scanUserRow scans a User from a single sql.Row.
func scanUserRow(row *sql.Row) (*User, error) {
	var u User
	var displayName, email, role, lastLogin sql.NullString
	var isActive sql.NullBool

	if err := row.Scan(&u.ID, &u.Username, &displayName, &email, &role, &isActive,
		&lastLogin, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}

	applyNullables(&u, displayName, email, role, isActive, lastLogin)
	return &u, nil
}

// applyNullables maps nullable SQL fields onto the User struct.
func applyNullables(u *User, displayName, email, role sql.NullString, isActive sql.NullBool, lastLogin sql.NullString) {
	if displayName.Valid {
		u.DisplayName = &displayName.String
	}
	if email.Valid {
		u.Email = &email.String
	}
	u.Role = role.String
	if u.Role == "" {
		u.Role = "admin"
	}
	u.IsActive = !isActive.Valid || isActive.Bool // default true
	if lastLogin.Valid {
		u.LastLogin = &lastLogin.String
	}
}
