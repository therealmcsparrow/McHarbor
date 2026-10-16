// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// RoleGrant is one role assignment that reaches a user, either directly or
// through a group, and the scope it was assigned at.
type RoleGrant struct {
	RoleID        string `json:"roleId"`
	Source        string `json:"source"`
	GroupID       string `json:"groupId,omitempty"`
	EnvironmentID string `json:"environmentId,omitempty"`
	StackName     string `json:"stackName,omitempty"`
}

// Role grant sources.
const (
	GrantSourceUser  = "user"
	GrantSourceGroup = "group"
)

// PermissionSource explains one reason a permission appears in an effective
// permission listing. Kind is one of the SourceKind constants.
type PermissionSource struct {
	Kind          string `json:"kind"`
	RoleID        string `json:"roleId,omitempty"`
	RoleName      string `json:"roleName,omitempty"`
	Via           string `json:"via,omitempty"`
	GroupID       string `json:"groupId,omitempty"`
	GroupName     string `json:"groupName,omitempty"`
	EnvironmentID string `json:"environmentId,omitempty"`
	StackName     string `json:"stackName,omitempty"`
	Wildcard      bool   `json:"wildcard,omitempty"`
	ElevationID   string `json:"elevationId,omitempty"`
	Until         string `json:"until,omitempty"`
	OverrideID    string `json:"overrideId,omitempty"`
	ResourceType  string `json:"resourceType,omitempty"`
	ResourceID    string `json:"resourceId,omitempty"`
	Conditional   bool   `json:"conditional,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

// Permission source kinds.
const (
	SourceKindRole          = "role"
	SourceKindElevation     = "elevation"
	SourceKindBreakGlass    = "break_glass"
	SourceKindOverrideGrant = "override_grant"
	SourceKindOverrideDeny  = "override_deny"
)

// ExplainedPermission is one permission key with the verdict the route gates
// reach for it and every source that contributes to that verdict.
type ExplainedPermission struct {
	Key            Permission         `json:"key"`
	Effective      bool               `json:"effective"`
	AuthorizeGrant bool               `json:"authorizeGrant"`
	Sources        []PermissionSource `json:"sources"`
}

// PermissionExplanation is the whole effective permission set of one user at
// one scope. Effective is what every RequirePermission gate consults (roles,
// elevations, break-glass); AuthorizeGrant also folds in user-level overrides
// the way Authorize does, because overrides are only read on that path.
type PermissionExplanation struct {
	UserID        string                `json:"userId"`
	EnvironmentID string                `json:"environmentId,omitempty"`
	StackName     string                `json:"stackName,omitempty"`
	Wildcard      bool                  `json:"wildcard"`
	BreakGlass    bool                  `json:"breakGlass"`
	ABACEnforced  bool                  `json:"abacEnforced"`
	Permissions   []ExplainedPermission `json:"permissions"`
	Grants        []RoleGrant           `json:"grants"`
	StackScopes   []string              `json:"stackScopes"`
}

// ExplainPermissions resolves userID's permissions at envID (and stackName when
// set) through effectivePermissionsFor, the function every gate reaches, then
// attributes each key to its sources. catalog lists the keys to report; nil
// means CatalogSeed. Any API key in ctx is dropped: the answer is the user's.
func (s *Service) ExplainPermissions(ctx context.Context, userID, envID, stackName string, catalog []Permission) (*PermissionExplanation, error) {
	if stackName != "" && envID == "" {
		return nil, fmt.Errorf("stack scope requires an environment")
	}
	ctx = WithAPIKey(ctx, nil)
	if catalog == nil {
		for _, entry := range CatalogSeed() {
			catalog = append(catalog, entry.Key)
		}
	}

	effective, err := s.effectivePermissionsFor(ctx, userID, envID, stackName)
	if err != nil {
		return nil, err
	}
	grants, err := s.collectRoleGrants(ctx, userID, envID)
	if err != nil {
		return nil, err
	}
	scoped, err := s.collectScopedRoleGrants(ctx, userID, envID)
	if err != nil {
		return nil, err
	}
	stackScopes := make([]string, 0)
	seenStack := map[string]bool{}
	for _, grant := range scoped {
		if !seenStack[grant.StackName] {
			seenStack[grant.StackName] = true
			stackScopes = append(stackScopes, grant.StackName)
		}
		if stackName != "" && grant.StackName == stackName {
			grants = append(grants, grant)
		}
	}
	sort.Strings(stackScopes)

	elevations, err := s.activeElevations(ctx, userID, envID)
	if err != nil {
		return nil, err
	}
	breakGlass, err := s.breakGlassActive(ctx, userID)
	if err != nil {
		return nil, err
	}
	roles, err := s.roleDetails(ctx, grants)
	if err != nil {
		return nil, err
	}
	groupNames, err := s.groupNames(ctx, grants)
	if err != nil {
		return nil, err
	}
	overrides, err := s.userOverrides(ctx, userID)
	if err != nil {
		return nil, err
	}

	sources := map[Permission][]PermissionSource{}
	wildcard := false
	for _, grant := range grants {
		role, ok := roles[grant.RoleID]
		if !ok {
			continue
		}
		base := PermissionSource{
			Kind:          SourceKindRole,
			RoleID:        grant.RoleID,
			RoleName:      role.name,
			Via:           grant.Source,
			GroupID:       grant.GroupID,
			GroupName:     groupNames[grant.GroupID],
			EnvironmentID: grant.EnvironmentID,
			StackName:     grant.StackName,
		}
		for _, perm := range role.permissions {
			if perm == PermWildcard {
				wildcard = true
				source := base
				source.Wildcard = true
				for _, key := range catalog {
					sources[key] = append(sources[key], source)
				}
				continue
			}
			sources[perm] = append(sources[perm], base)
		}
	}
	for _, grant := range elevations {
		sources[grant.Permission] = append(sources[grant.Permission], PermissionSource{
			Kind:          SourceKindElevation,
			ElevationID:   grant.RequestID,
			EnvironmentID: grant.EnvironmentID,
			Until:         grant.ApprovedUntil,
		})
	}
	if breakGlass {
		for _, key := range catalog {
			sources[key] = append(sources[key], PermissionSource{Kind: SourceKindBreakGlass, Wildcard: true})
		}
	}

	now := time.Now()
	denied := map[Permission]bool{}
	granted := map[Permission]bool{}
	for _, o := range overrides {
		req := AuthzRequest{
			UserID:        userID,
			Permission:    o.permission,
			EnvironmentID: envID,
			StackName:     stackName,
			ResourceType:  o.row.ResourceType.String,
			ResourceID:    o.row.ResourceID.String,
		}
		if !overrideApplies(o.row, req, now) {
			continue
		}
		conditional := o.row.ResourceType.Valid || o.row.ResourceID.Valid
		kind := SourceKindOverrideGrant
		if o.effect == "deny" {
			kind = SourceKindOverrideDeny
		}
		if !conditional {
			if o.effect == "deny" {
				denied[o.permission] = true
			} else {
				granted[o.permission] = true
			}
		}
		sources[o.permission] = append(sources[o.permission], PermissionSource{
			Kind:          kind,
			OverrideID:    o.id,
			EnvironmentID: o.row.EnvironmentID.String,
			StackName:     o.row.StackName.String,
			ResourceType:  o.row.ResourceType.String,
			ResourceID:    o.row.ResourceID.String,
			Until:         o.row.ExpiresAt.String,
			Conditional:   conditional,
			Reason:        o.reason,
		})
	}

	keys := append([]Permission(nil), catalog...)
	known := make(map[Permission]bool, len(keys))
	for _, key := range keys {
		known[key] = true
	}
	for key := range sources {
		if !known[key] && key != PermWildcard {
			known[key] = true
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	out := &PermissionExplanation{
		UserID:        userID,
		EnvironmentID: envID,
		StackName:     stackName,
		Wildcard:      wildcard,
		BreakGlass:    breakGlass,
		ABACEnforced:  s.policies != nil && s.attributes != nil && s.abacEnabled(ctx),
		Permissions:   make([]ExplainedPermission, 0, len(keys)),
		Grants:        grants,
		StackScopes:   stackScopes,
	}
	if out.Grants == nil {
		out.Grants = []RoleGrant{}
	}
	for _, key := range keys {
		isEffective := hasPermission(effective, key)
		authorize := isEffective
		if granted[key] {
			authorize = true
		}
		if denied[key] {
			authorize = false
		}
		list := sources[key]
		if list == nil {
			list = []PermissionSource{}
		}
		out.Permissions = append(out.Permissions, ExplainedPermission{
			Key:            key,
			Effective:      isEffective,
			AuthorizeGrant: authorize,
			Sources:        list,
		})
	}
	return out, nil
}

// collectRoleGrants returns the environment-wide and global role grants for
// userID: direct user_roles first, then group_roles, stack-scoped rows excluded.
func (s *Service) collectRoleGrants(ctx context.Context, userID, envID string) ([]RoleGrant, error) {
	var grants []RoleGrant

	rows, err := s.db.QueryContext(ctx,
		`SELECT role_id, COALESCE(environment_id, '') FROM user_roles
		 WHERE user_id = ? AND stack_name IS NULL
		   AND (environment_id IS NULL OR environment_id = ?)`,
		userID, envID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying user_roles: %w", err)
	}
	for rows.Next() {
		grant := RoleGrant{Source: GrantSourceUser}
		if err := rows.Scan(&grant.RoleID, &grant.EnvironmentID); err != nil {
			closeRows(rows)
			return nil, fmt.Errorf("scanning user_role: %w", err)
		}
		grants = append(grants, grant)
	}
	if err := rows.Err(); err != nil {
		closeRows(rows)
		return nil, fmt.Errorf("iterating user_roles: %w", err)
	}
	closeRows(rows)

	groupRows, err := s.db.QueryContext(ctx,
		`SELECT gr.role_id, gr.group_id, COALESCE(gr.environment_id, '') FROM group_roles gr
		 INNER JOIN group_members gm ON gm.group_id = gr.group_id
		 WHERE gm.user_id = ? AND gr.stack_name IS NULL
		   AND (gr.environment_id IS NULL OR gr.environment_id = ?)`,
		userID, envID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying group_roles: %w", err)
	}
	defer closeRows(groupRows)
	for groupRows.Next() {
		grant := RoleGrant{Source: GrantSourceGroup}
		if err := groupRows.Scan(&grant.RoleID, &grant.GroupID, &grant.EnvironmentID); err != nil {
			return nil, fmt.Errorf("scanning group_role: %w", err)
		}
		grants = append(grants, grant)
	}
	if err := groupRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating group_roles: %w", err)
	}
	return grants, nil
}

// collectScopedRoleGrants returns the stack-scoped grants in envID; stack
// scopes only exist inside an environment, so an empty envID yields none.
func (s *Service) collectScopedRoleGrants(ctx context.Context, userID, envID string) ([]RoleGrant, error) {
	if envID == "" {
		return nil, nil
	}
	var grants []RoleGrant

	rows, err := s.db.QueryContext(ctx,
		`SELECT role_id, stack_name FROM user_roles
		 WHERE user_id = ? AND environment_id = ? AND stack_name IS NOT NULL`,
		userID, envID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying scoped user_roles: %w", err)
	}
	for rows.Next() {
		grant := RoleGrant{Source: GrantSourceUser, EnvironmentID: envID}
		if err := rows.Scan(&grant.RoleID, &grant.StackName); err != nil {
			closeRows(rows)
			return nil, fmt.Errorf("scanning scoped user_role: %w", err)
		}
		grants = append(grants, grant)
	}
	if err := rows.Err(); err != nil {
		closeRows(rows)
		return nil, fmt.Errorf("iterating scoped user_roles: %w", err)
	}
	closeRows(rows)

	groupRows, err := s.db.QueryContext(ctx,
		`SELECT gr.role_id, gr.group_id, gr.stack_name FROM group_roles gr
		 INNER JOIN group_members gm ON gm.group_id = gr.group_id
		 WHERE gm.user_id = ? AND gr.environment_id = ? AND gr.stack_name IS NOT NULL`,
		userID, envID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying scoped group_roles: %w", err)
	}
	defer closeRows(groupRows)
	for groupRows.Next() {
		grant := RoleGrant{Source: GrantSourceGroup, EnvironmentID: envID}
		if err := groupRows.Scan(&grant.RoleID, &grant.GroupID, &grant.StackName); err != nil {
			return nil, fmt.Errorf("scanning scoped group_role: %w", err)
		}
		grants = append(grants, grant)
	}
	if err := groupRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating scoped group_roles: %w", err)
	}
	return grants, nil
}

// parseRolePermissions decodes a roles.permissions JSON array; ok is false for
// a malformed row, which resolution skips rather than failing the request.
func parseRolePermissions(permsJSON string) ([]Permission, bool) {
	var raw []string
	if err := json.Unmarshal([]byte(permsJSON), &raw); err != nil {
		return nil, false
	}
	out := make([]Permission, 0, len(raw))
	for _, p := range raw {
		out = append(out, Permission(p))
	}
	return out, true
}

// overrideRow is the scope part of a user_permissions row.
type overrideRow struct {
	EnvironmentID sql.NullString
	StackName     sql.NullString
	ResourceType  sql.NullString
	ResourceID    sql.NullString
	ExpiresAt     sql.NullString
}

// overrideApplies reports whether an unexpired override row matches req; a
// NULL column is a wildcard and a set one must equal the request. Shared by
// Authorize and ExplainPermissions so the two cannot drift.
func overrideApplies(row overrideRow, req AuthzRequest, now time.Time) bool {
	if row.ExpiresAt.Valid && row.ExpiresAt.String != "" {
		if row.ExpiresAt.String <= now.UTC().Format("2006-01-02T15:04:05Z") {
			return false
		}
	}
	if row.EnvironmentID.Valid && row.EnvironmentID.String != req.EnvironmentID {
		return false
	}
	if row.StackName.Valid && row.StackName.String != req.StackName {
		return false
	}
	if row.ResourceType.Valid && row.ResourceType.String != req.ResourceType {
		return false
	}
	if row.ResourceID.Valid && row.ResourceID.String != req.ResourceID {
		return false
	}
	return true
}

type roleDetail struct {
	name        string
	permissions []Permission
}

type userOverride struct {
	id         string
	permission Permission
	effect     string
	reason     string
	row        overrideRow
}

func (s *Service) roleDetails(ctx context.Context, grants []RoleGrant) (map[string]roleDetail, error) {
	out := map[string]roleDetail{}
	ids := make([]any, 0, len(grants))
	seen := map[string]bool{}
	for _, grant := range grants {
		if !seen[grant.RoleID] {
			seen[grant.RoleID] = true
			ids = append(ids, grant.RoleID)
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	rows, err := s.db.QueryContext(ctx, "SELECT id, name, permissions FROM roles WHERE id IN ("+placeholders+")", ids...) // SQL identifier: placeholders holds only "?" markers
	if err != nil {
		return nil, fmt.Errorf("querying role details: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var id, name, permsJSON string
		if err := rows.Scan(&id, &name, &permsJSON); err != nil {
			return nil, fmt.Errorf("scanning role details: %w", err)
		}
		perms, ok := parseRolePermissions(permsJSON)
		if !ok {
			continue
		}
		out[id] = roleDetail{name: name, permissions: perms}
	}
	return out, rows.Err()
}

func (s *Service) groupNames(ctx context.Context, grants []RoleGrant) (map[string]string, error) {
	out := map[string]string{}
	ids := make([]any, 0)
	for _, grant := range grants {
		if grant.GroupID != "" {
			if _, ok := out[grant.GroupID]; !ok {
				out[grant.GroupID] = ""
				ids = append(ids, grant.GroupID)
			}
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	rows, err := s.db.QueryContext(ctx, "SELECT id, name FROM groups WHERE id IN ("+placeholders+")", ids...) // SQL identifier: placeholders holds only "?" markers
	if err != nil {
		return nil, fmt.Errorf("querying group names: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("scanning group names: %w", err)
		}
		out[id] = name
	}
	return out, rows.Err()
}

func (s *Service) userOverrides(ctx context.Context, userID string) ([]userOverride, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, permission_key, effect, COALESCE(reason, ''),
		       environment_id, stack_name, resource_type, resource_id, expires_at
		  FROM user_permissions
		 WHERE user_id = ?
		 LIMIT 1000`, userID)
	if err != nil {
		return nil, fmt.Errorf("querying user overrides: %w", err)
	}
	defer closeRows(rows)
	var out []userOverride
	for rows.Next() {
		var o userOverride
		var key string
		if err := rows.Scan(&o.id, &key, &o.effect, &o.reason,
			&o.row.EnvironmentID, &o.row.StackName, &o.row.ResourceType, &o.row.ResourceID, &o.row.ExpiresAt); err != nil {
			return nil, fmt.Errorf("scanning user override: %w", err)
		}
		o.permission = Permission(key)
		out = append(out, o)
	}
	return out, rows.Err()
}
