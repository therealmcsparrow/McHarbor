// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package sqldialect

import (
	"database/sql/driver"
	"testing"
)

// TestRebind guards the one rewrite every Postgres query goes through: placeholders
// are numbered in order, and a `?` that is data or commentary is left alone.
func TestRebind(t *testing.T) {
	cases := map[string]string{
		"SELECT 1":                                   "SELECT 1",
		"SELECT * FROM t WHERE a = ? AND b = ?":      "SELECT * FROM t WHERE a = $1 AND b = $2",
		"UPDATE t SET note = 'why?' WHERE id = ?":    "UPDATE t SET note = 'why?' WHERE id = $1",
		"SELECT 'it''s ?' , ? FROM t":                "SELECT 'it''s ?' , $1 FROM t",
		"SELECT \"odd?col\" FROM t WHERE x = ?":      "SELECT \"odd?col\" FROM t WHERE x = $1",
		"SELECT ? -- trailing ?\n, ?":                "SELECT $1 -- trailing ?\n, $2",
		"SELECT /* a ? here */ ? FROM t":             "SELECT /* a ? here */ $1 FROM t",
		"INSERT INTO t (a, b, c) VALUES (?, ?, ?)":   "INSERT INTO t (a, b, c) VALUES ($1, $2, $3)",
		"SELECT * FROM t WHERE x IN (?,?,?) LIMIT ?": "SELECT * FROM t WHERE x IN ($1,$2,$3) LIMIT $4",
		"SELECT 'unterminated ? ":                    "SELECT 'unterminated ? ",
	}
	for in, want := range cases {
		if got := Rebind(in); got != want {
			t.Errorf("Rebind(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestPostgresTranslatesIdioms guards that runtime statements get the same SQLite
// idiom rewrites the migrations do, not only the placeholder change.
func TestPostgresTranslatesIdioms(t *testing.T) {
	got := Postgres("INSERT OR IGNORE INTO t (a) VALUES (?)")
	if got != "INSERT INTO t (a) VALUES ($1) ON CONFLICT DO NOTHING" {
		t.Fatalf("got %q", got)
	}
	if Postgres("SELECT * FROM t WHERE a = ?") != "SELECT * FROM t WHERE a = $1" {
		t.Fatal("memoised path must return the same translation")
	}
}

// TestPostgresRuntimeIdioms pins the translation of each SQLite idiom the application
// uses, and that translating twice changes nothing (migrations pass through twice).
func TestPostgresRuntimeIdioms(t *testing.T) {
	cases := map[string]string{
		"UPDATE t SET updated_at = datetime('now') WHERE id = ?":                                                    "UPDATE t SET updated_at = to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI:SS') WHERE id = $1",
		"DELETE FROM a WHERE julianday(ts) < julianday('now', '-' || ? || ' days')":                                 "DELETE FROM a WHERE (extract(epoch from (ts)::timestamptz) / 86400.0 + 2440587.5) < (extract(epoch from (now() + ('-' || $1 || ' days')::interval)) / 86400.0 + 2440587.5)",
		"SELECT GROUP_CONCAT(name, char(31)) FROM r":                                                                "SELECT string_agg((name)::text, chr(31)) FROM r",
		"SELECT (CAST(strftime('%s', occurred_at) AS INTEGER) / ?) * ? AS b":                                        "SELECT (CAST(extract(epoch from (occurred_at)::timestamptz)::bigint AS BIGINT) / $1) * $2 AS b",
		"ORDER BY kind, name COLLATE NOCASE LIMIT 1000":                                                             "ORDER BY kind, lower(name) LIMIT 1000",
		"INSERT OR REPLACE INTO workflow_kv (key, value, expires_at, updated_at) VALUES (?, ?, ?, datetime('now'))": "INSERT INTO workflow_kv (key, value, expires_at, updated_at) VALUES ($1, $2, $3, to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI:SS')) ON CONFLICT (key) DO UPDATE SET value = excluded.value, expires_at = excluded.expires_at, updated_at = excluded.updated_at",
		"SELECT note FROM t WHERE note = 'datetime(''now'')'":                                                       "SELECT note FROM t WHERE note = 'datetime(''now'')'",
	}
	for in, want := range cases {
		got := Postgres(in)
		if got != want {
			t.Errorf("Postgres(%q)\n got %q\nwant %q", in, got, want)
		}
		if again := Rebind(postgresRules(got)); again != got {
			t.Errorf("not idempotent: %q -> %q", got, again)
		}
	}
}

func TestPostgresBinds(t *testing.T) {
	args := []driver.NamedValue{{Ordinal: 1, Value: true}, {Ordinal: 2, Value: false}, {Ordinal: 3, Value: "x"}}
	PostgresArgs("", args)
	if args[0].Value != int64(1) || args[1].Value != int64(0) || args[2].Value != "x" {
		t.Fatalf("args = %+v", args)
	}
	if got := PostgresDSN("postgres://u@h/db?sslmode=disable"); got != "postgres://u@h/db?sslmode=disable&default_query_exec_mode=simple_protocol" {
		t.Fatalf("dsn = %s", got)
	}
	if got := PostgresDSN("postgres://u@h/db?default_query_exec_mode=exec"); got != "postgres://u@h/db?default_query_exec_mode=exec" {
		t.Fatal("an explicit mode must win")
	}
}
