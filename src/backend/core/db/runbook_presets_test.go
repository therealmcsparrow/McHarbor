// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"sort"
	"testing"
)

const runbookPresetMigration = "113_runbook_presets.sql"

func presetKeys(t *testing.T, database *sql.DB, preset string) []string {
	t.Helper()
	var raw string
	if err := database.QueryRow(`SELECT permissions FROM role_presets WHERE key = ?`, preset).Scan(&raw); err != nil {
		t.Fatalf("preset %s: %v", preset, err)
	}
	var keys []string
	if err := json.Unmarshal([]byte(raw), &keys); err != nil {
		t.Fatalf("preset %s permissions %q: %v", preset, raw, err)
	}
	sort.Strings(keys)
	return keys
}

func holds(keys []string, key string) bool {
	i := sort.SearchStrings(keys, key)
	return i < len(keys) && keys[i] == key
}

// TestRunbookPresetGrants pins the governance decision recorded in 07c: run and
// view to the operational presets, manage only beside alerts.manage, view alone
// to the read-only viewer, and nothing to presets with no on-call role.
func TestRunbookPresetGrants(t *testing.T) {
	database, err := openSQLite(filepath.Join(t.TempDir(), "presets.db"))
	if err != nil {
		t.Fatalf("opening sqlite: %v", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			t.Logf("closing database: %v", err)
		}
	}()
	if err := Migrate(database, DriverSQLite); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	want := map[string][]string{
		"container_operator":     {"runbooks.run", "runbooks.view"},
		"operator":               {"runbooks.manage", "runbooks.run", "runbooks.view"},
		"developer":              {"runbooks.run", "runbooks.view"},
		"platform_admin":         {"runbooks.manage", "runbooks.run", "runbooks.view"},
		"viewer":                 {"runbooks.view"},
		"auditor":                {},
		"security_admin":         {},
		"backup_manager":         {},
		"read_only_auditor":      {},
		"notification_recipient": {},
	}
	before := map[string][]string{}
	for preset, granted := range want {
		keys := presetKeys(t, database, preset)
		before[preset] = keys
		for _, k := range []string{"runbooks.view", "runbooks.run", "runbooks.manage"} {
			expected := false
			for _, g := range granted {
				expected = expected || g == k
			}
			if holds(keys, k) != expected {
				t.Errorf("preset %s: holds %s = %v, want %v", preset, k, !expected, expected)
			}
		}
		if holds(keys, "runbooks.manage") && !holds(keys, "alerts.manage") {
			t.Errorf("preset %s got runbooks.manage without alerts.manage", preset)
		}
	}

	body, err := MigrationSQL(runbookPresetMigration)
	if err != nil {
		t.Fatalf("reading %s: %v", runbookPresetMigration, err)
	}
	// A correlated guard would make MySQL skip the statement loudly, and the
	// grant would silently never reach a MySQL or MariaDB install.
	for _, driver := range []Driver{DriverPostgres, DriverMySQL, DriverMSSQL} {
		for _, stmt := range SplitStatements(body) {
			if got := ClassifyStatement(driver, stmt); got != ExecuteStatement {
				t.Errorf("%s would not execute on %s (action %d): %s", runbookPresetMigration, driver, got, firstLine(stmt))
			}
		}
	}
	if _, err := database.Exec(body); err != nil {
		t.Fatalf("re-applying %s: %v", runbookPresetMigration, err)
	}
	for preset, keys := range before {
		again := presetKeys(t, database, preset)
		seen := map[string]bool{}
		for _, k := range again {
			if seen[k] {
				t.Errorf("preset %s carries %s twice after a re-run", preset, k)
			}
			seen[k] = true
		}
		if len(again) != len(keys) {
			t.Errorf("preset %s changed size on a re-run: %d -> %d", preset, len(keys), len(again))
		}
	}
}
