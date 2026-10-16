// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package permcatalog builds the permission catalog role editors and access
// reviews read: the host's CatalogSeed plus installed plugins' declared keys.
package permcatalog

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"

	corerbac "github.com/therealmcsparrow/mcharbor/core/rbac"
)

// Entry is the permission metadata exposed to the frontend.
type Entry struct {
	Key         string `json:"key"`
	Category    string `json:"category"`
	Action      string `json:"action"`
	Description string `json:"description"`
	Scope       string `json:"scope"`
	// Elevatable reports whether this key may be requested as a temporary
	// grant through modules/elevations. The flag lives only in the
	// permission_catalog table (seeded by migration 068 and deliberately
	// untouched by UpsertCatalog's ON CONFLICT clause), not in CatalogSeed(),
	// so it has to be read back from the database rather than derived here.
	//
	// Without it a request form can only offer every permission and let the
	// backend reject most of them with ErrPermissionNotElevatable, which is a
	// worse answer than not offering them.
	Elevatable bool `json:"elevatable"`
}

// Load returns the canonical permission metadata used by role editors:
// the host's own permissions plus the ones installed plugins have declared.
//
// Plugins are included because their routes are gated on their keys either way
// (routeRegistry.mount wraps every declared route in PluginPermissionMiddleware).
// A key absent from this list is a key no role editor can offer, which leaves the
// plugin usable only by a wildcard role.
func Load(ctx context.Context, db *sql.DB, logger *slog.Logger) []Entry {
	seed := corerbac.CatalogSeed()
	entries := make([]Entry, 0, len(seed))
	seen := make(map[string]struct{}, len(seed))
	for _, entry := range seed {
		entries = append(entries, Entry{
			Key:         string(entry.Key),
			Category:    entry.Category,
			Action:      entry.Action,
			Description: entry.Description,
			Scope:       entry.Scope,
		})
		seen[string(entry.Key)] = struct{}{}
	}
	entries = append(entries, pluginPermissions(ctx, db, logger, seen)...)
	markElevatable(ctx, db, logger, entries)
	return entries
}

// markElevatable sets Elevatable from the effective elevation rule, plugin keys
// included. A read failure is logged and swallowed: losing the flag costs the
// request form its filter, which is a smaller harm than costing the role
// editor its entire catalog.
func markElevatable(ctx context.Context, db *sql.DB, logger *slog.Logger, entries []Entry) {
	if db == nil {
		return
	}
	catalog, err := corerbac.ElevatableCatalog(ctx, db)
	if err != nil {
		logger.Error("auth: reading elevatable permissions", "error", err)
		return
	}
	elevatable := make(map[string]bool, len(catalog))
	for _, e := range catalog {
		elevatable[e.Key] = e.Elevatable
	}
	for i := range entries {
		entries[i].Elevatable = elevatable[entries[i].Key]
	}
}

// pluginPermissions returns the permissions installed plugins registered
// through AccessRegistry.AddPermission, skipping any key a system permission
// already owns so a plugin cannot shadow one.
//
// The plugin slug becomes the category: plugin_permissions carries no category
// column, and grouping by origin gives each plugin its own section in the role
// editor rather than one undifferentiated "Plugins" pile.
//
// Read failures are logged and swallowed. A broken plugin table should cost the
// operator the plugin rows, not the entire role editor.
func pluginPermissions(ctx context.Context, db *sql.DB, logger *slog.Logger, seen map[string]struct{}) []Entry {
	rows, err := db.QueryContext(ctx, `
		SELECT origin, permission_key, description, scope
		  FROM plugin_permissions
		 ORDER BY origin, permission_key
		 LIMIT 1000`)
	if err != nil {
		logger.Error("auth: querying plugin permissions", "error", err)
		return nil
	}
	defer func() {
		if err := rows.Close(); err != nil {
			logger.Error("auth: closing plugin permissions", "error", err)
		}
	}()

	var entries []Entry
	for rows.Next() {
		var origin, key, description, scope string
		if err := rows.Scan(&origin, &key, &description, &scope); err != nil {
			logger.Error("auth: scanning plugin permission", "error", err)
			return entries
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		entries = append(entries, Entry{
			Key:         key,
			Category:    origin,
			Action:      permissionAction(key),
			Description: description,
			Scope:       CatalogScope(scope),
		})
	}
	if err := rows.Err(); err != nil {
		logger.Error("auth: iterating plugin permissions", "error", err)
	}
	return entries
}

// permissionAction derives the catalog's action field from a permission key —
// "reporting.generate" is the "generate" action.
func permissionAction(key string) string {
	if i := strings.LastIndex(key, "."); i >= 0 && i+1 < len(key) {
		return key[i+1:]
	}
	return key
}

// CatalogScope maps a plugin manifest's scope vocabulary (global|env|stack)
// onto the catalog's (global|environment|stack|owner|account). The frontend
// models the latter as a closed union, so an unmapped "env" would arrive as a
// value it does not know.
func CatalogScope(scope string) string {
	switch scope {
	case "env", "environment":
		return "environment"
	case "stack":
		return "stack"
	case "owner":
		return "owner"
	case "account":
		return "account"
	default:
		return "global"
	}
}
