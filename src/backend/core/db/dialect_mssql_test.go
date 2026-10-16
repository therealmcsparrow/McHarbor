// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"strings"
	"testing"
)

// TestMSSQLTranslationRules pins each rule that would silently produce wrong
// data rather than a syntax error. A mistranslation that still parses is the
// failure mode worth a test; one that does not parse fails at apply time on its
// own.
func TestMSSQLTranslationRules(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   []string
		absent []string
	}{
		{
			name: "create table if not exists becomes an OBJECT_ID predicate",
			in:   "CREATE TABLE IF NOT EXISTS widgets (id TEXT PRIMARY KEY, label TEXT);",
			want: []string{"IF OBJECT_ID(N'widgets', N'U') IS NULL CREATE TABLE widgets"},
			// SQL Server has no IF NOT EXISTS on DDL at all.
			absent: []string{"IF NOT EXISTS"},
		},
		{
			name: "a keyed text column is narrowed and an unkeyed one is not",
			in:   "CREATE TABLE IF NOT EXISTS widgets (\n  id TEXT PRIMARY KEY,\n  payload TEXT\n);",
			want: []string{"id NVARCHAR(450)", "payload NVARCHAR(MAX)"},
		},
		{
			name: "integer becomes BIGINT and boolean does too, never BIT",
			in:   "CREATE TABLE IF NOT EXISTS widgets (\n  id TEXT PRIMARY KEY,\n  count INTEGER,\n  enabled BOOLEAN,\n  ratio REAL,\n  raw BLOB\n);",
			want: []string{"count BIGINT", "enabled BIGINT", "ratio FLOAT", "raw VARBINARY(MAX)"},
			// A BIT column hands database/sql a Go bool, which will not scan
			// into the int every flag in this application is read as.
			absent: []string{"BIT"},
		},
		{
			name: "the timestamp default keeps producing RFC3339 text",
			in:   "CREATE TABLE IF NOT EXISTS widgets (\n  id TEXT PRIMARY KEY,\n  created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)\n);",
			want: []string{"CONVERT(nvarchar(23), SYSUTCDATETIME(), 126) + 'Z'"},
			// A bare CURRENT_TIMESTAMP would store SQL Server's own datetime
			// rendering, which is not RFC3339 and which every reader parses.
			absent: []string{"DEFAULT CURRENT_TIMESTAMP", "DEFAULT (CURRENT_TIMESTAMP)"},
		},
		{
			name:   "datetime('now', '-1 day') keeps its offset",
			in:     "DELETE FROM sessions WHERE created_at < datetime('now', '-1 day');",
			want:   []string{"DATEADD(day, -1, SYSUTCDATETIME())"},
			absent: []string{"datetime('now'"},
		},
		{
			name: "a trailing LIMIT becomes a TOP on the SELECT",
			in:   "SELECT id FROM users ORDER BY created_at ASC LIMIT 1;",
			want: []string{"SELECT TOP (1) id FROM users ORDER BY created_at ASC;"},
			// OFFSET/FETCH would need an ORDER BY that several of these
			// subqueries do not have.
			absent: []string{"LIMIT", "FETCH NEXT"},
		},
		{
			name: "concatenation outside a literal becomes + and inside one does not",
			in:   "INSERT INTO t (id, expr) SELECT a.id || ':' || b.id, 'x || y' FROM a, b;",
			want: []string{"a.id + ':' + b.id", "'x || y'"},
		},
		{
			name: "ON CONFLICT DO NOTHING becomes a NOT EXISTS over the source rows",
			in:   "INSERT INTO groups (id, name)\nVALUES ('g1', 'one'), ('g2', 'two')\nON CONFLICT (id) DO NOTHING;",
			want: []string{
				"FROM (VALUES ('g1', 'one'), ('g2', 'two')) AS src ([id], [name])",
				"WHERE NOT EXISTS (SELECT 1 FROM [groups] AS tgt WHERE tgt.[id] = src.[id])",
			},
			// Swallowing a duplicate-key error instead would abort the whole
			// multi-row insert and lose the rows that were new.
			absent: []string{"ON CONFLICT"},
		},
		{
			name: "ON CONFLICT DO UPDATE becomes a MERGE and excluded becomes the source",
			in:   "INSERT INTO retention (id, name)\nVALUES ('a', 'A')\nON CONFLICT(id) DO UPDATE SET\n    name = excluded.name;",
			want: []string{
				"MERGE [retention] AS tgt",
				"USING (VALUES ('a', 'A')) AS src ([id], [name])",
				"ON tgt.[id] = src.[id]",
				"WHEN MATCHED THEN UPDATE SET name = src.name",
				"WHEN NOT MATCHED THEN INSERT ([id], [name]) VALUES (src.[id], src.[name])",
			},
			absent: []string{"ON CONFLICT", "excluded."},
		},
		{
			name: "an INSERT OR IGNORE on settings resolves on key, not on its random id",
			in:   "INSERT OR IGNORE INTO settings (id, key, value) VALUES (lower(hex(randomblob(8))), 'a', 'b');",
			// Resolving on id would insert a duplicate setting on every run,
			// because the id is minted fresh each time.
			want:   []string{"tgt.[key] = src.[key]"},
			absent: []string{"tgt.[id] = src.[id]", "INSERT OR IGNORE"},
		},
		{
			name:   "json_each becomes OPENJSON, whose value column keeps its name",
			in:     "SELECT value FROM json_each(roles.permissions);",
			want:   []string{"OPENJSON(COALESCE(NULLIF(roles.permissions, ''), '[]'))"},
			absent: []string{"json_each"},
		},
		{
			name:   "GLOB becomes a LIKE with the same character class",
			in:     "SELECT id FROM lifecycle_events WHERE timestamp GLOB '[0-9]*';",
			want:   []string{"LIKE '[0-9]%'"},
			absent: []string{"GLOB"},
		},
		{
			name:   "hex(randomblob(n)) produces 2n hex characters",
			in:     "INSERT INTO t (id) VALUES (lower(hex(randomblob(8))));",
			want:   []string{"CONVERT(nvarchar(16), CRYPT_GEN_RANDOM(8), 2)"},
			absent: []string{"randomblob"},
		},
		{
			name: "a DROP TABLE drops the keys that reference it first",
			in:   "DROP TABLE users_old;",
			// SQL Server has no DROP TABLE CASCADE, and every DROP in this
			// corpus closes a table rebuild whose old table is still referenced.
			want: []string{"sys.foreign_keys", "DROP CONSTRAINT", "DROP TABLE users_old;"},
		},
		{
			name: "a reserved word used as a column name is bracketed",
			in:   "CREATE TABLE IF NOT EXISTS settings (\n  id TEXT PRIMARY KEY,\n  key TEXT NOT NULL UNIQUE\n);",
			want: []string{"[key] NVARCHAR(450)"},
		},
		{
			name: "PRIMARY KEY is not bracketed just because KEY is reserved",
			in:   "CREATE TABLE IF NOT EXISTS widgets (\n  id TEXT PRIMARY KEY\n);",
			want: []string{"PRIMARY KEY"},
			// The first attempt at the MySQL quoter backticked exactly this.
			absent: []string{"PRIMARY [key]"},
		},
		{
			name: "CAST targets SQL Server does not know are renamed",
			in:   "SELECT CAST(count AS INTEGER), CAST(id AS TEXT) FROM t;",
			want: []string{"CAST(count AS BIGINT)", "CAST(id AS NVARCHAR(MAX))"},
		},
		// The rules below were each the first failure of a real SQL Server 2022
		// run (2026-09-27); none of them had been applied to a server before.
		{
			name:   "ADD COLUMN loses the COLUMN keyword",
			in:     "ALTER TABLE host_metrics ADD COLUMN net_rx INTEGER;",
			want:   []string{"ALTER TABLE host_metrics ADD net_rx BIGINT;"},
			absent: []string{"COLUMN"},
		},
		{
			name:   "RENAME TO becomes sp_rename",
			in:     "ALTER TABLE environments_new RENAME TO environments;",
			want:   []string{"EXEC sp_rename N'environments_new', N'environments';"},
			absent: []string{"RENAME TO"},
		},
		{
			name:   "a LIMIT inside a subquery becomes a TOP on that subquery's SELECT",
			in:     "UPDATE environments SET is_default = 0 WHERE is_default = 1 AND id NOT IN (SELECT id FROM environments WHERE is_default = 1 ORDER BY id ASC LIMIT 1);",
			want:   []string{"UPDATE environments SET", "(SELECT TOP (1) id FROM environments WHERE is_default = 1 ORDER BY id ASC)"},
			absent: []string{"LIMIT"},
		},
		{
			name:   "CREATE INDEX IF NOT EXISTS becomes a sys.indexes predicate, not a tolerated error",
			in:     "CREATE UNIQUE INDEX IF NOT EXISTS idx_widgets_label ON widgets(label);",
			want:   []string{"IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'idx_widgets_label' AND object_id = OBJECT_ID(N'widgets')) CREATE UNIQUE INDEX idx_widgets_label ON widgets(label)"},
			absent: []string{"INDEX IF NOT EXISTS"},
		},
		{
			name: "DROP COLUMN drops the system-named DEFAULT and CHECK constraints first",
			in:   "ALTER TABLE environments DROP COLUMN k8s_namespace;",
			want: []string{"sys.default_constraints", "sys.check_constraints", "OBJECT_ID(N'environments') AND c.name = N'k8s_namespace'",
				"EXEC sp_executesql @drop_cc; ALTER TABLE environments DROP COLUMN k8s_namespace;"},
		},
		{
			name:   "length() becomes LEN()",
			in:     "CREATE TABLE lifecycle_events_new (\n  source TEXT NOT NULL DEFAULT 'docker' CHECK(length(source) > 0)\n);",
			want:   []string{"CHECK(LEN(source) > 0)"},
			absent: []string{"length("},
		},
		{
			name: "an unnamed derived table gets an alias",
			in:   "UPDATE roles SET permissions = (SELECT json_group_array(value) FROM (SELECT value FROM json_each(roles.permissions) UNION SELECT value FROM json_each('[]'))) WHERE name = 'Operator';",
			want: []string{") AS mch_d1) WHERE name"},
		},
		{
			name:   "a derived table that already has an alias keeps it",
			in:     "SELECT n FROM (SELECT 1 AS n) AS src WHERE n = 1;",
			absent: []string{"mch_d"},
		},
		{
			name:   "a bare CURRENT_TIMESTAMP value is written as RFC3339 text",
			in:     "UPDATE roles SET updated_at = CURRENT_TIMESTAMP WHERE id = 'x';",
			want:   []string{"updated_at = (CONVERT(nvarchar(23), SYSUTCDATETIME(), 126) + 'Z')"},
			absent: []string{"CURRENT_TIMESTAMP"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := translateMSSQL(tc.in)
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q in:\n%s", want, got)
				}
			}
			for _, absent := range tc.absent {
				if strings.Contains(got, absent) {
					t.Errorf("%q should not survive translation:\n%s", absent, got)
				}
			}
		})
	}
}

// TestMSSQLClassificationKeepsTheFileApplying checks the classifier decisions
// whose absence would fail a whole migration file rather than one statement.
func TestMSSQLClassificationKeepsTheFileApplying(t *testing.T) {
	cases := []struct {
		name string
		stmt string
		want StatementAction
	}{
		{"PRAGMA is skipped", "PRAGMA journal_mode = WAL;", SkipStatement},
		{"the foreign-key PRAGMA is skipped, there being no global switch",
			"PRAGMA foreign_keys = OFF;", SkipStatement},
		{"a WORM trigger is skipped loudly, since RAISE(ABORT) has no equivalent",
			"CREATE TRIGGER audit_no_update BEFORE UPDATE ON audit_log BEGIN SELECT RAISE(ABORT, 'no'); END;",
			SkipStatementLoudly},
		{"CREATE INDEX IF NOT EXISTS relies on tolerating the duplicate",
			"CREATE INDEX IF NOT EXISTS idx_a ON t(a);", TolerateDuplicate},
		{"DROP INDEX IF EXISTS relies on tolerating the absence",
			"DROP INDEX IF EXISTS idx_a;", TolerateMissing},
		{"an ordinary CREATE TABLE executes",
			"CREATE TABLE IF NOT EXISTS t (id TEXT PRIMARY KEY);", ExecuteStatement},
		{"a transaction statement is skipped so it cannot break the outer rollback",
			"BEGIN TRANSACTION;", SkipStatement},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyStatement(DriverMSSQL, tc.stmt); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestMSSQLMigrationPlaceholdersAreNamed guards the one thing that fails on
// every single statement if it is wrong: SQL Server binds @p1, not ? and not $1.
func TestMSSQLMigrationPlaceholdersAreNamed(t *testing.T) {
	if got := migrationLookupSQL(DriverMSSQL); !strings.Contains(got, "@p1") {
		t.Errorf("lookup must bind @p1, got %q", got)
	}
	if got := migrationRecordSQL(DriverMSSQL); !strings.Contains(got, "@p1") {
		t.Errorf("record must bind @p1, got %q", got)
	}
	for _, other := range []Driver{DriverSQLite, DriverMySQL} {
		if got := migrationLookupSQL(other); strings.Contains(got, "@p1") {
			t.Errorf("%s must not bind @p1, got %q", other, got)
		}
	}
}
