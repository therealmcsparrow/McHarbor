// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package apikeys

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

const keyPrefix = "mch_"

var randRead = rand.Read

// Sentinel errors the handler maps to i18n codes.
var (
	ErrGrantsRequired     = errors.New("api key needs at least one role or scope")
	ErrGrantInvalid       = errors.New("api key grant references an unknown role, permission, or environment")
	ErrGrantExceedsCaller = errors.New("api key grant exceeds the caller's own permissions")
)

// Service handles API key operations.
type Service struct {
	db   *sql.DB
	rbac *rbac.Service
}

// NewService creates a new API keys service.
func NewService(database *sql.DB, rbacSvc *rbac.Service) *Service {
	return &Service{db: database, rbac: rbacSvc}
}

const selectKey = `SELECT ak.id, ak.user_id, u.username, ak.name, ak.key_prefix, ak.scopes,
        ak.expires_at, ak.last_used_at, ak.is_active, ak.created_at, ak.updated_at
   FROM api_keys ak
  INNER JOIN users u ON u.id = ak.user_id`

// List returns API keys. If userID is empty, returns all keys.
func (s *Service) List(ctx context.Context, userID string) ([]APIKey, error) {
	var rows *sql.Rows
	var err error

	if userID == "" {
		rows, err = s.db.QueryContext(ctx, selectKey+` ORDER BY ak.created_at DESC LIMIT 1000`)
	} else {
		rows, err = s.db.QueryContext(ctx, selectKey+` WHERE ak.user_id = ? ORDER BY ak.created_at DESC LIMIT 1000`, userID)
	}
	if err != nil {
		return nil, fmt.Errorf("querying api keys: %w", err)
	}
	defer func() { logCloseErr(rows.Close()) }()

	items := []APIKey{}
	for rows.Next() {
		k, err := scanKey(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning api key: %w", err)
		}
		items = append(items, *k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating api keys: %w", err)
	}
	if err := s.attachGrants(ctx, items); err != nil {
		return nil, err
	}

	// Retained tombstones sit in the same table as live keys. A deleted key
	// that still has history is something an operator needs to find, and
	// hiding it until retention expires would make the history unreachable
	// through the only surface that links to it.
	tombstones, err := s.ListTombstones(ctx, userID)
	if err != nil {
		return nil, err
	}
	items = append(items, tombstones...)
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].CreatedAt > items[j].CreatedAt
	})

	return items, nil
}

// Get returns a single API key, or nil when it does not exist.
func (s *Service) Get(ctx context.Context, id string) (*APIKey, error) {
	row := s.db.QueryRowContext(ctx, selectKey+` WHERE ak.id = ?`, id)

	k, err := scanKey(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying api key: %w", err)
	}
	items := []APIKey{*k}
	if err := s.attachGrants(ctx, items); err != nil {
		return nil, err
	}
	return &items[0], nil
}

// Create generates a new API key and returns it with the plaintext key.
// The caller must have run ValidateGrants first.
func (s *Service) Create(ctx context.Context, userID string, input *CreateAPIKeyInput) (*CreateAPIKeyResult, error) {
	plainKey, keyHash, displayPrefix, err := newKeyMaterial()
	if err != nil {
		return nil, err
	}

	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	scopesJSON, err := json.Marshal(nonNil(input.Scopes))
	if err != nil {
		return nil, fmt.Errorf("encoding scopes: %w", err)
	}

	err = inTx(ctx, s.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO api_keys (id, user_id, name, key_hash, key_prefix, scopes, expires_at, is_active, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
			id, userID, input.Name, keyHash, displayPrefix, string(scopesJSON), input.ExpiresAt, now, now,
		); err != nil {
			return fmt.Errorf("inserting api key: %w", err)
		}
		if err := replaceRoles(ctx, tx, id, input.RoleIDs, now); err != nil {
			return err
		}
		return replaceEnvironments(ctx, tx, id, input.EnvironmentIDs, now)
	})
	if err != nil {
		return nil, err
	}

	key, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if key == nil {
		return nil, fmt.Errorf("created api key not found")
	}

	return &CreateAPIKeyResult{APIKey: *key, Key: plainKey}, nil
}

// Update changes name and grants. Nil slices leave a grant list untouched.
// The caller must have run ValidateGrants on the resulting lists first.
func (s *Service) Update(ctx context.Context, id string, input *UpdateAPIKeyInput) (*APIKey, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	err := inTx(ctx, s.db, func(tx *sql.Tx) error {
		if input.Name != nil {
			if _, err := tx.ExecContext(ctx, `UPDATE api_keys SET name = ?, updated_at = ? WHERE id = ?`, *input.Name, now, id); err != nil {
				return fmt.Errorf("updating api key name: %w", err)
			}
		}
		if input.Scopes != nil {
			scopesJSON, err := json.Marshal(input.Scopes)
			if err != nil {
				return fmt.Errorf("encoding scopes: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `UPDATE api_keys SET scopes = ?, updated_at = ? WHERE id = ?`, string(scopesJSON), now, id); err != nil {
				return fmt.Errorf("updating api key scopes: %w", err)
			}
		}
		if input.RoleIDs != nil {
			if err := replaceRoles(ctx, tx, id, input.RoleIDs, now); err != nil {
				return err
			}
		}
		if input.EnvironmentIDs != nil {
			if err := replaceEnvironments(ctx, tx, id, input.EnvironmentIDs, now); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE api_keys SET updated_at = ? WHERE id = ?`, now, id); err != nil {
			return fmt.Errorf("touching api key: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.rbac.InvalidateAPIKeyCache(id)
	return s.Get(ctx, id)
}

// Revoke deactivates an API key.
func (s *Service) Revoke(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx, "UPDATE api_keys SET is_active = 0, updated_at = ? WHERE id = ?", now, id)
	if err != nil {
		return fmt.Errorf("revoking api key: %w", err)
	}
	if db.RowsAffected(result) == 0 {
		return sql.ErrNoRows
	}
	s.rbac.InvalidateAPIKeyCache(id)
	return nil
}

// Delete permanently removes a key that has already been revoked. An
// active key is refused (sql.ErrNoRows) so the caller revokes first; the
// grant rows go with the key through the foreign-key cascade.
func (s *Service) Delete(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM api_keys WHERE id = ? AND is_active = 0", id)
	if err != nil {
		return fmt.Errorf("deleting api key: %w", err)
	}
	if db.RowsAffected(result) == 0 {
		return sql.ErrNoRows
	}
	s.rbac.InvalidateAPIKeyCache(id)
	return nil
}

// CanViewAll reports whether the caller may see and manage other users'
// keys: api_keys.view (or the wildcard) through the caller's own
// principal, so a key without that permission cannot enumerate keys.
func (s *Service) CanViewAll(ctx context.Context, userID string) (bool, error) {
	if userID == "system" {
		return true, nil
	}
	return s.rbac.HasPermission(ctx, userID, "", rbac.PermAPIKeysView)
}

// GrantOptions lists what the caller may put on a key.
func (s *Service) GrantOptions(ctx context.Context, userID string) (*GrantOptions, error) {
	callerPerms, err := s.rbac.EffectivePermissions(ctx, userID, "")
	if err != nil {
		return nil, fmt.Errorf("resolving caller permissions: %w", err)
	}
	wildcard := userID == "system" || hasPerm(callerPerms, rbac.PermWildcard)
	held := make(map[string]bool, len(callerPerms))
	for _, p := range callerPerms {
		held[string(p)] = true
	}

	out := &GrantOptions{Roles: []GrantOption{}, Environments: []EnvironmentOption{}, Permissions: []string{}, Wildcard: wildcard}

	rows, err := s.db.QueryContext(ctx, `SELECT id, name, description, permissions FROM roles ORDER BY name LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("querying roles: %w", err)
	}
	defer func() { logCloseErr(rows.Close()) }()
	for rows.Next() {
		var opt GrantOption
		var description sql.NullString
		var permsJSON string
		if err := rows.Scan(&opt.ID, &opt.Name, &description, &permsJSON); err != nil {
			return nil, fmt.Errorf("scanning role: %w", err)
		}
		opt.Description = description.String
		if err := json.Unmarshal([]byte(permsJSON), &opt.Permissions); err != nil || opt.Permissions == nil {
			opt.Permissions = []string{}
		}
		opt.Grantable = wildcard || allHeld(opt.Permissions, held)
		out.Roles = append(out.Roles, opt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating roles: %w", err)
	}

	envRows, err := s.db.QueryContext(ctx, `SELECT id, name FROM environments ORDER BY name LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("querying environments: %w", err)
	}
	defer func() { logCloseErr(envRows.Close()) }()
	for envRows.Next() {
		var env EnvironmentOption
		if err := envRows.Scan(&env.ID, &env.Name); err != nil {
			return nil, fmt.Errorf("scanning environment: %w", err)
		}
		out.Environments = append(out.Environments, env)
	}
	if err := envRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating environments: %w", err)
	}

	for _, entry := range rbac.CatalogSeed() {
		key := string(entry.Key)
		if wildcard || held[key] {
			out.Permissions = append(out.Permissions, key)
		}
	}
	return out, nil
}

// ValidateGrants enforces the grant rules for one create or update: at
// least one role or scope (when requireSome), every id must exist, every
// scope must be a known key or "*", and nothing outside the caller's own
// set unless the caller holds the wildcard.
func (s *Service) ValidateGrants(ctx context.Context, callerID string, roleIDs, scopes, envIDs []string, requireSome bool) error {
	if requireSome && len(roleIDs) == 0 && len(scopes) == 0 {
		return ErrGrantsRequired
	}

	callerPerms, err := s.rbac.EffectivePermissions(ctx, callerID, "")
	if err != nil {
		return fmt.Errorf("resolving caller permissions: %w", err)
	}
	wildcard := callerID == "system" || hasPerm(callerPerms, rbac.PermWildcard)
	held := make(map[string]bool, len(callerPerms))
	for _, p := range callerPerms {
		held[string(p)] = true
	}

	known := make(map[string]bool)
	for _, entry := range rbac.CatalogSeed() {
		known[string(entry.Key)] = true
	}
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "*" {
			if !wildcard {
				return ErrGrantExceedsCaller
			}
			continue
		}
		if !known[scope] && !held[scope] {
			return ErrGrantInvalid
		}
		if !wildcard && !held[scope] {
			return ErrGrantExceedsCaller
		}
	}

	// Both checks below read the whole id set in one statement. A row that
	// comes back short means an id did not resolve, which is the same
	// ErrGrantInvalid the per-id loop used to return.
	if len(roleIDs) > 0 {
		if err := s.checkGrantedRoles(ctx, roleIDs, held, wildcard); err != nil {
			return err
		}
	}

	if len(envIDs) > 0 {
		var found int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM environments WHERE id IN (`+placeholders(len(envIDs))+`)`,
			anyArgs(envIDs)...).Scan(&found); err != nil {
			return fmt.Errorf("querying granted environments: %w", err)
		}
		if found != len(envIDs) {
			return ErrGrantInvalid
		}
	}
	return nil
}

// placeholders builds "?, ?, ?" for an IN clause of n arguments.
func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// anyArgs widens a string slice for the variadic query argument list.
func anyArgs(values []string) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = v
	}
	return out
}

// attachGrants fills Roles and EnvironmentIDs for every key in items with
// two queries, regardless of how many keys there are.
func (s *Service) attachGrants(ctx context.Context, items []APIKey) error {
	if len(items) == 0 {
		return nil
	}
	index := make(map[string]int, len(items))
	args := make([]any, 0, len(items))
	for i := range items {
		items[i].Roles = []APIKeyRole{}
		items[i].EnvironmentIDs = []string{}
		items[i].EnvironmentNames = []string{}
		if items[i].Scopes == nil {
			items[i].Scopes = []string{}
		}
		index[items[i].ID] = i
		args = append(args, items[i].ID)
	}
	inList := strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")

	roleQuery := rolesForKeysSelect + inList + rolesForKeysTail // SQL identifier: inList holds only "?" markers
	roleRows, err := s.db.QueryContext(ctx, roleQuery, args...)
	if err != nil {
		return fmt.Errorf("querying api key roles: %w", err)
	}
	defer func() { logCloseErr(roleRows.Close()) }()
	for roleRows.Next() {
		var keyID string
		var role APIKeyRole
		if err := roleRows.Scan(&keyID, &role.ID, &role.Name); err != nil {
			return fmt.Errorf("scanning api key role: %w", err)
		}
		if i, ok := index[keyID]; ok {
			items[i].Roles = append(items[i].Roles, role)
		}
	}
	if err := roleRows.Err(); err != nil {
		return fmt.Errorf("iterating api key roles: %w", err)
	}

	envQuery := envsForKeysSelect + inList + envsForKeysTail // SQL identifier: inList holds only "?" markers
	envRows, err := s.db.QueryContext(ctx, envQuery, args...)
	if err != nil {
		return fmt.Errorf("querying api key environments: %w", err)
	}
	defer func() { logCloseErr(envRows.Close()) }()
	for envRows.Next() {
		var keyID, envID string
		var envName sql.NullString
		if err := envRows.Scan(&keyID, &envID, &envName); err != nil {
			return fmt.Errorf("scanning api key environment: %w", err)
		}
		if i, ok := index[keyID]; ok {
			items[i].EnvironmentIDs = append(items[i].EnvironmentIDs, envID)
			if envName.Valid {
				items[i].EnvironmentNames = append(items[i].EnvironmentNames, envName.String)
			}
		}
	}
	if err := envRows.Err(); err != nil {
		return fmt.Errorf("iterating api key environments: %w", err)
	}
	return nil
}

func replaceRoles(ctx context.Context, tx *sql.Tx, keyID string, roleIDs []string, now string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM api_key_roles WHERE api_key_id = ?`, keyID); err != nil {
		return fmt.Errorf("clearing api key roles: %w", err)
	}
	for _, roleID := range dedupe(roleIDs) {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO api_key_roles (id, api_key_id, role_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			xid.New().String(), keyID, roleID, now, now); err != nil {
			return fmt.Errorf("inserting api key role: %w", err)
		}
	}
	return nil
}

func replaceEnvironments(ctx context.Context, tx *sql.Tx, keyID string, envIDs []string, now string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM api_key_environments WHERE api_key_id = ?`, keyID); err != nil {
		return fmt.Errorf("clearing api key environments: %w", err)
	}
	for _, envID := range dedupe(envIDs) {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO api_key_environments (id, api_key_id, environment_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			xid.New().String(), keyID, envID, now, now); err != nil {
			return fmt.Errorf("inserting api key environment: %w", err)
		}
	}
	return nil
}

func dedupe(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func nonNil(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func hasPerm(perms []rbac.Permission, perm rbac.Permission) bool {
	for _, p := range perms {
		if p == perm {
			return true
		}
	}
	return false
}

func allHeld(perms []string, held map[string]bool) bool {
	for _, p := range perms {
		if p == "*" || !held[p] {
			return false
		}
	}
	return true
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
		return nil, err
	}

	if err := json.Unmarshal([]byte(scopesJSON), &k.Scopes); err != nil || k.Scopes == nil {
		k.Scopes = []string{}
	}
	if expiresAt.Valid {
		k.ExpiresAt = &expiresAt.String
	}
	if lastUsedAt.Valid {
		k.LastUsedAt = &lastUsedAt.String
	}
	k.Status = DeriveStatus(&k, time.Now().UTC())
	return &k, nil
}

// checkGrantedRoles verifies every role id resolves and, unless the caller holds the wildcard, that the caller holds all of each role's permissions.
func (s *Service) checkGrantedRoles(ctx context.Context, roleIDs []string, held map[string]bool, wildcard bool) error {
	found := 0
	rows, err := s.db.QueryContext(ctx,
		`SELECT permissions FROM roles WHERE id IN (`+placeholders(len(roleIDs))+`)`, // SQL identifier: placeholders() emits only "?" markers
		anyArgs(roleIDs)...)
	if err != nil {
		return fmt.Errorf("querying granted roles: %w", err)
	}
	defer func() { logCloseErr(rows.Close()) }()
	for rows.Next() {
		var permsJSON string
		if err := rows.Scan(&permsJSON); err != nil {
			return fmt.Errorf("scanning granted role: %w", err)
		}
		found++
		if wildcard {
			continue
		}
		var perms []string
		if err := json.Unmarshal([]byte(permsJSON), &perms); err != nil {
			return ErrGrantInvalid
		}
		if !allHeld(perms, held) {
			return ErrGrantExceedsCaller
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating granted roles: %w", err)
	}
	if found != len(roleIDs) {
		return ErrGrantInvalid
	}
	return nil
}

const (
	rolesForKeysSelect = `SELECT akr.api_key_id, r.id, r.name
		   FROM api_key_roles akr
		  INNER JOIN roles r ON r.id = akr.role_id
		  WHERE akr.api_key_id IN (`
	rolesForKeysTail = `)
		  ORDER BY r.name LIMIT 1000`
	envsForKeysSelect = `SELECT ake.api_key_id, ake.environment_id, e.name
		   FROM api_key_environments ake
		   LEFT JOIN environments e ON e.id = ake.environment_id
		  WHERE ake.api_key_id IN (`
	envsForKeysTail = `) ORDER BY ake.environment_id LIMIT 1000`
)
