// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rs/xid"
)

// ElevatableEntry is one permission as the elevation form and its admin list
// see it; Source says whether an admin decided it or the default rule did.
type ElevatableEntry struct {
	Key         string `json:"key"`
	Category    string `json:"category"`
	Action      string `json:"action"`
	Description string `json:"description"`
	Origin      string `json:"origin"`
	Elevatable  bool   `json:"elevatable"`
	Source      string `json:"source"`
}

// Elevation sources reported on ElevatableEntry.Source.
const (
	ElevationSourceAdmin   = "admin"
	ElevationSourceDefault = "default"
)

var elevatableActions = map[string]bool{
	"manage": true, "delete": true, "exec": true, "run": true, "restore": true,
	"apply": true, "reveal": true, "enforce": true, "access": true, "act": true,
	"export": true, "use": true, "execute": true, "deploy": true, "promote": true,
	"rollback": true, "shell": true, "console": true, "update": true, "create": true,
}

var nonElevatableCategories = map[string]bool{
	"users": true, "roles": true, "groups": true, "permissions": true, "api_keys": true,
	"approvals": true, "account": true, "sessions": true, "identity_providers": true,
	"scim": true, "license": true, "rbac": true, "elevations": true, "audit": true,
}

// DefaultElevatable is the rule for keys no admin has decided: operational verbs
// may be borrowed, while anything that administers access itself never can, since
// a temporary grant there could make itself permanent.
func DefaultElevatable(key string) bool {
	category, action := splitKey(key)
	if category == "" || nonElevatableCategories[category] {
		return false
	}
	return elevatableActions[action]
}

func splitKey(key string) (string, string) {
	i := strings.LastIndex(key, ".")
	if i <= 0 || i == len(key)-1 {
		return "", ""
	}
	return strings.SplitN(key, ".", 2)[0], key[i+1:]
}

// ElevatableCatalog lists every host and plugin permission with its effective
// elevation flag: an admin override first, then the legacy catalog flag, then
// DefaultElevatable.
func ElevatableCatalog(ctx context.Context, db *sql.DB) ([]ElevatableEntry, error) {
	overrides, err := elevationOverrides(ctx, db)
	if err != nil {
		return nil, err
	}
	legacy := map[string]bool{}
	rows, err := db.QueryContext(ctx, `SELECT key, is_elevatable FROM permission_catalog LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("reading permission catalog: %w", err)
	}
	for rows.Next() {
		var key string
		var flag int
		if err := rows.Scan(&key, &flag); err != nil {
			closeRows(rows)
			return nil, fmt.Errorf("scanning permission catalog: %w", err)
		}
		legacy[key] = flag == 1
	}
	if err := rows.Err(); err != nil {
		closeRows(rows)
		return nil, fmt.Errorf("iterating permission catalog: %w", err)
	}
	closeRows(rows)

	seen := map[string]bool{}
	var out []ElevatableEntry
	add := func(key, category, action, description, origin string) {
		if seen[key] {
			return
		}
		seen[key] = true
		e := ElevatableEntry{Key: key, Category: category, Action: action, Description: description, Origin: origin, Source: ElevationSourceDefault}
		switch v, ok := overrides[key]; {
		case ok:
			e.Elevatable, e.Source = v, ElevationSourceAdmin
		case legacy[key]:
			e.Elevatable = true
		default:
			e.Elevatable = DefaultElevatable(key)
		}
		out = append(out, e)
	}
	for _, p := range CatalogSeed() {
		add(string(p.Key), p.Category, p.Action, p.Description, "host")
	}
	prows, err := db.QueryContext(ctx, `SELECT origin, permission_key, description FROM plugin_permissions ORDER BY origin, permission_key LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("reading plugin permissions: %w", err)
	}
	defer closeRows(prows)
	for prows.Next() {
		var origin, key, description string
		if err := prows.Scan(&origin, &key, &description); err != nil {
			return nil, fmt.Errorf("scanning plugin permission: %w", err)
		}
		category, action := splitKey(key)
		add(key, category, action, description, origin)
	}
	if err := prows.Err(); err != nil {
		return nil, fmt.Errorf("iterating plugin permissions: %w", err)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// IsElevatable reports whether key may be requested; found is false for a key
// neither the host nor any installed plugin defines.
func IsElevatable(ctx context.Context, db *sql.DB, key string) (elevatable, found bool, err error) {
	entries, err := ElevatableCatalog(ctx, db)
	if err != nil {
		return false, false, err
	}
	for _, e := range entries {
		if e.Key == key {
			return e.Elevatable, true, nil
		}
	}
	return false, false, nil
}

// SetElevatable records an admin's decision for key, which then outranks both
// the legacy flag and the default rule.
func SetElevatable(ctx context.Context, db *sql.DB, key string, elevatable bool, actorID string) error {
	flag := 0
	if elevatable {
		flag = 1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.ExecContext(ctx,
		`INSERT INTO permission_elevation (id, permission_key, elevatable, updated_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(permission_key) DO UPDATE SET elevatable = excluded.elevatable,
		   updated_by = excluded.updated_by, updated_at = excluded.updated_at`,
		xid.New().String(), key, flag, actorID, now, now)
	if err != nil {
		return fmt.Errorf("saving elevation override for %s: %w", key, err)
	}
	return nil
}

func elevationOverrides(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT permission_key, elevatable FROM permission_elevation LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("reading elevation overrides: %w", err)
	}
	defer closeRows(rows)
	out := map[string]bool{}
	for rows.Next() {
		var key string
		var flag int
		if err := rows.Scan(&key, &flag); err != nil {
			return nil, fmt.Errorf("scanning elevation override: %w", err)
		}
		out[key] = flag == 1
	}
	return out, rows.Err()
}
