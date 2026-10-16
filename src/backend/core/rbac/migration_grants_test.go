// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/plugin/types"
)

// pluginManifestsGlob finds every in-tree plugin manifest, relative to this
// package. It is a var, not a const, so a test can point it at a directory
// that does not exist and prove the fixture fallback below without moving
// the real plugin tree.
var pluginManifestsGlob = "../../../plugins/Plugins/*/plugin.yaml"

// pluginPermissionsFixture is the checked-in fallback pluginDeclaredKeys reads
// when the plugin tree is absent (src/plugins is untracked, so CI does not
// always have it). Regenerate with:
//
//	node scripts/sanity/missing-api/plugin-permissions.mjs
var pluginPermissionsFixture = "testdata/plugin-permissions.json"

// openMigratedSQLite runs every migration against a fresh on-disk sqlite file
// (modernc.org/sqlite does not support ":memory:" across the pool the way
// db.Migrate expects, so this follows the same t.TempDir() pattern the other
// db tests use) and returns the resulting connection.
func openMigratedSQLite(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "migration-grants.db")
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=60000&_foreign_keys=ON&_synchronous=NORMAL", path)
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("opening sqlite: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Logf("closing sqlite: %v", err)
		}
	})
	if err := db.Migrate(database, db.DriverSQLite); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return database
}

// tableHasPermissionsColumn reports whether table declares a column literally
// named "permissions". table is always a name this file's own
// permissionBearingTables() read from sqlite_master.
func tableHasPermissionsColumn(t *testing.T, ctx context.Context, database *sql.DB, table string) bool {
	t.Helper()
	rows, err := database.QueryContext(ctx, `PRAGMA table_info(`+db.QuoteIdent(db.DriverSQLite, table)+`)`) // SQL identifier: table, from sqlite_master
	if err != nil {
		t.Fatalf("PRAGMA table_info(%s): %v", table, err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scanning table_info(%s): %v", table, err)
		}
		if name == "permissions" {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating table_info(%s): %v", table, err)
	}
	return false
}

// permissionBearingTables returns every table with a column literally named
// "permissions" — role_presets and the legacy roles table today — found by
// introspecting the migrated schema rather than assuming table names.
func permissionBearingTables(t *testing.T, ctx context.Context, database *sql.DB) []string {
	t.Helper()
	rows, err := database.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		t.Fatalf("listing tables: %v", err)
	}
	defer closeRows(rows)

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scanning table name: %v", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating tables: %v", err)
	}

	var out []string
	for _, table := range tables {
		if tableHasPermissionsColumn(t, ctx, database, table) {
			out = append(out, table)
		}
	}
	if len(out) == 0 {
		t.Fatal("found no table with a permissions column; the migrated schema looks wrong")
	}
	sort.Strings(out)
	return out
}

// grantedKeys returns every distinct permission key present in any row of
// table's permissions JSON array, after every migration has run — so a key
// that one migration grants and a later migration retires never appears.
// table is always a name this file's own permissionBearingTables() produced.
func grantedKeys(t *testing.T, ctx context.Context, database *sql.DB, table string) map[string][]string {
	t.Helper()
	quoted := db.QuoteIdent(db.DriverSQLite, table) // SQL identifier: table, from permissionBearingTables()
	rows, err := database.QueryContext(ctx,
		`SELECT je.value FROM `+quoted+` AS t, json_each(t.permissions) AS je`)
	if err != nil {
		t.Fatalf("querying granted keys from %s: %v", table, err)
	}
	defer closeRows(rows)

	byKey := map[string][]string{}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			t.Fatalf("scanning %s grant: %v", table, err)
		}
		byKey[value] = append(byKey[value], table)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating %s grants: %v", table, err)
	}
	return byKey
}

// pluginPermissionManifest is the slice of plugin.yaml this test reads.
type pluginPermissionManifest struct {
	Slug      string                 `yaml:"slug"`
	AddsPerms []types.PermissionDecl `yaml:"adds_permissions"`
}

// pluginDeclaredKeys returns every permission key declared by an in-tree
// plugin's adds_permissions. When the plugin tree is absent it falls back to
// pluginPermissionsFixture, the checked-in snapshot of the same data — a
// missing tree must never look like a plugin that declares no permissions at
// all, or every key one of them grants (addon-swarm's ~30 swarm_* keys among
// them) reads as uncatalogued in CI, which never checks the tree out.
func pluginDeclaredKeys(t *testing.T) map[string]bool {
	t.Helper()
	manifests, err := filepath.Glob(pluginManifestsGlob)
	if err != nil {
		t.Fatalf("glob %s: %v", pluginManifestsGlob, err)
	}
	if len(manifests) == 0 {
		return pluginDeclaredKeysFromFixture(t)
	}
	keys := map[string]bool{}
	for _, m := range manifests {
		raw, err := os.ReadFile(m)
		if err != nil {
			t.Fatalf("read %s: %v", m, err)
		}
		var man pluginPermissionManifest
		if err := yaml.Unmarshal(raw, &man); err != nil {
			t.Errorf("parse %s: %v", m, err)
			continue
		}
		for _, decl := range man.AddsPerms {
			keys[decl.Key] = true
		}
	}
	return keys
}

// pluginDeclaredKeysFromFixture reads pluginPermissionsFixture — a sorted
// JSON array of permission keys — as the fallback source of truth when the
// plugin tree is not present in this checkout.
func pluginDeclaredKeysFromFixture(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(pluginPermissionsFixture)
	if err != nil {
		t.Fatalf("read %s: %v (regenerate with node scripts/sanity/missing-api/plugin-permissions.mjs)",
			pluginPermissionsFixture, err)
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("parse %s: %v", pluginPermissionsFixture, err)
	}
	keys := make(map[string]bool, len(list))
	for _, key := range list {
		keys[key] = true
	}
	return keys
}

// TestPluginPermissionsFixtureIsCurrent fails when pluginPermissionsFixture
// no longer matches a live scan of the plugin tree — the same drift
// TestMigrationGrantsAreCatalogued would otherwise never see, because it only
// reads the fixture when the tree is absent. It is a no-op when the tree is
// absent, since there is then nothing live to compare the fixture against.
func TestPluginPermissionsFixtureIsCurrent(t *testing.T) {
	manifests, err := filepath.Glob(pluginManifestsGlob)
	if err != nil {
		t.Fatalf("glob %s: %v", pluginManifestsGlob, err)
	}
	if len(manifests) == 0 {
		t.Skip("plugin tree not present; nothing to compare the fixture against")
	}

	live := map[string]bool{}
	for _, m := range manifests {
		raw, err := os.ReadFile(m)
		if err != nil {
			t.Fatalf("read %s: %v", m, err)
		}
		var man pluginPermissionManifest
		if err := yaml.Unmarshal(raw, &man); err != nil {
			t.Fatalf("parse %s: %v", m, err)
		}
		for _, decl := range man.AddsPerms {
			live[decl.Key] = true
		}
	}
	liveList := make([]string, 0, len(live))
	for key := range live {
		liveList = append(liveList, key)
	}
	sort.Strings(liveList)

	raw, err := os.ReadFile(pluginPermissionsFixture)
	if err != nil {
		t.Fatalf("read %s: %v", pluginPermissionsFixture, err)
	}
	var fixture []string
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("parse %s: %v", pluginPermissionsFixture, err)
	}

	if len(liveList) != len(fixture) {
		t.Fatalf("%s is stale (%d keys checked in, %d live); regenerate with "+
			"node scripts/sanity/missing-api/plugin-permissions.mjs", pluginPermissionsFixture, len(fixture), len(liveList))
	}
	for i := range liveList {
		if liveList[i] != fixture[i] {
			t.Fatalf("%s is stale at entry %d (%q checked in, %q live); regenerate with "+
				"node scripts/sanity/missing-api/plugin-permissions.mjs", pluginPermissionsFixture, i, fixture[i], liveList[i])
		}
	}
}

// TestMigrationGrantsAreCatalogued fails when, after every migration has run,
// a role or role preset holds a permission key that exists nowhere: not in
// CatalogSeed(), not registered through the RegisterPermissions seam, and not
// declared by any in-tree plugin. Such a key denies every role that was
// meant to hold it, silently, because the role editor has never heard of it.
// Reading the final migrated state (rather than every string any migration
// ever wrote) means a key a later migration retires is correctly not flagged.
func TestMigrationGrantsAreCatalogued(t *testing.T) {
	ctx := t.Context()
	known := map[string]bool{}
	for _, e := range CatalogSeed() {
		known[string(e.Key)] = true
	}
	for _, e := range registeredPermissions() {
		known[string(e.Key)] = true
	}
	for key := range pluginDeclaredKeys(t) {
		known[key] = true
	}

	database := openMigratedSQLite(t)
	var uncatalogued []string
	for _, table := range permissionBearingTables(t, ctx, database) {
		for key, tables := range grantedKeys(t, ctx, database, table) {
			if key == "*" || known[key] {
				continue
			}
			sort.Strings(tables)
			uncatalogued = append(uncatalogued, key+" (held by "+tables[0]+")")
		}
	}
	sort.Strings(uncatalogued)
	if len(uncatalogued) > 0 {
		t.Errorf("%d granted permission keys are in no catalog after migration:\n  %s",
			len(uncatalogued), strings.Join(uncatalogued, "\n  "))
	}
}
