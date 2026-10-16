// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"fmt"
)

// envGrantable reports whether a key with this catalog scope may be granted by
// an environment-scoped assignment. A plugin declares "env" where the host
// catalog says "environment".
func envGrantable(scope string) bool {
	switch scope {
	case "environment", "env", "stack", "owner":
		return true
	}
	return false
}

// permissionScopes maps every known key to its scope: the host catalog plus the
// keys installed plugins declared. A key in neither is absent, and absent keys
// are treated as not environment-grantable.
func (s *Service) permissionScopes(ctx context.Context) (_ map[Permission]string, retErr error) {
	scopes := make(map[Permission]string, len(CatalogSeed()))
	for _, e := range CatalogSeed() {
		scopes[e.Key] = e.Scope
	}
	if s.db == nil {
		return scopes, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT permission_key, scope FROM plugin_permissions LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("querying plugin permission scopes: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil && retErr == nil {
			retErr = fmt.Errorf("closing plugin permission scopes: %w", cerr)
		}
	}()
	for rows.Next() {
		var key, scope string
		if err := rows.Scan(&key, &scope); err != nil {
			return nil, fmt.Errorf("scanning plugin permission scope: %w", err)
		}
		if _, host := scopes[Permission(key)]; !host {
			scopes[Permission(key)] = scope
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating plugin permission scopes: %w", err)
	}
	return scopes, nil
}

// environmentScoped narrows what an environment-scoped assignment grants to the
// keys that are meaningful per environment. Without it a role assigned on one
// environment granted its global keys too (users.manage, roles.manage, …) to any
// request naming that environment, which is a path to global admin. A wildcard
// on such a role expands to the environment-grantable keys only.
func (s *Service) environmentScoped(ctx context.Context, perms []Permission) ([]Permission, error) {
	if len(perms) == 0 {
		return perms, nil
	}
	scopes, err := s.scopesFor(ctx, perms)
	if err != nil {
		return nil, err
	}
	if hasPermission(perms, PermWildcard) {
		out := make([]Permission, 0, len(scopes))
		for key, scope := range scopes {
			if envGrantable(scope) {
				out = append(out, key)
			}
		}
		return out, nil
	}
	out := make([]Permission, 0, len(perms))
	for _, p := range perms {
		if envGrantable(scopes[p]) {
			out = append(out, p)
		}
	}
	return out, nil
}

// scopesFor returns the host catalog's scopes, reading plugin scopes only when
// perms holds a wildcard or a key the catalog does not know, which keeps the
// common case off the database.
func (s *Service) scopesFor(ctx context.Context, perms []Permission) (map[Permission]string, error) {
	catalog := make(map[Permission]string, len(CatalogSeed()))
	for _, e := range CatalogSeed() {
		catalog[e.Key] = e.Scope
	}
	for _, p := range perms {
		if _, known := catalog[p]; !known {
			return s.permissionScopes(ctx)
		}
	}
	return catalog, nil
}

// unionPermissions merges two permission sets without duplicates.
func unionPermissions(a, b []Permission) []Permission {
	seen := make(map[Permission]bool, len(a)+len(b))
	out := make([]Permission, 0, len(a)+len(b))
	for _, set := range [][]Permission{a, b} {
		for _, p := range set {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out
}
