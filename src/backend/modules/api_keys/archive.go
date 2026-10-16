// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package apikeys manages API keys: issuing, rotating, scoping, archiving and recording their usage.
package apikeys

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// DeriveStatus collapses the lifecycle flags into the single word the UI
// shows. Order matters: a deleted key is deleted whatever else it was, and a
// revoked key is revoked even if it would also have expired by now.
func DeriveStatus(key *APIKey, now time.Time) string {
	if key == nil {
		return StatusRevoked
	}
	if key.DeletedAt != nil {
		return StatusDeleted
	}
	if !key.IsActive {
		return StatusRevoked
	}
	if key.ExpiresAt != nil {
		if expires, err := time.Parse(time.RFC3339, *key.ExpiresAt); err == nil && now.After(expires) {
			return StatusExpired
		}
	}
	return StatusActive
}

// principalFor rebuilds the authorization identity of a stored key so the
// detail view can ask the RBAC engine the same questions the Bearer
// middleware asks at request time. Same inputs, same answers.
func principalFor(key *APIKey) *rbac.APIKeyPrincipal {
	principal := &rbac.APIKeyPrincipal{
		KeyID:          key.ID,
		RoleIDs:        make([]string, 0, len(key.Roles)),
		Scopes:         make([]rbac.Permission, 0, len(key.Scopes)),
		EnvironmentIDs: append([]string{}, key.EnvironmentIDs...),
	}
	for _, role := range key.Roles {
		principal.RoleIDs = append(principal.RoleIDs, role.ID)
	}
	for _, scope := range key.Scopes {
		principal.Scopes = append(principal.Scopes, rbac.Permission(scope))
	}
	return principal
}

// Detail returns the full view of one key, live or tombstoned.
//
// It looks in api_keys first and falls back to api_key_tombstones, so the
// detail route keeps working for a deleted key until retention removes it —
// which is the whole point of keeping the tombstone.
func (s *Service) Detail(ctx context.Context, id string) (*APIKeyDetail, error) {
	key, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if key == nil {
		return s.tombstoneDetail(ctx, id)
	}

	detail := &APIKeyDetail{
		APIKey:                *key,
		RoleDetails:           []APIKeyRoleDetail{},
		Environments:          []APIKeyEnvironmentDetail{},
		ConfiguredPermissions: []string{},
		EffectivePermissions:  []string{},
	}

	detail.RoleDetails, err = s.roleDetails(ctx, key.ID)
	if err != nil {
		return nil, err
	}
	detail.Environments, err = s.environmentDetails(ctx, key.ID)
	if err != nil {
		return nil, err
	}

	principal := principalFor(key)
	configured, err := s.rbac.ConfiguredPermissionsForKeyContext(ctx, principal)
	if err != nil {
		return nil, fmt.Errorf("resolving configured permissions: %w", err)
	}
	effective, err := s.rbac.EffectivePermissionsForKeyContext(ctx, key.UserID, principal)
	if err != nil {
		return nil, fmt.Errorf("resolving effective permissions: %w", err)
	}
	detail.ConfiguredPermissions = permissionStrings(configured)
	detail.EffectivePermissions = permissionStrings(effective)

	return detail, nil
}

// roleDetails loads each assigned role with the permissions it carries.
func (s *Service) roleDetails(ctx context.Context, keyID string) ([]APIKeyRoleDetail, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id, r.name, COALESCE(r.description, ''), r.permissions
		  FROM api_key_roles akr
		  INNER JOIN roles r ON r.id = akr.role_id
		 WHERE akr.api_key_id = ?
		 ORDER BY r.name
		 LIMIT 1000`, keyID)
	if err != nil {
		return nil, fmt.Errorf("querying api key role details: %w", err)
	}
	defer func() { logCloseErr(rows.Close()) }()

	details := []APIKeyRoleDetail{}
	for rows.Next() {
		var detail APIKeyRoleDetail
		var permsJSON string
		if err := rows.Scan(&detail.ID, &detail.Name, &detail.Description, &permsJSON); err != nil {
			return nil, fmt.Errorf("scanning api key role detail: %w", err)
		}
		if err := json.Unmarshal([]byte(permsJSON), &detail.Permissions); err != nil || detail.Permissions == nil {
			detail.Permissions = []string{}
		}
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating api key role details: %w", err)
	}
	return details, nil
}

// environmentDetails resolves the fence to names, so the UI never has to
// render a raw environment id at a human.
func (s *Service) environmentDetails(ctx context.Context, keyID string) ([]APIKeyEnvironmentDetail, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.id, e.name
		  FROM api_key_environments ake
		  INNER JOIN environments e ON e.id = ake.environment_id
		 WHERE ake.api_key_id = ?
		 ORDER BY e.name
		 LIMIT 1000`, keyID)
	if err != nil {
		return nil, fmt.Errorf("querying api key environment details: %w", err)
	}
	defer func() { logCloseErr(rows.Close()) }()

	details := []APIKeyEnvironmentDetail{}
	for rows.Next() {
		var detail APIKeyEnvironmentDetail
		if err := rows.Scan(&detail.ID, &detail.Name); err != nil {
			return nil, fmt.Errorf("scanning api key environment detail: %w", err)
		}
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating api key environment details: %w", err)
	}
	return details, nil
}

const selectTombstone = `SELECT id, COALESCE(user_id, ''), username, name, key_prefix,
        roles_snapshot, explicit_scopes_snapshot, configured_perms_snapshot,
        effective_perms_snapshot, environments_snapshot,
        expires_at, last_used_at, key_created_at, key_updated_at, deleted_at
   FROM api_key_tombstones`

// tombstoneDetail reads a deleted key's frozen record.
//
// Every list here comes from the snapshot taken at deletion, never from a
// join against live roles. A role renamed or removed since then must not
// change what this page says the key could do, and a deleted role must not
// make its grants disappear from the record.
func (s *Service) tombstoneDetail(ctx context.Context, id string) (*APIKeyDetail, error) {
	row := s.db.QueryRowContext(ctx, selectTombstone+` WHERE id = ?`, id)

	var (
		detail                                                        APIKeyDetail
		rolesJSON, scopesJSON, configuredJSON, effectiveJSON, envJSON string
		expiresAt, lastUsedAt                                         sql.NullString
		deletedAt                                                     string
	)

	err := row.Scan(&detail.ID, &detail.UserID, &detail.Username, &detail.Name, &detail.KeyPrefix,
		&rolesJSON, &scopesJSON, &configuredJSON, &effectiveJSON, &envJSON,
		&expiresAt, &lastUsedAt, &detail.CreatedAt, &detail.UpdatedAt, &deletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying api key tombstone: %w", err)
	}

	detail.IsActive = false
	detail.DeletedAt = &deletedAt
	detail.Status = StatusDeleted
	detail.EffectiveSnapshot = true
	if expiresAt.Valid {
		detail.ExpiresAt = &expiresAt.String
	}
	if lastUsedAt.Valid {
		detail.LastUsedAt = &lastUsedAt.String
	}

	detail.RoleDetails = decodeJSON[[]APIKeyRoleDetail](rolesJSON, []APIKeyRoleDetail{})
	detail.Environments = decodeJSON[[]APIKeyEnvironmentDetail](envJSON, []APIKeyEnvironmentDetail{})
	detail.Scopes = decodeJSON[[]string](scopesJSON, []string{})
	detail.ConfiguredPermissions = decodeJSON[[]string](configuredJSON, []string{})
	detail.EffectivePermissions = decodeJSON[[]string](effectiveJSON, []string{})

	// Roles on the embedded APIKey mirror the snapshot so the shared table
	// renderer works for a tombstone without a special case.
	detail.Roles = make([]APIKeyRole, 0, len(detail.RoleDetails))
	for _, role := range detail.RoleDetails {
		detail.Roles = append(detail.Roles, APIKeyRole{ID: role.ID, Name: role.Name})
	}
	detail.EnvironmentIDs = make([]string, 0, len(detail.Environments))
	for _, env := range detail.Environments {
		detail.EnvironmentIDs = append(detail.EnvironmentIDs, env.ID)
	}

	return &detail, nil
}

// ListTombstones returns retained deleted keys as list rows, so the table can
// show them beside live ones. userID empty means every key.
func (s *Service) ListTombstones(ctx context.Context, userID string) ([]APIKey, error) {
	query := selectTombstone + ` ORDER BY deleted_at DESC LIMIT 1000`
	args := []any{}
	if userID != "" {
		query = selectTombstone + ` WHERE user_id = ? ORDER BY deleted_at DESC LIMIT 1000`
		args = append(args, userID)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying api key tombstones: %w", err)
	}
	defer func() { logCloseErr(rows.Close()) }()

	items := []APIKey{}
	for rows.Next() {
		var (
			key                                                           APIKey
			rolesJSON, scopesJSON, configuredJSON, effectiveJSON, envJSON string
			expiresAt, lastUsedAt                                         sql.NullString
			deletedAt                                                     string
		)
		if err := rows.Scan(&key.ID, &key.UserID, &key.Username, &key.Name, &key.KeyPrefix,
			&rolesJSON, &scopesJSON, &configuredJSON, &effectiveJSON, &envJSON,
			&expiresAt, &lastUsedAt, &key.CreatedAt, &key.UpdatedAt, &deletedAt); err != nil {
			return nil, fmt.Errorf("scanning api key tombstone: %w", err)
		}

		key.IsActive = false
		key.Status = StatusDeleted
		key.DeletedAt = &deletedAt
		if expiresAt.Valid {
			key.ExpiresAt = &expiresAt.String
		}
		if lastUsedAt.Valid {
			key.LastUsedAt = &lastUsedAt.String
		}
		key.Scopes = decodeJSON[[]string](scopesJSON, []string{})
		key.EnvironmentIDs = []string{}
		key.EnvironmentNames = []string{}
		for _, env := range decodeJSON[[]APIKeyEnvironmentDetail](envJSON, []APIKeyEnvironmentDetail{}) {
			key.EnvironmentIDs = append(key.EnvironmentIDs, env.ID)
			if env.Name != "" {
				key.EnvironmentNames = append(key.EnvironmentNames, env.Name)
			}
		}
		key.Roles = []APIKeyRole{}
		for _, role := range decodeJSON[[]APIKeyRoleDetail](rolesJSON, []APIKeyRoleDetail{}) {
			key.Roles = append(key.Roles, APIKeyRole{ID: role.ID, Name: role.Name})
		}
		items = append(items, key)
	}
	return items, rows.Err()
}

// DeleteAndArchive destroys a revoked key's credential and keeps a read-only
// record of what it was allowed to do.
//
// The snapshot and the delete share one transaction: a tombstone without the
// deletion would show a key that still works, and a deletion without the
// tombstone is the data loss this whole feature exists to prevent. Usage
// events are deliberately untouched — they carry the same id and stay
// queryable through it.
func (s *Service) DeleteAndArchive(ctx context.Context, id string) error {
	detail, err := s.Detail(ctx, id)
	if err != nil {
		return err
	}
	if detail == nil {
		return sql.ErrNoRows
	}
	if detail.DeletedAt != nil {
		// Already a tombstone; nothing left to destroy.
		return sql.ErrNoRows
	}
	if detail.IsActive {
		// Revoke first. Mirrors the previous Delete contract.
		return sql.ErrNoRows
	}

	now := time.Now().UTC().Format(time.RFC3339)
	err = inTx(ctx, s.db, func(tx *sql.Tx) error {
		if err := insertTombstone(ctx, tx, detail, now); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM api_keys WHERE id = ? AND is_active = 0`, id)
		if err != nil {
			return fmt.Errorf("deleting archived api key: %w", err)
		}
		if db.RowsAffected(result) == 0 {
			return sql.ErrNoRows
		}
		return nil
	})
	if err != nil {
		return err
	}

	s.rbac.InvalidateAPIKeyCache(id)
	return nil
}

// ArchiveOwnedKeys writes a tombstone for every key belonging to a user who
// is about to be deleted.
//
// api_keys.user_id cascades, so the rows vanish inside SQLite the moment the
// user row goes. Calling this first is what gives those tombstones a real
// effective-permission snapshot; the BEFORE DELETE trigger added in migration
// 085 is the fallback for any path that forgets, and it cannot compute the
// owner intersection because that needs the RBAC engine.
func (s *Service) ArchiveOwnedKeys(ctx context.Context, userID string) error {
	return inTx(ctx, s.db, func(tx *sql.Tx) error {
		return s.ArchiveOwnedKeysTx(ctx, tx, userID)
	})
}

// ArchiveOwnedKeysTx is ArchiveOwnedKeys inside a caller's transaction.
//
// The user-deletion path needs this: archiving has to happen before the row
// goes, because the cascade destroys the keys the moment it does, but the
// tombstones must not outlive a delete that then fails. Sharing one
// transaction makes both true at once — a user who still exists never has
// records saying their credentials were destroyed.
func (s *Service) ArchiveOwnedKeysTx(ctx context.Context, tx *sql.Tx, userID string) error {
	if userID == "" {
		return nil
	}

	ids, err := s.ownedKeyIDs(ctx, userID)
	if err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	for _, id := range ids {
		detail, err := s.Detail(ctx, id)
		if err != nil {
			return err
		}
		if detail == nil || detail.DeletedAt != nil {
			continue
		}
		if err := insertTombstone(ctx, tx, detail, now); err != nil {
			return err
		}
		s.rbac.InvalidateAPIKeyCache(id)
	}
	return nil
}

// insertTombstone writes the frozen record. INSERT OR IGNORE so it composes
// with the database trigger: whichever runs first wins, and the second is a
// no-op rather than a constraint failure.
func insertTombstone(ctx context.Context, tx *sql.Tx, detail *APIKeyDetail, now string) error {
	roles, err := json.Marshal(detail.RoleDetails)
	if err != nil {
		return fmt.Errorf("encoding roles snapshot: %w", err)
	}
	scopes, err := json.Marshal(nonNil(detail.Scopes))
	if err != nil {
		return fmt.Errorf("encoding scopes snapshot: %w", err)
	}
	configured, err := json.Marshal(nonNil(detail.ConfiguredPermissions))
	if err != nil {
		return fmt.Errorf("encoding configured snapshot: %w", err)
	}
	effective, err := json.Marshal(nonNil(detail.EffectivePermissions))
	if err != nil {
		return fmt.Errorf("encoding effective snapshot: %w", err)
	}
	environments, err := json.Marshal(detail.Environments)
	if err != nil {
		return fmt.Errorf("encoding environments snapshot: %w", err)
	}

	var userID any
	if detail.UserID != "" {
		userID = detail.UserID
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO api_key_tombstones (
			id, user_id, username, name, key_prefix,
			roles_snapshot, explicit_scopes_snapshot, configured_perms_snapshot,
			effective_perms_snapshot, environments_snapshot,
			expires_at, last_used_at, key_created_at, key_updated_at,
			deleted_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		detail.ID, userID, detail.Username, detail.Name, detail.KeyPrefix,
		string(roles), string(scopes), string(configured), string(effective), string(environments),
		detail.ExpiresAt, detail.LastUsedAt, detail.CreatedAt, detail.UpdatedAt,
		now, now, now,
	); err != nil {
		return fmt.Errorf("inserting api key tombstone: %w", err)
	}
	return nil
}

// permissionStrings converts and sorts, so two calls with the same grants
// produce the same JSON and a snapshot diff means something changed.
func permissionStrings(perms []rbac.Permission) []string {
	out := make([]string, 0, len(perms))
	for _, perm := range perms {
		out = append(out, string(perm))
	}
	sort.Strings(out)
	return out
}

// decodeJSON parses a snapshot column, falling back to the zero value. A
// tombstone with an unreadable column still renders; it just shows nothing
// for that section rather than failing the whole page.
func decodeJSON[T any](raw string, fallback T) T {
	if raw == "" {
		return fallback
	}
	var out T
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return fallback
	}
	return out
}

// ownedKeyIDs lists a user's key ids and closes the cursor before the caller issues follow-up reads on the same pool.
func (s *Service) ownedKeyIDs(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM api_keys WHERE user_id = ? LIMIT 1000`, userID)
	if err != nil {
		return nil, fmt.Errorf("listing api keys for owner: %w", err)
	}
	defer func() { logCloseErr(rows.Close()) }()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning api key id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating api keys for owner: %w", err)
	}
	return ids, nil
}

// inTx runs fn in a transaction bound to ctx and rolls it back on error, so a cancelled request cannot leave a half-applied write.
func inTx(ctx context.Context, conn *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer db.RollbackTx(tx)
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// logCloseErr reports a failed Close of a sql resource through slog, because a deferred close has no caller to return the error to.
func logCloseErr(err error) {
	if err != nil {
		slog.Warn("closing sql resource failed", "error", err)
	}
}
