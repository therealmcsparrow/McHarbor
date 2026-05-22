// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

// Service provides RBAC permission checking.
type Service struct {
	db    *sql.DB
	mu    sync.RWMutex
	cache map[string][]Permission // key: "userID:envID:stackName"
}

// NewService creates a new RBAC service.
func NewService(db *sql.DB) *Service {
	return &Service{
		db:    db,
		cache: make(map[string][]Permission),
	}
}

// HasPermission checks if a user has a specific permission for an environment.
// envID can be empty for global checks.
func (s *Service) HasPermission(userID, envID string, perm Permission) (bool, error) {
	perms, err := s.EffectivePermissions(userID, envID)
	if err != nil {
		return false, err
	}

	return hasPermission(perms, perm), nil
}

// HasStackPermission checks if a user has a specific permission for a stack.
// Stack-scoped assignments are only considered for stack routes.
func (s *Service) HasStackPermission(userID, envID, stackName string, perm Permission) (bool, error) {
	perms, err := s.EffectivePermissionsForStack(userID, envID, stackName)
	if err != nil {
		return false, err
	}

	return hasPermission(perms, perm), nil
}

// HasAnyStackPermission checks if a user can access at least one stack for the given permission.
func (s *Service) HasAnyStackPermission(userID, envID string, perm Permission) (bool, error) {
	allowed, err := s.HasPermission(userID, envID, perm)
	if err != nil || allowed {
		return allowed, err
	}

	stackNames, err := s.AllowedStackNames(userID, envID, perm)
	if err != nil {
		return false, err
	}

	return len(stackNames) > 0, nil
}

// EffectivePermissions returns the union of all permissions for a user in an environment.
// Combines direct user_roles and group_roles assignments.
func (s *Service) EffectivePermissions(userID, envID string) ([]Permission, error) {
	return s.effectivePermissions(userID, envID, "")
}

// EffectivePermissionsForStack returns the permissions available for a specific stack.
func (s *Service) EffectivePermissionsForStack(userID, envID, stackName string) ([]Permission, error) {
	return s.effectivePermissions(userID, envID, stackName)
}

func (s *Service) effectivePermissions(userID, envID, stackName string) ([]Permission, error) {
	cacheKey := permissionCacheKey(userID, envID, stackName)

	s.mu.RLock()
	if cached, ok := s.cache[cacheKey]; ok {
		s.mu.RUnlock()
		return cached, nil
	}
	s.mu.RUnlock()

	roleIDs, err := s.collectRoleIDs(userID, envID)
	if err != nil {
		return nil, err
	}
	if stackName != "" {
		scopedRoleIDs, err := s.collectScopedRoleIDs(userID, envID)
		if err != nil {
			return nil, err
		}
		roleIDs = appendUniqueRoleIDs(roleIDs, scopedRoleIDs[stackName])
	}

	if len(roleIDs) == 0 {
		return []Permission{}, nil
	}

	perms, err := s.resolvePermissions(roleIDs)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.cache[cacheKey] = perms
	s.mu.Unlock()

	return perms, nil
}

// AllowedStackNames returns stack names a user can access with the given permission.
func (s *Service) AllowedStackNames(userID, envID string, perm Permission) ([]string, error) {
	if envID == "" {
		return []string{}, nil
	}

	scopedRoleIDs, err := s.collectScopedRoleIDs(userID, envID)
	if err != nil {
		return nil, err
	}

	stackNames := make([]string, 0, len(scopedRoleIDs))
	for stackName, roleIDs := range scopedRoleIDs {
		perms, err := s.resolvePermissions(roleIDs)
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
func (s *Service) collectRoleIDs(userID, envID string) ([]string, error) {
	seen := make(map[string]bool)
	var roleIDs []string

	// Direct user_roles: global (environment_id IS NULL) + environment-specific
	rows, err := s.db.Query(
		`SELECT role_id FROM user_roles
		 WHERE user_id = ? AND stack_name IS NULL
		   AND (environment_id IS NULL OR environment_id = ?)`,
		userID, envID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying user_roles: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var rid string
		if err := rows.Scan(&rid); err != nil {
			return nil, fmt.Errorf("scanning user_role: %w", err)
		}
		if !seen[rid] {
			seen[rid] = true
			roleIDs = append(roleIDs, rid)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating user_roles: %w", err)
	}

	// Group memberships → group_roles
	groupRows, err := s.db.Query(
		`SELECT gr.role_id FROM group_roles gr
		 INNER JOIN group_members gm ON gm.group_id = gr.group_id
		 WHERE gm.user_id = ? AND gr.stack_name IS NULL
		   AND (gr.environment_id IS NULL OR gr.environment_id = ?)`,
		userID, envID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying group_roles: %w", err)
	}
	defer groupRows.Close()

	for groupRows.Next() {
		var rid string
		if err := groupRows.Scan(&rid); err != nil {
			return nil, fmt.Errorf("scanning group_role: %w", err)
		}
		if !seen[rid] {
			seen[rid] = true
			roleIDs = append(roleIDs, rid)
		}
	}
	if err := groupRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating group_roles: %w", err)
	}

	return roleIDs, nil
}

func (s *Service) collectScopedRoleIDs(userID, envID string) (map[string][]string, error) {
	scopedRoleIDs := make(map[string][]string)
	if envID == "" {
		return scopedRoleIDs, nil
	}

	rows, err := s.db.Query(
		`SELECT role_id, stack_name FROM user_roles
		 WHERE user_id = ? AND environment_id = ? AND stack_name IS NOT NULL`,
		userID, envID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying scoped user_roles: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var roleID, stackName string
		if err := rows.Scan(&roleID, &stackName); err != nil {
			return nil, fmt.Errorf("scanning scoped user_role: %w", err)
		}
		scopedRoleIDs[stackName] = appendUniqueRoleIDs(scopedRoleIDs[stackName], []string{roleID})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating scoped user_roles: %w", err)
	}

	groupRows, err := s.db.Query(
		`SELECT gr.role_id, gr.stack_name FROM group_roles gr
		 INNER JOIN group_members gm ON gm.group_id = gr.group_id
		 WHERE gm.user_id = ? AND gr.environment_id = ? AND gr.stack_name IS NOT NULL`,
		userID, envID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying scoped group_roles: %w", err)
	}
	defer groupRows.Close()

	for groupRows.Next() {
		var roleID, stackName string
		if err := groupRows.Scan(&roleID, &stackName); err != nil {
			return nil, fmt.Errorf("scanning scoped group_role: %w", err)
		}
		scopedRoleIDs[stackName] = appendUniqueRoleIDs(scopedRoleIDs[stackName], []string{roleID})
	}
	if err := groupRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating scoped group_roles: %w", err)
	}

	return scopedRoleIDs, nil
}

// resolvePermissions loads permission JSON from roles and unions them.
func (s *Service) resolvePermissions(roleIDs []string) ([]Permission, error) {
	seen := make(map[Permission]bool)
	var result []Permission

	for _, rid := range roleIDs {
		var permsJSON string
		err := s.db.QueryRow("SELECT permissions FROM roles WHERE id = ?", rid).Scan(&permsJSON)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("querying role permissions: %w", err)
		}

		var perms []string
		if err := json.Unmarshal([]byte(permsJSON), &perms); err != nil {
			continue
		}

		for _, p := range perms {
			perm := Permission(p)
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
