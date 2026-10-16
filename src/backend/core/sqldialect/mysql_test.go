// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package sqldialect

import (
	"database/sql/driver"
	"net/url"
	"strings"
	"testing"
)

// TestMySQLQuotesReservedNamesExactly pins the reserved-word quoting: a word is
// backticked where it names something and left alone where it is the keyword.
func TestMySQLQuotesReservedNamesExactly(t *testing.T) {
	cases := map[string]string{
		"INSERT INTO permission_catalog (id, key, category) VALUES (?, ?, ?)":            "INSERT INTO permission_catalog (id, `key`, category) VALUES (?, ?, ?)",
		"SELECT value FROM settings WHERE key = ?":                                       "SELECT value FROM settings WHERE `key` = ?",
		"SELECT g.id FROM groups g JOIN group_roles gr ON gr.group_id = g.id":            "SELECT g.id FROM `groups` g JOIN group_roles gr ON gr.group_id = g.id",
		"INSERT INTO groups(id, name) VALUES (?, ?)":                                     "INSERT INTO `groups`(id, name) VALUES (?, ?)",
		"SELECT s.key, s.value FROM settings s ORDER BY s.key":                           "SELECT s.`key`, s.value FROM settings s ORDER BY s.`key`",
		"UPDATE alert_rules SET condition = ?, updated_at = ? WHERE id = ?":              "UPDATE alert_rules SET `condition` = ?, updated_at = ? WHERE id = ?",
		"SELECT id FROM t ORDER BY name":                                                 "SELECT id FROM t ORDER BY name",
		"SELECT id, trigger, system FROM workflows":                                      "SELECT id, `trigger`, `system` FROM workflows",
		"SELECT RANK() OVER (ORDER BY id) FROM t":                                        "SELECT RANK() OVER (ORDER BY id) FROM t",
		"SELECT DATE_ADD(x, INTERVAL 1 DAY) FROM t":                                      "SELECT DATE_ADD(x, INTERVAL 1 DAY) FROM t",
		"SELECT interval, key FROM schedules":                                            "SELECT `interval`, `key` FROM schedules",
		"INSERT INTO t (a) VALUES (?) ON DUPLICATE KEY UPDATE a = VALUES(a)":             "INSERT INTO t (a) VALUES (?) ON DUPLICATE KEY UPDATE a = VALUES(a)",
		"SELECT 'key = order' AS note FROM settings WHERE key = 'groups'":                "SELECT 'key = order' AS note FROM settings WHERE `key` = 'groups'",
		"SELECT \"key\" FROM settings -- key\n WHERE `key` = ?":                          "SELECT \"key\" FROM settings -- key\n WHERE `key` = ?",
		"CREATE TABLE x (id TEXT PRIMARY KEY, key TEXT)":                                 "CREATE TABLE x (id TEXT PRIMARY KEY, key TEXT)",
		"INSERT INTO plugin_cli_commands (id, short, long, example) VALUES (?, ?, ?, ?)": "INSERT INTO plugin_cli_commands (id, short, `long`, example) VALUES (?, ?, ?, ?)",
	}
	for in, want := range cases {
		got := quoteReservedMySQL(in)
		if got != want {
			t.Errorf("quote(%q)\n got %q\nwant %q", in, got, want)
		}
		if again := quoteReservedMySQL(got); again != got {
			t.Errorf("not idempotent: %q -> %q", got, again)
		}
	}
}

// TestMySQLStatementRules pins the statement-shape rewrites and their exact output.
func TestMySQLStatementRules(t *testing.T) {
	cases := map[string]string{
		"SELECT a FROM t WHERE env IS ? AND day >= ?":                      "SELECT a FROM t WHERE env <=> ? AND day >= ?",
		"SELECT a FROM t WHERE env IS NULL":                                "SELECT a FROM t WHERE env IS NULL",
		"SELECT CAST(x AS TEXT), CAST(y AS INTEGER)":                       "SELECT CAST(x AS CHAR), CAST(y AS SIGNED)",
		"SELECT CAST(AVG(CAST(m AS FLOAT)) AS BIGINT) FROM t":              "SELECT CAST(AVG(CAST(m AS DOUBLE)) AS SIGNED) FROM t",
		"SELECT x AS text FROM t":                                          "SELECT x AS text FROM t",
		"SELECT id FROM (SELECT id FROM t) WHERE 1":                        "SELECT id FROM (SELECT id FROM t) AS mch_d1 WHERE 1",
		"SELECT id FROM (SELECT id FROM t) s":                              "SELECT id FROM (SELECT id FROM t) s",
		"DELETE FROM e WHERE id IN (SELECT id FROM e ORDER BY ts LIMIT ?)": "DELETE FROM e WHERE id IN (SELECT * FROM (SELECT id FROM e ORDER BY ts LIMIT ?) AS mch_d1)",
		"SELECT a FROM t WHERE id IN (SELECT id FROM u)":                   "SELECT a FROM t WHERE id IN (SELECT id FROM u)",
		"SELECT 'x IS ?' FROM t":                                           "SELECT 'x IS ?' FROM t",
		"UPDATE t SET a = 1 WHERE id IN (SELECT id FROM t WHERE b = ?)":    "UPDATE t SET a = 1 WHERE id IN (SELECT * FROM (SELECT id FROM t WHERE b = ?) AS mch_d1)",
	}
	for in, want := range cases {
		got := mysqlStatementRules(in)
		if got != want {
			t.Errorf("rules(%q)\n got %q\nwant %q", in, got, want)
		}
	}
}

// TestMySQLUpsert pins the ON CONFLICT translations.
func TestMySQLUpsert(t *testing.T) {
	cases := map[string]string{
		"INSERT INTO s (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = ?": "INSERT INTO s (key, value) VALUES (?, ?) ON DUPLICATE KEY UPDATE value = VALUES(value), updated_at = ?",
		"INSERT INTO s (id, name) VALUES (?, ?) ON CONFLICT (id) DO NOTHING":                                             "INSERT INTO s (id, name) VALUES (?, ?) ON DUPLICATE KEY UPDATE id = id",
		"INSERT INTO s (id) VALUES ('on conflict do nothing')":                                                           "INSERT INTO s (id) VALUES ('on conflict do nothing')",
	}
	for in, want := range cases {
		if got := mysqlUpsert(in); got != want {
			t.Errorf("upsert(%q)\n got %q\nwant %q", in, got, want)
		}
	}
}

// TestMySQLFullTranslation runs whole statements through MySQL(), including the
// GROUP_CONCAT shapes the user and group lists use, and checks idempotency.
func TestMySQLFullTranslation(t *testing.T) {
	in := "SELECT g.id, (SELECT GROUP_CONCAT(name, char(31)) FROM (SELECT DISTINCT r.name AS name FROM group_roles gr JOIN roles r ON r.id = gr.role_id WHERE gr.group_id = g.id)) AS role_names FROM groups g"
	want := "SELECT g.id, (SELECT GROUP_CONCAT(DISTINCT r.name SEPARATOR '\x1f') FROM group_roles gr JOIN roles r ON r.id = gr.role_id WHERE gr.group_id = g.id) AS role_names FROM `groups` g"
	got := MySQL(in)
	if got != want {
		t.Fatalf("MySQL()\n got %q\nwant %q", got, want)
	}
	if again := MySQL(got); again != got {
		t.Fatalf("not idempotent: %q", again)
	}
}

// TestMySQLArgsOnlyTouchDatetimeColumns guards the RFC3339 conversion: a stamp bound
// to a DATETIME column becomes UTC wall-clock time, the same string bound to a text
// column is left byte for byte, and non-RFC3339 strings are never touched.
func TestMySQLArgsOnlyTouchDatetimeColumns(t *testing.T) {
	setTimeRegistry(timeRegistry{
		"cluster_state":    {"created_at": {precision: 0}},
		"audit_logs":       {"timestamp": {precision: 6}},
		"license_installs": {"created_at": {}},
		"days":             {"day": {dateOnly: true}},
	})
	t.Cleanup(func() { setTimeRegistry(nil) })
	stamp := "2026-09-26T22:50:04.987654321+02:00"
	run := func(q string, vals ...any) []any {
		args := make([]driver.NamedValue, len(vals))
		for i, v := range vals {
			args[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
		}
		MySQLArgs(q, args)
		out := make([]any, len(args))
		for i := range args {
			out[i] = args[i].Value
		}
		return out
	}
	got := run(`INSERT INTO cluster_state (id, name, value, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, "x", "n", stamp, stamp, stamp)
	if got[2] != stamp || got[3] != "2026-09-26 20:50:04" || got[4] != stamp {
		t.Fatalf("insert args = %v", got)
	}
	got = run(`SELECT id FROM audit_logs a WHERE a.timestamp >= ? AND a.details = ?`, stamp, stamp)
	if got[0] != "2026-09-26 20:50:04.987654" || got[1] != stamp {
		t.Fatalf("where args = %v", got)
	}
	got = run(`SELECT id FROM audit_logs WHERE timestamp BETWEEN ? AND ?`, stamp, stamp)
	if got[0] != "2026-09-26 20:50:04.987654" || got[1] != "2026-09-26 20:50:04.987654" {
		t.Fatalf("between args = %v", got)
	}
	got = run(`INSERT INTO license_installs (id, created_at) SELECT ?, ? WHERE NOT EXISTS (SELECT 1 FROM license_installs)`, "i", stamp)
	if got[1] != "2026-09-26 20:50:04" {
		t.Fatalf("insert-select args = %v", got)
	}
	got = run(`SELECT 1 FROM days WHERE day IN (?, ?)`, stamp, "2026-09-26")
	if got[0] != "2026-09-26" || got[1] != "2026-09-26" {
		t.Fatalf("in args = %v", got)
	}
	got = run(`UPDATE cluster_state SET created_at = ? WHERE name = ?`, "not a time", stamp)
	if got[0] != "not a time" || got[1] != stamp {
		t.Fatalf("non-time args = %v", got)
	}
}

// TestMySQLDSN guards the per-connection session settings.
func TestMySQLDSN(t *testing.T) {
	got := MySQLDSN("u:p@tcp(h:3306)/db?parseTime=false&loc=Europe%2FAmsterdam")
	base, query, _ := strings.Cut(got, "?")
	if base != "u:p@tcp(h:3306)/db" {
		t.Fatalf("base = %s", base)
	}
	v, err := url.ParseQuery(query)
	if err != nil {
		t.Fatal(err)
	}
	if v.Get("parseTime") != "true" || v.Get("loc") != "Europe/Amsterdam" || v.Get("time_zone") != "'+00:00'" ||
		!strings.Contains(v.Get("sql_mode"), "NO_BACKSLASH_ESCAPES") || v.Get("charset") != "utf8mb4" {
		t.Fatalf("params = %v", v)
	}
}
