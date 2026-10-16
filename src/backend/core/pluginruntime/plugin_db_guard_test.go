// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pluginruntime

import (
	"errors"
	"testing"

	plugin "github.com/therealmcsparrow/mcharbor/core/plugin"
)

// TestPluginDBGuardRejectsAHostTable covers the shapes the containment test let
// through. Each one mentions the plugin's prefix somewhere and touches a table
// that is not the plugin's, which is exactly the case the guard exists for.
func TestPluginDBGuardRejectsAHostTable(t *testing.T) {
	const prefix = "plugin_foo_"

	refused := []struct {
		name  string
		query string
	}{
		{"a line comment carrying the prefix",
			"DELETE FROM users -- plugin_foo_\n"},
		{"a block comment carrying the prefix",
			"UPDATE users SET is_admin = 1 /* plugin_foo_ */"},
		{"a trailing statement after a legitimate one",
			"SELECT * FROM plugin_foo_items; DROP TABLE users"},
		{"a join onto a host table",
			"SELECT * FROM plugin_foo_items JOIN users ON users.id = plugin_foo_items.user_id"},
		{"an insert into a host table",
			"INSERT INTO user_roles (id) SELECT id FROM plugin_foo_items"},
		{"a DDL against a host table",
			"ALTER TABLE users ADD COLUMN plugin_foo_x TEXT"},
		{"a prefix that appears only in a string literal",
			"DELETE FROM secrets WHERE name = 'plugin_foo_key'"},
		{"no table and no prefix at all",
			"SELECT 1"},
	}

	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			if err := pluginDBExec(prefix, tc.query); !errors.Is(err, plugin.ErrScopeViolation) {
				t.Errorf("must be refused, got %v", err)
			}
		})
	}
}

// TestPluginDBGuardAllowsTheProperlyScopedStatements: a guard that refuses the
// plugin's own legitimate SQL is a guard people work around.
func TestPluginDBGuardAllowsTheProperlyScopedStatements(t *testing.T) {
	const prefix = "plugin_foo_"

	allowed := []struct {
		name  string
		query string
	}{
		{"a plain select", "SELECT id, name FROM plugin_foo_items WHERE id = ?"},
		{"an insert", "INSERT INTO plugin_foo_items (id, name) VALUES (?, ?)"},
		{"an update", "UPDATE plugin_foo_items SET name = ? WHERE id = ?"},
		{"a delete", "DELETE FROM plugin_foo_items WHERE id = ?"},
		{"a create with IF NOT EXISTS",
			"CREATE TABLE IF NOT EXISTS plugin_foo_items (id TEXT PRIMARY KEY)"},
		{"a join between two of its own tables",
			"SELECT * FROM plugin_foo_items i JOIN plugin_foo_runs r ON r.item_id = i.id"},
		{"a quoted identifier", `SELECT * FROM "plugin_foo_items"`},
		{"a backticked identifier", "SELECT * FROM `plugin_foo_items`"},
		{"a bracketed identifier", "SELECT * FROM [plugin_foo_items]"},
		{"a derived table over its own table",
			"SELECT COUNT(*) FROM (SELECT id FROM plugin_foo_items) AS src"},
		{"a comment of its own", "-- reap old rows\nDELETE FROM plugin_foo_runs WHERE created_at < ?"},
	}

	for _, tc := range allowed {
		t.Run(tc.name, func(t *testing.T) {
			if err := pluginDBExec(prefix, tc.query); err != nil {
				t.Errorf("must be allowed, got %v", err)
			}
		})
	}
}

// TestStripSQLCommentsKeepsLiterals: a `--` inside a string is data, and
// stripping it would change the statement rather than only its commentary.
func TestStripSQLCommentsKeepsLiterals(t *testing.T) {
	cases := []struct{ in, want string }{
		{"SELECT '--not a comment' FROM t", "SELECT '--not a comment' FROM t"},
		{"SELECT '/* nor this */' FROM t", "SELECT '/* nor this */' FROM t"},
		{"SELECT 1 -- gone\nSELECT 2", "SELECT 1 \nSELECT 2"},
		{"SELECT /* gone */ 1", "SELECT   1"},
	}
	for _, tc := range cases {
		if got := stripSQLComments(tc.in); got != tc.want {
			t.Errorf("stripSQLComments(%q)\n got %q\nwant %q", tc.in, got, tc.want)
		}
	}
}

// TestPluginDBGuardHonoursADeclaredPrefix: a plugin may declare its own table
// prefix, and the guard must use that rather than the slug-derived default.
func TestPluginDBGuardHonoursADeclaredPrefix(t *testing.T) {
	if got := pluginTablePrefix("addon-kvm", ""); got != "plugin_addon_kvm_" {
		t.Errorf("the default prefix must replace hyphens, got %q", got)
	}
	if got := pluginTablePrefix("addon-kvm", "vm_"); got != "vm_" {
		t.Errorf("a declared prefix must win, got %q", got)
	}
	if err := pluginDBExec("vm_", "SELECT * FROM vm_guests"); err != nil {
		t.Errorf("a declared prefix must be honoured: %v", err)
	}
	if err := pluginDBExec("vm_", "SELECT * FROM users"); !errors.Is(err, plugin.ErrScopeViolation) {
		t.Errorf("a declared prefix must still scope, got %v", err)
	}
}
