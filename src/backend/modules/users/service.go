// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package users

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

var (
	// ErrUserNotFound is returned when a user does not exist.
	ErrUserNotFound = errors.New("user not found")

	// ErrUserExists is returned when a username is already in use.
	ErrUserExists = errors.New("user already exists")

	// ErrRoleNotFound is returned when a requested default role does not exist.
	ErrRoleNotFound = errors.New("role not found")

	// ErrRoleAssignmentNotFound is returned when a role assignment does not exist.
	ErrRoleAssignmentNotFound = errors.New("role assignment does not exist")

	// ErrNoDefaultRole is returned when a local user is created
	// without an explicit role AND the auth.default_user_role_id
	// setting is empty. Per Phase 1 of the RBAC roadmap we refuse
	// the create rather than silently defaulting to a role the
	// operator never picked.
	ErrNoDefaultRole = errors.New("no default role configured for new local users")
)

// Service handles user business logic and database operations.
type Service struct {
	db   *sql.DB
	rbac *rbac.Service
	// auth is the core auth.Service so account.go can reuse the
	// password-history + SetPassword helpers without re-implementing
	// them. Wired from NewHandler via app.AuthService.
	auth *auth.Service
	// apiKeyArchiver tombstones a user's API keys before the delete
	// cascade destroys them. Injected by main.go; see bridges.go.
	apiKeyArchiver APIKeyArchiver
}

// NewService creates a new users service.
func NewService(db *sql.DB, rbac *rbac.Service, authSvc *auth.Service) *Service {
	return &Service{db: db, rbac: rbac, auth: authSvc}
}

// List returns a paginated list of users and the total count.
func (s *Service) List(ctx context.Context, page, perPage int) ([]User, int64, error) {
	var total int64
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting users: %w", err)
	}

	offset := (page - 1) * perPage
	rows, err := s.db.QueryContext(ctx,
		`SELECT u.id, u.username, u.display_name, u.email, u.role, u.is_active, u.last_login,
		        u.password_locked_until, u.created_at, u.updated_at, COALESCE(u.principal_kind, 'user'),
		        (SELECT GROUP_CONCAT(name, char(31)) FROM (SELECT DISTINCT r.name AS name FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = u.id)) AS role_names,
		        (SELECT GROUP_CONCAT(name, char(31)) FROM (SELECT DISTINCT g.name AS name FROM group_members gm JOIN groups g ON g.id = gm.group_id WHERE gm.user_id = u.id)) AS group_names
		 FROM users u ORDER BY u.username ASC LIMIT ? OFFSET ?`,
		perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("listing users: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()

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
func (s *Service) ByID(ctx context.Context, id string) (*User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, username, display_name, email, role, is_active, last_login,
		        password_locked_until, created_at, updated_at, COALESCE(principal_kind, 'user')
		 FROM users WHERE id = ?`, id,
	)

	u, err := scanUserRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetching user %s: %w", id, err)
	}

	return u, nil
}

// Exists checks whether a user with the given ID exists.
func (s *Service) Exists(ctx context.Context, id string) (bool, error) {
	var existsID string
	err := s.db.QueryRowContext(ctx, "SELECT id FROM users WHERE id = ?", id).Scan(&existsID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking user existence %s: %w", id, err)
	}
	return true, nil
}

// Create creates a local username/password user and assigns the requested RBAC role.
//
// If the caller did not specify a role, the default-role setting
// (`auth.default_user_role_id` in the `settings` table) is consulted.
// If that setting is empty the create is refused with ErrNoDefaultRole
// — we never silently default to a role the operator never picked.
// Caller-specified roles always win.
func (s *Service) Create(ctx context.Context, input CreateUserInput) (*User, error) {
	// Usernames are case-insensitive; normalise once so the duplicate
	// check and the INSERT below agree on the same value. input is
	// passed by value, so this only mutates the local copy.
	input.Username = auth.NormalizeUsername(input.Username)

	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE username = ?", input.Username).Scan(&count); err != nil {
		return nil, fmt.Errorf("checking username %s: %w", input.Username, err)
	}
	if count > 0 {
		return nil, ErrUserExists
	}

	roleID := ""
	if input.RoleID != nil && *input.RoleID != "" {
		roleID = *input.RoleID
	} else {
		// No explicit role — consult the default-role setting.
		var defaultRoleID string
		err := s.db.QueryRowContext(ctx,
			"SELECT value FROM settings WHERE key = ? AND value != ''",
			"auth.default_user_role_id",
		).Scan(&defaultRoleID)
		if err != nil && err != sql.ErrNoRows {
			return nil, fmt.Errorf("reading default role setting: %w", err)
		}
		if defaultRoleID == "" {
			return nil, ErrNoDefaultRole
		}
		roleID = defaultRoleID
	}

	roleName, err := s.roleName(ctx, roleID)
	if err != nil {
		return nil, err
	}

	// A service principal has no password and no identity provider: it never
	// signs in, and auth refuses it a session whatever the row says.
	service := input.Kind == KindService
	hash, provider, kind := "", "service", KindService
	if !service {
		provider, kind = "local", KindUser
		if hash, err = auth.HashPassword(input.Password); err != nil {
			return nil, fmt.Errorf("hashing password: %w", err)
		}
	}

	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("starting user create transaction: %w", err)
	}
	defer closeutil.Rollback(tx)

	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO users (id, username, password_hash, email, display_name, role, auth_provider, principal_kind, is_active, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, input.Username, hash, input.Email, input.DisplayName, roleName, provider, kind, isActive, now, now,
	); err != nil {
		return nil, fmt.Errorf("inserting user %s: %w", input.Username, err)
	}

	assignmentID := xid.New().String()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO user_roles (id, user_id, role_id, environment_id, stack_name, created_at, updated_at)
		 VALUES (?, ?, ?, NULL, NULL, ?, ?)`,
		assignmentID, id, roleID, now, now,
	); err != nil {
		return nil, fmt.Errorf("assigning default role to user %s: %w", input.Username, err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing user create transaction: %w", err)
	}

	if s.rbac != nil {
		s.rbac.InvalidateCache(id)
	}

	return s.ByID(ctx, id)
}

// Update applies partial updates to a user record.
func (s *Service) Update(ctx context.Context, id string, input UpdateUserInput) (*User, error) {
	exists, err := s.Exists(ctx, id)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrUserNotFound
	}

	now := time.Now().UTC().Format(time.RFC3339)

	if input.DisplayName != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE users SET display_name = ?, updated_at = ? WHERE id = ?", *input.DisplayName, now, id); err != nil {
			return nil, fmt.Errorf("updating display name: %w", err)
		}
	}
	if input.Email != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE users SET email = ?, updated_at = ? WHERE id = ?", *input.Email, now, id); err != nil {
			return nil, fmt.Errorf("updating email: %w", err)
		}
	}
	if input.Role != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE users SET role = ?, updated_at = ? WHERE id = ?", *input.Role, now, id); err != nil {
			return nil, fmt.Errorf("updating role: %w", err)
		}
	}
	if input.IsActive != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE users SET is_active = ?, updated_at = ? WHERE id = ?", *input.IsActive, now, id); err != nil {
			return nil, fmt.Errorf("updating is_active: %w", err)
		}
	}

	return s.ByID(ctx, id)
}

func (s *Service) roleName(ctx context.Context, id string) (string, error) {
	var name string
	err := s.db.QueryRowContext(ctx, "SELECT name FROM roles WHERE id = ?", id).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrRoleNotFound
	}
	if err != nil {
		return "", fmt.Errorf("querying role %s: %w", id, err)
	}
	if name == "" {
		return "viewer", nil
	}
	return name, nil
}

// Delete removes a user and their sessions. Returns ErrUserNotFound if the user does not exist.
func (s *Service) Delete(ctx context.Context, id string) error {
	archiver := s.apiKeyArchiver
	if archiver == nil {
		return fmt.Errorf("deleting user %s: API key archiver is not wired", id)
	}

	// api_keys.user_id carries ON DELETE CASCADE, so the DELETE below
	// destroys this user's API keys inside SQLite with no Go code in the
	// loop. Archive them first: the tombstone is the only record that the
	// credentials ever existed, and only this side of the delete can still
	// compute what they were effectively allowed to do.
	//
	// Both share one transaction. Archiving must come first, but a
	// tombstone must not survive a delete that then fails — a user who
	// still exists would otherwise carry records saying their credentials
	// had been destroyed.
	//
	// Migration 085 adds a BEFORE DELETE trigger as a backstop for any path
	// that misses this call. That trigger cannot resolve the owner
	// intersection — it has no access to the RBAC engine — so the snapshot
	// it writes is poorer. This is the good path; that is the safe one.
	var removed int64
	err := db.TxContext(ctx, s.db, func(tx *sql.Tx) error {
		if err := archiver.ArchiveOwnedKeysTx(ctx, tx, id); err != nil {
			return fmt.Errorf("archiving api keys for user %s: %w", id, err)
		}

		result, err := tx.ExecContext(ctx, "DELETE FROM users WHERE id = ?", id)
		if err != nil {
			return fmt.Errorf("deleting user %s: %w", id, err)
		}
		removed = db.RowsAffected(result)
		if removed == 0 {
			return nil
		}

		// Clean up sessions for the deleted user
		if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ?", id); err != nil {
			return fmt.Errorf("deleting sessions for user %s: %w", id, err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if removed == 0 {
		return ErrUserNotFound
	}

	return nil
}

// PasswordHash returns the stored password hash for a user.
// Returns ErrUserNotFound if the user does not exist.
func (s *Service) PasswordHash(ctx context.Context, id string) (string, error) {
	var hash string
	err := s.db.QueryRowContext(ctx, "SELECT password_hash FROM users WHERE id = ?", id).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrUserNotFound
	}
	if err != nil {
		return "", fmt.Errorf("fetching password hash for user %s: %w", id, err)
	}
	return hash, nil
}

// ChangePassword validates the candidate against the runtime password
// policy and stores it. This is the single write path shared by the
// self-service change endpoint and the public reset flow, so the
// policy, the reuse check, and the bookkeeping all live here rather
// than in the callers.
//
// The actual write goes through auth.Service.SetPassword, which
// records the previous hash in password_history, stamps
// password_changed_at, and clears must_reset_password plus the
// lockout counters. Writing the users row directly would skip all
// of that.
//
// Returns ErrUserIdentityProviderLocked for non-local accounts and
// *auth.PasswordValidationError ("too_short", "too_weak", "reused")
// when the policy rejects the candidate.
func (s *Service) ChangePassword(ctx context.Context, id, newPassword string) error {
	if managed, err := s.isIdentityProviderManaged(ctx, id); err != nil {
		return err
	} else if managed {
		return ErrUserIdentityProviderLocked
	}

	policy := auth.LoadPasswordPolicy(ctx, s.db)
	if err := auth.ValidatePassword(newPassword, policy); err != nil {
		return err
	}

	if reused, err := s.auth.InHistoryContext(ctx, id, newPassword, policy); err != nil {
		return fmt.Errorf("checking password history: %w", err)
	} else if reused {
		return &auth.PasswordValidationError{Field: "reused"}
	}

	newHash, err := auth.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}

	if err := s.auth.SetPasswordContext(ctx, id, newHash, policy); err != nil {
		return fmt.Errorf("updating password for user %s: %w", id, err)
	}

	return nil
}

// ListGroups returns all group memberships for a user.
func (s *Service) ListGroups(ctx context.Context, userID string) ([]UserGroup, error) {
	rows, err := s.db.QueryContext(ctx,
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
	defer func() { closeutil.Report(rows.Close(), "rows") }()

	var items []UserGroup
	for rows.Next() {
		var ug UserGroup
		if err := rows.Scan(&ug.ID, &ug.GroupID, &ug.GroupName, &ug.IsSystem, &ug.MemberCount); err != nil {
			return nil, fmt.Errorf("scanning group row: %w", err)
		}
		items = append(items, ug)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating groups: %w", err)
	}
	if items == nil {
		items = []UserGroup{}
	}

	return items, nil
}

// ListRoles returns all role assignments for a user.
func (s *Service) ListRoles(ctx context.Context, userID string) ([]UserRole, error) {
	rows, err := s.db.QueryContext(ctx,
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
	defer func() { closeutil.Report(rows.Close(), "rows") }()

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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating roles: %w", err)
	}
	if items == nil {
		items = []UserRole{}
	}

	return items, nil
}

// AssignRole creates a role assignment for a user and invalidates the RBAC cache.
func (s *Service) AssignRole(ctx context.Context, userID string, input AssignRoleInput) error {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.ExecContext(ctx,
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
// Phase 3 of the RBAC roadmap: returns the scope of the removed
// assignment so the caller can include it in the audit entry.
func (s *Service) UnassignRole(ctx context.Context, userID, assignmentID string) (scopeDesc string, err error) {
	// Read the scope before the delete so the audit log captures it.
	var roleID, envID, stackName sql.NullString
	if err = s.db.QueryRowContext(ctx,
		`SELECT role_id, environment_id, stack_name FROM user_roles WHERE id = ? AND user_id = ?`,
		assignmentID, userID,
	).Scan(&roleID, &envID, &stackName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrRoleAssignmentNotFound
		}
		return "", fmt.Errorf("loading assignment for unassign: %w", err)
	}

	result, err := s.db.ExecContext(ctx,
		"DELETE FROM user_roles WHERE id = ? AND user_id = ?", assignmentID, userID,
	)
	if err != nil {
		return "", fmt.Errorf("unassigning role from user %s: %w", userID, err)
	}

	if db.RowsAffected(result) == 0 {
		return "", ErrRoleAssignmentNotFound
	}

	scopeDesc = "global"
	if envID.Valid {
		scopeDesc = "env=" + envID.String
		if stackName.Valid {
			scopeDesc += "/stack=" + stackName.String
		}
	}

	if s.rbac != nil {
		s.rbac.InvalidateCache(userID)
	}

	return scopeDesc, nil
}

// ListOverrides returns every per-user grant/deny entry for the user.
// Phase 2 of the RBAC roadmap.
func (s *Service) ListOverrides(ctx context.Context, userID string) ([]UserPermissionOverride, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, permission_key, effect, environment_id, stack_name,
		        resource_type, resource_id, granted_by, expires_at, reason,
		        created_at, updated_at
		 FROM user_permissions
		 WHERE user_id = ?
		 ORDER BY permission_key ASC, environment_id ASC, stack_name ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing overrides for user %s: %w", userID, err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()

	var items []UserPermissionOverride
	for rows.Next() {
		var o UserPermissionOverride
		var envID, stackName, resType, resID, expiresAt, reason sql.NullString
		if err := rows.Scan(
			&o.ID, &o.PermissionKey, &o.Effect, &envID, &stackName,
			&resType, &resID, &o.GrantedBy, &expiresAt, &reason,
			&o.CreatedAt, &o.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning override row: %w", err)
		}
		if envID.Valid {
			s := envID.String
			o.EnvironmentID = &s
		}
		if stackName.Valid {
			s := stackName.String
			o.StackName = &s
		}
		if resType.Valid {
			s := resType.String
			o.ResourceType = &s
		}
		if resID.Valid {
			s := resID.String
			o.ResourceID = &s
		}
		if expiresAt.Valid {
			s := expiresAt.String
			o.ExpiresAt = &s
		}
		if reason.Valid {
			s := reason.String
			o.Reason = &s
		}
		items = append(items, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating overrides: %w", err)
	}
	if items == nil {
		items = []UserPermissionOverride{}
	}
	return items, nil
}

// SetOverrides replaces the user's full override list atomically.
// Returns the count of added/removed rows so the handler can write
// one summary audit entry. RBAC cache is invalidated.
func (s *Service) SetOverrides(ctx context.Context, userID string, grantedBy string, input SetUserOverridesInput) (added, removed int, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("starting override-set transaction: %w", err)
	}
	defer closeutil.Rollback(tx)

	type keyTuple struct {
		perm, env, stack, resType, resID, expires, reason, effect string
	}
	existingByLongKey := make(map[string]keyTuple)
	existingIDByShortKey := make(map[string]string)
	loadExisting := func() error {
		// Load existing rows for diffing.
		rows, err := tx.QueryContext(ctx,
			`SELECT id, permission_key, effect, environment_id, stack_name,
			        resource_type, resource_id, expires_at, reason
			 FROM user_permissions WHERE user_id = ?`,
			userID,
		)
		if err != nil {
			return fmt.Errorf("loading existing overrides: %w", err)
		}
		defer func() { closeutil.Report(rows.Close(), "rows") }()
		for rows.Next() {
			var id, perm, effect string
			var env, stack, rt, rid, exp, rsn sql.NullString
			if err := rows.Scan(&id, &perm, &effect, &env, &stack, &rt, &rid, &exp, &rsn); err != nil {
				return fmt.Errorf("scanning existing override: %w", err)
			}
			k := keyTuple{
				perm:    perm,
				env:     env.String,
				stack:   stack.String,
				resType: rt.String,
				resID:   rid.String,
				expires: exp.String,
				reason:  rsn.String,
				effect:  effect,
			}
			longKey := k.perm + "\x00" + k.env + "\x00" + k.stack + "\x00" + k.resType + "\x00" + k.resID + "\x00" + k.expires + "\x00" + k.reason + "\x00" + k.effect
			existingByLongKey[longKey] = k
			existingIDByShortKey[shortOverrideKey(k.perm, k.env, k.stack, k.resType, k.resID)] = id
		}
		return rows.Err()
	}
	if err := loadExisting(); err != nil {
		return 0, 0, err
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Build the set of incoming (perm|env|stack|resType|resID) -> override.
	type incomingKey = string
	incoming := make(map[incomingKey]UserPermissionOverride)
	for _, o := range input.Overrides {
		k := shortOverrideKey(
			o.PermissionKey,
			optStr(o.EnvironmentID),
			optStr(o.StackName),
			optStr(o.ResourceType),
			optStr(o.ResourceID),
		)
		if o.ID == "" {
			o.ID = xid.New().String()
		}
		o.GrantedBy = grantedBy
		o.CreatedAt = now
		o.UpdatedAt = now
		incoming[k] = o
	}

	// Delete rows that are no longer in the incoming set.
	for longKey, k := range existingByLongKey {
		shortKey := shortOverrideKey(k.perm, k.env, k.stack, k.resType, k.resID)
		if _, ok := incoming[shortKey]; !ok {
			id := existingIDByShortKey[shortKey]
			if id == "" {
				continue
			}
			if _, derr := tx.ExecContext(ctx, `DELETE FROM user_permissions WHERE id = ?`, id); derr != nil {
				return 0, 0, fmt.Errorf("deleting override %s: %w", id, derr)
			}
			removed++
		}
		_ = longKey
	}

	// Insert / upsert each incoming row. The per-row SELECT is bounded by the
	// override list in the request body, and the row it finds is updated
	// individually, so there is nothing to batch it into.
	for _, o := range input.Overrides {
		env := sql.NullString{}
		if o.EnvironmentID != nil && *o.EnvironmentID != "" {
			env = sql.NullString{String: *o.EnvironmentID, Valid: true}
		}
		stack := sql.NullString{}
		if o.StackName != nil && *o.StackName != "" {
			stack = sql.NullString{String: *o.StackName, Valid: true}
		}
		rt := sql.NullString{}
		if o.ResourceType != nil && *o.ResourceType != "" {
			rt = sql.NullString{String: *o.ResourceType, Valid: true}
		}
		rid := sql.NullString{}
		if o.ResourceID != nil && *o.ResourceID != "" {
			rid = sql.NullString{String: *o.ResourceID, Valid: true}
		}
		exp := sql.NullString{}
		if o.ExpiresAt != nil && *o.ExpiresAt != "" {
			exp = sql.NullString{String: *o.ExpiresAt, Valid: true}
		}
		rsn := sql.NullString{}
		if o.Reason != nil && *o.Reason != "" {
			rsn = sql.NullString{String: *o.Reason, Valid: true}
		}

		// Manual upsert: SELECT to check if a row with this exact
		// scope already exists; UPDATE if so, INSERT otherwise. There's
		// no UNIQUE constraint covering (user_id, permission_key,
		// environment_id, stack_name, resource_type, resource_id), and
		// adding one is non-trivial because of NULL semantics in SQL
		// unique indexes, so the upsert has to be explicit. Effect is
		// fixed at insert time; if you want to flip grant<->deny, delete
		// the row first (the diff-loop above handles that).
		var existingID string
		err := tx.QueryRowContext(ctx,
			`SELECT id FROM user_permissions
			 WHERE user_id = ?
			   AND permission_key = ?
			   AND ((environment_id IS NULL AND ? IS NULL) OR environment_id = ?)
			   AND ((stack_name     IS NULL AND ? IS NULL) OR stack_name     = ?)
			   AND ((resource_type  IS NULL AND ? IS NULL) OR resource_type  = ?)
			   AND ((resource_id    IS NULL AND ? IS NULL) OR resource_id    = ?)
			 LIMIT 1`,
			userID,
			o.PermissionKey,
			optStr(o.EnvironmentID), optStr(o.EnvironmentID),
			optStr(o.StackName), optStr(o.StackName),
			optStr(o.ResourceType), optStr(o.ResourceType),
			optStr(o.ResourceID), optStr(o.ResourceID),
		).Scan(&existingID)
		if err != nil && err != sql.ErrNoRows {
			return 0, 0, fmt.Errorf("looking up existing override %s: %w", o.PermissionKey, err)
		}

		if existingID != "" {
			if _, uerr := tx.ExecContext(ctx,
				`UPDATE user_permissions
				 SET effect = ?, granted_by = ?, expires_at = ?, reason = ?, updated_at = ?
				 WHERE id = ?`,
				o.Effect, grantedBy, exp, rsn, now, existingID,
			); uerr != nil {
				return 0, 0, fmt.Errorf("updating override %s: %w", o.PermissionKey, uerr)
			}
		} else {
			if _, ierr := tx.ExecContext(ctx,
				`INSERT INTO user_permissions
				        (id, user_id, permission_key, effect, environment_id, stack_name,
				         resource_type, resource_id, granted_by, expires_at, reason,
				         created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				o.ID, userID, o.PermissionKey, o.Effect, env, stack, rt, rid,
				grantedBy, exp, rsn, now, now,
			); ierr != nil {
				return 0, 0, fmt.Errorf("inserting override %s: %w", o.PermissionKey, ierr)
			}
		}
		added++
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("committing override set: %w", err)
	}

	if s.rbac != nil {
		s.rbac.InvalidateCache(userID)
	}

	return added, removed, nil
}

// shortOverrideKey builds the user_permissions unique-index key
// (user_id, permission_key, environment_id, stack_name, resource_type,
// resource_id). The migrations 070_rbac_v2.sql index covers this
// exact tuple.
func shortOverrideKey(perm, env, stack, resType, resID string) string {
	return perm + "|" + env + "|" + stack + "|" + resType + "|" + resID
}

// optStr safely returns the pointed-to string or empty.
func optStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// scanUser scans a User, its role names and its group names from the
// List query's GROUP_CONCAT join.
func scanUser(rows *sql.Rows) (*User, error) {
	var u User
	var displayName, email, role, lastLogin, lockedUntil sql.NullString
	var isActive sql.NullBool
	var roleNames, groupNames sql.NullString

	if err := rows.Scan(&u.ID, &u.Username, &displayName, &email, &role, &isActive,
		&lastLogin, &lockedUntil, &u.CreatedAt, &u.UpdatedAt, &u.Kind, &roleNames, &groupNames); err != nil {
		return nil, err
	}

	applyNullables(&u, displayName, email, role, isActive, lastLogin, lockedUntil)
	u.RoleNames = splitGroupConcat(roleNames)
	u.GroupNames = splitGroupConcat(groupNames)
	return &u, nil
}

// splitGroupConcat splits a `GROUP_CONCAT(x, char(31))` result on the unit
// separator, returning an empty (never nil) slice for a NULL aggregate.
func splitGroupConcat(v sql.NullString) []string {
	if !v.Valid || v.String == "" {
		return []string{}
	}
	return strings.Split(v.String, "\x1f")
}

// scanUserRow scans a User from a single sql.Row.
func scanUserRow(row *sql.Row) (*User, error) {
	var u User
	var displayName, email, role, lastLogin, lockedUntil sql.NullString
	var isActive sql.NullBool

	if err := row.Scan(&u.ID, &u.Username, &displayName, &email, &role, &isActive,
		&lastLogin, &lockedUntil, &u.CreatedAt, &u.UpdatedAt, &u.Kind); err != nil {
		return nil, err
	}

	applyNullables(&u, displayName, email, role, isActive, lastLogin, lockedUntil)
	return &u, nil
}

// applyNullables maps nullable SQL fields onto the User struct.
func applyNullables(u *User, displayName, email, role sql.NullString, isActive sql.NullBool, lastLogin, lockedUntil sql.NullString) {
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
	if lockedUntil.Valid && lockedUntil.String != "" {
		u.LockedUntil = &lockedUntil.String
	}
}
