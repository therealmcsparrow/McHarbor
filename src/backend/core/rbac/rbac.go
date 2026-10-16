// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Service provides RBAC permission checking.
type Service struct {
	db    *sql.DB
	mu    sync.RWMutex
	cache map[string][]Permission // key: "userID:envID:stackName"

	// Optional ABAC wiring. Each setter is a no-op when the argument
	// is nil, so legacy installs that don't enable ABAC continue to
	// work without any change to main.go.
	policies   *PolicyService   // nil → ABAC disabled
	attributes *AttributeBundle // nil → ABAC disabled
	abacExec   *ABACExecutor    // nil → ABAC disabled
	rolloutV3  *RolloutV3       // nil → v3 flag defaults to off

	// Lazy-initialised map of permission → scope string from the
	// catalog seed. Populated on first call to scopeFor(); guarded by
	// scopeOnce so concurrent reads are safe.
	scopeOnce sync.Once
	scopeMap  map[Permission]string
}

// NewService creates a new RBAC service.
func NewService(db *sql.DB) *Service {
	return &Service{
		db:    db,
		cache: make(map[string][]Permission),
	}
}

// SetPolicyService installs the ABAC policy service. Pass nil to disable.
func (s *Service) SetPolicyService(p *PolicyService) { s.policies = p }

// SetAttributeBundle installs the attribute provider bundle. Pass nil to disable.
func (s *Service) SetAttributeBundle(b *AttributeBundle) { s.attributes = b }

// SetExecutor installs the ABAC JS executor. Pass nil to disable.
func (s *Service) SetExecutor(e *ABACExecutor) { s.abacExec = e }

// SetRolloutV3 installs the v3 rollout flag. Pass nil to disable v3.
func (s *Service) SetRolloutV3(r *RolloutV3) { s.rolloutV3 = r }

// abacEnabled returns true when the ABAC feature flag is on. A nil
// rollout watcher, a missing settings row, or a malformed value all
// resolve to false so the role-based gate remains the source of
// truth unless an admin has explicitly opted in via Settings.
func (s *Service) abacEnabled(ctx context.Context) bool {
	if s == nil || s.rolloutV3 == nil {
		return false
	}
	return s.rolloutV3.isEnabled(ctx)
}

// HasPermission checks if the principal has a specific permission for an
// environment. envID can be empty for global checks. When ctx carries an
// APIKeyPrincipal the answer is the intersection of the key's set and the
// owner's set.
func (s *Service) HasPermission(ctx context.Context, userID, envID string, perm Permission) (bool, error) {
	perms, err := s.EffectivePermissions(ctx, userID, envID)
	if err != nil {
		return false, err
	}

	return hasPermission(perms, perm), nil
}

// HasStackPermission checks if the principal has a specific permission for a stack.
// Stack-scoped assignments are only considered for stack routes.
func (s *Service) HasStackPermission(ctx context.Context, userID, envID, stackName string, perm Permission) (bool, error) {
	perms, err := s.EffectivePermissionsForStack(ctx, userID, envID, stackName)
	if err != nil {
		return false, err
	}

	return hasPermission(perms, perm), nil
}

// HasAnyStackPermission checks if the principal can access at least one stack for the given permission.
func (s *Service) HasAnyStackPermission(ctx context.Context, userID, envID string, perm Permission) (bool, error) {
	allowed, err := s.HasPermission(ctx, userID, envID, perm)
	if err != nil || allowed {
		return allowed, err
	}

	stackNames, err := s.AllowedStackNames(ctx, userID, envID, perm)
	if err != nil {
		return false, err
	}

	return len(stackNames) > 0, nil
}

// EffectivePermissions returns the union of all permissions for a user in an
// environment, intersected with the API key principal in ctx when present.
// Combines direct user_roles and group_roles assignments.
func (s *Service) EffectivePermissions(ctx context.Context, userID, envID string) ([]Permission, error) {
	return s.effectivePermissionsFor(ctx, userID, envID, "")
}

// EffectivePermissionsForStack returns the permissions available for a specific stack.
func (s *Service) EffectivePermissionsForStack(ctx context.Context, userID, envID, stackName string) ([]Permission, error) {
	return s.effectivePermissionsFor(ctx, userID, envID, stackName)
}

// effectivePermissionsFor applies the API key rule on top of the owner set.
func (s *Service) effectivePermissionsFor(ctx context.Context, userID, envID, stackName string) ([]Permission, error) {
	owner, err := s.effectivePermissions(ctx, userID, envID, stackName)
	if err != nil {
		return nil, err
	}
	key := APIKeyFromContext(ctx)
	if key == nil {
		// Interactive session: fold in any approved, unexpired elevation.
		//
		// This sits outside effectivePermissions on purpose. That function
		// caches its result, which is right for role-derived permissions and
		// wrong for a time-bounded grant — a cached elevation would keep
		// granting after approved_until until something evicted the entry.
		//
		// Deliberately NOT applied on the API-key path below. An elevation is
		// approved for a person in a session; silently extending it to a
		// long-lived non-interactive credential that happens to act as them is
		// a wider grant than the approver agreed to. A key is still limited to
		// its own permission set, so nothing here narrows what keys can do.
		elevated, err := s.activeElevatedPermissions(ctx, userID, envID)
		if err != nil {
			return nil, err
		}
		// An open break-glass session is the whole emergency grant; it is
		// session-only for the same reason elevations are.
		emergency, err := s.breakGlassActive(ctx, userID)
		if err != nil {
			return nil, err
		}
		if emergency {
			return []Permission{PermWildcard}, nil
		}
		return withElevations(owner, elevated), nil
	}
	if !key.allowsEnvironment(envID) {
		return []Permission{}, nil
	}
	keyPerms, err := s.keyPermissions(ctx, key)
	if err != nil {
		return nil, err
	}
	return intersectWithKey(keyPerms, owner), nil
}

func (s *Service) effectivePermissions(ctx context.Context, userID, envID, stackName string) ([]Permission, error) {
	cacheKey := permissionCacheKey(userID, envID, stackName)

	s.mu.RLock()
	if cached, ok := s.cache[cacheKey]; ok {
		s.mu.RUnlock()
		return cached, nil
	}
	s.mu.RUnlock()

	grants, err := s.collectRoleGrants(ctx, userID, envID)
	if err != nil {
		return nil, err
	}
	var globalRoleIDs, envRoleIDs []string
	for _, grant := range grants {
		if grant.EnvironmentID == "" {
			globalRoleIDs = appendUniqueRoleIDs(globalRoleIDs, []string{grant.RoleID})
		} else {
			envRoleIDs = appendUniqueRoleIDs(envRoleIDs, []string{grant.RoleID})
		}
	}
	if stackName != "" {
		scopedRoleIDs, err := s.collectScopedRoleIDs(ctx, userID, envID)
		if err != nil {
			return nil, err
		}
		envRoleIDs = appendUniqueRoleIDs(envRoleIDs, scopedRoleIDs[stackName])
	}

	if len(globalRoleIDs) == 0 && len(envRoleIDs) == 0 {
		return []Permission{}, nil
	}

	globalPerms, err := s.resolvePermissions(ctx, globalRoleIDs)
	if err != nil {
		return nil, err
	}
	envPerms, err := s.resolvePermissions(ctx, envRoleIDs)
	if err != nil {
		return nil, err
	}
	// An assignment on one environment grants only what is scoped to an
	// environment; its global and account keys need a global assignment.
	envPerms, err = s.environmentScoped(ctx, envPerms)
	if err != nil {
		return nil, err
	}
	perms := unionPermissions(globalPerms, envPerms)

	s.mu.Lock()
	s.cache[cacheKey] = perms
	s.mu.Unlock()

	return perms, nil
}

// AllowedStackNames returns stack names the principal can access with the given permission.
func (s *Service) AllowedStackNames(ctx context.Context, userID, envID string, perm Permission) ([]string, error) {
	if envID == "" {
		return []string{}, nil
	}
	if key := APIKeyFromContext(ctx); key != nil {
		if !key.allowsEnvironment(envID) {
			return []string{}, nil
		}
		keyPerms, err := s.keyPermissions(ctx, key)
		if err != nil {
			return nil, err
		}
		if !hasPermission(keyPerms, perm) {
			return []string{}, nil
		}
	}

	scopedRoleIDs, err := s.collectScopedRoleIDs(ctx, userID, envID)
	if err != nil {
		return nil, err
	}

	stackNames := make([]string, 0, len(scopedRoleIDs))
	for stackName, roleIDs := range scopedRoleIDs {
		perms, err := s.resolvePermissions(ctx, roleIDs)
		if err != nil {
			return nil, err
		}
		if hasPermission(perms, perm) {
			stackNames = append(stackNames, stackName)
		}
	}

	sort.Strings(stackNames)
	return stackNames, nil
}

// InvalidateCache clears the permission cache for all users or a specific user.
func (s *Service) InvalidateCache(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if userID == "" {
		s.cache = make(map[string][]Permission)
		return
	}

	for key := range s.cache {
		// Keys are "userID:envID:stackName", check prefix
		if len(key) >= len(userID) && key[:len(userID)] == userID && (len(key) == len(userID) || key[len(userID)] == ':') {
			delete(s.cache, key)
		}
	}
}

// collectRoleIDs gathers all role IDs from direct assignments and group memberships.
// The grants come from collectRoleGrants so the permission explainer reads exactly
// the rows this resolution path reads.
func (s *Service) collectRoleIDs(ctx context.Context, userID, envID string) ([]string, error) {
	grants, err := s.collectRoleGrants(ctx, userID, envID)
	if err != nil {
		return nil, err
	}
	var roleIDs []string
	for _, grant := range grants {
		roleIDs = appendUniqueRoleIDs(roleIDs, []string{grant.RoleID})
	}
	return roleIDs, nil
}

func (s *Service) collectScopedRoleIDs(ctx context.Context, userID, envID string) (map[string][]string, error) {
	scopedRoleIDs := make(map[string][]string)
	grants, err := s.collectScopedRoleGrants(ctx, userID, envID)
	if err != nil {
		return nil, err
	}
	for _, grant := range grants {
		scopedRoleIDs[grant.StackName] = appendUniqueRoleIDs(scopedRoleIDs[grant.StackName], []string{grant.RoleID})
	}
	return scopedRoleIDs, nil
}

// resolvePermissions loads permission JSON from roles and unions them.
//
// One statement for the whole role set, not one per role. This runs on the
// permission-check path, so a user holding six roles used to cost six
// round-trips on every request that consulted their permissions.
func (s *Service) resolvePermissions(ctx context.Context, roleIDs []string) ([]Permission, error) {
	if len(roleIDs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(roleIDs))
	args := make([]any, len(roleIDs))
	for i, rid := range roleIDs {
		placeholders[i] = "?"
		args[i] = rid
	}

	rows, err := s.db.QueryContext(ctx,
		"SELECT permissions FROM roles WHERE id IN ("+strings.Join(placeholders, ",")+")", args...) // SQL identifier: placeholders holds only "?" markers
	if err != nil {
		return nil, fmt.Errorf("querying role permissions: %w", err)
	}
	defer closeRows(rows)

	seen := make(map[Permission]bool)
	var result []Permission

	for rows.Next() {
		var permsJSON string
		if err := rows.Scan(&permsJSON); err != nil {
			return nil, fmt.Errorf("scanning role permissions: %w", err)
		}

		perms, ok := parseRolePermissions(permsJSON)
		if !ok {
			continue
		}

		for _, perm := range perms {
			if !seen[perm] {
				seen[perm] = true
				result = append(result, perm)
			}
			// Short-circuit on wildcard
			if perm == PermWildcard {
				return []Permission{PermWildcard}, nil
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating role permissions: %w", err)
	}

	return result, nil
}

func permissionCacheKey(userID, envID, stackName string) string {
	return userID + ":" + envID + ":" + stackName
}

func hasPermission(perms []Permission, perm Permission) bool {
	for _, p := range perms {
		if p == PermWildcard || p == perm {
			return true
		}
	}

	return false
}

func appendUniqueRoleIDs(existing []string, additions []string) []string {
	seen := make(map[string]bool, len(existing))
	for _, roleID := range existing {
		seen[roleID] = true
	}

	for _, roleID := range additions {
		if seen[roleID] {
			continue
		}
		seen[roleID] = true
		existing = append(existing, roleID)
	}

	return existing
}
