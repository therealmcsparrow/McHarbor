// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package sqldialect

import (
	"database/sql/driver"
	"net/url"
	"strings"
	"testing"
)

// check is the shared assertion: every rule below is pinned on the translated text,
// because the only thing this package produces is text and the only way to be wrong is
// to produce different text.
func check(t *testing.T, name string, fn func(string) string, cases map[string]string) {
	t.Helper()
	for in, want := range cases {
		if got := fn(in); got != want {
			t.Errorf("%s(%q)\n got %q\nwant %q", name, in, got, want)
		}
	}
}

// TestRebindMSSQLNumbersParameters guards the placeholder form go-mssqldb derives from
// each ordinal, and that a `?` which is data survives untouched.
func TestRebindMSSQLNumbersParameters(t *testing.T) {
	check(t, "rebindWith", func(q string) string { return rebindWith(q, "@p") }, map[string]string{
		"SELECT * FROM t WHERE a = ? AND b = ?":      "SELECT * FROM t WHERE a = @p1 AND b = @p2",
		"UPDATE t SET note = 'why?' WHERE id = ?":    "UPDATE t SET note = 'why?' WHERE id = @p1",
		"SELECT 'it''s ?' , ? FROM t":                "SELECT 'it''s ?' , @p1 FROM t",
		"SELECT \"odd?col\" FROM t WHERE x = ?":      "SELECT \"odd?col\" FROM t WHERE x = @p1",
		"SELECT ? -- trailing ?\n, ?":                "SELECT @p1 -- trailing ?\n, @p2",
		"SELECT /* a ? here */ ? FROM t":             "SELECT /* a ? here */ @p1 FROM t",
		"SELECT * FROM t WHERE x IN (?,?,?) LIMIT ?": "SELECT * FROM t WHERE x IN (@p1,@p2,@p3) LIMIT @p4",
		"SELECT 1": "SELECT 1",
	})
	// Rebind's own numbering must not have moved when the prefix became a parameter.
	if got := Rebind("SELECT ?, ?"); got != "SELECT $1, $2" {
		t.Fatalf("Rebind regressed: %q", got)
	}
}

// TestMSSQLLimitRules pins both LIMIT translations. OFFSET/FETCH reverses the operands
// relative to LIMIT, so the parameter names have to travel with their values; TOP is used
// where there is no ORDER BY for FETCH to require.
func TestMSSQLLimitRules(t *testing.T) {
	check(t, "mssqlLimitRules", mssqlLimitRules, map[string]string{
		"SELECT a FROM t LIMIT 50":                        "SELECT TOP (50) a FROM t",
		"SELECT a FROM t ORDER BY a LIMIT @p1":            "SELECT TOP (@p1) a FROM t ORDER BY a",
		"SELECT DISTINCT a FROM t LIMIT 10":               "SELECT DISTINCT TOP (10) a FROM t",
		"SELECT a FROM t ORDER BY a LIMIT @p1 OFFSET @p2": "SELECT a FROM t ORDER BY a OFFSET @p2 ROWS FETCH NEXT @p1 ROWS ONLY",
		"SELECT a FROM t ORDER BY a LIMIT 25 OFFSET 50":   "SELECT a FROM t ORDER BY a OFFSET 50 ROWS FETCH NEXT 25 ROWS ONLY",
		// A nested LIMIT belongs to the SELECT that owns it, not to the outermost one.
		"SELECT (SELECT s FROM d WHERE d.r = g.id ORDER BY d.c DESC LIMIT 1) FROM g ORDER BY n LIMIT @p1 OFFSET @p2": "SELECT (SELECT TOP (1) s FROM d WHERE d.r = g.id ORDER BY d.c DESC) FROM g ORDER BY n OFFSET @p2 ROWS FETCH NEXT @p1 ROWS ONLY",
		// OFFSET/FETCH needs an ORDER BY; without one the statement is left for the
		// server to refuse rather than silently paged wrongly.
		"SELECT a FROM t LIMIT @p1 OFFSET @p2": "SELECT a FROM t LIMIT @p1 OFFSET @p2",
		// TOP on one branch of a set operator would not bound the compound.
		"SELECT a FROM t UNION ALL SELECT a FROM u LIMIT 5": "SELECT a FROM t UNION ALL SELECT a FROM u LIMIT 5",
		// A LIMIT inside a derived table, with the compound one level down.
		"SELECT a FROM (SELECT a FROM t UNION ALL SELECT a FROM u) x ORDER BY a LIMIT 1": "SELECT TOP (1) a FROM (SELECT a FROM t UNION ALL SELECT a FROM u) x ORDER BY a",
		"SELECT 'LIMIT 5' AS note FROM t":                                                "SELECT 'LIMIT 5' AS note FROM t",
		"SELECT a FROM t":                                                                "SELECT a FROM t",
	})
}

// TestMSSQLScalarFunctions pins one case per scalar rewrite, including the two whose
// argument order or arity changes and would otherwise fail silently.
func TestMSSQLScalarFunctions(t *testing.T) {
	check(t, "mssqlRules", mssqlRules, map[string]string{
		// CHARINDEX takes the needle first. A rule that kept SQLite's order would
		// compile and return the wrong rows.
		"SELECT instr(name, 'ab') FROM t": "SELECT CHARINDEX('ab', name) FROM t",
		// SUBSTRING has no two-argument form.
		"SELECT substr(day, 6) FROM t":     "SELECT SUBSTRING(day, 6, 2147483647) FROM t",
		"SELECT substr(day, 1, 10) FROM t": "SELECT SUBSTRING(day, 1, 10) FROM t",
		// A negative start counts from the end in SQLite and from before the start in
		// T-SQL, so it is left as written for SQL Server to refuse.
		"SELECT substr(day, -3) FROM t":                     "SELECT substr(day, -3) FROM t",
		"SELECT substr(day, -3, 2) FROM t":                  "SELECT substr(day, -3, 2) FROM t",
		"SELECT substr(day, (-3), 2) FROM t":                "SELECT substr(day, (-3), 2) FROM t",
		"SELECT substr(day, n - 3) FROM t":                  "SELECT SUBSTRING(day, n - 3, 2147483647) FROM t",
		"SELECT ifnull(a, b) FROM t":                        "SELECT ISNULL(a, b) FROM t",
		"SELECT CAST(x AS INTEGER), CAST(y AS TEXT) FROM t": "SELECT CAST(x AS BIGINT), CAST(y AS NVARCHAR(MAX)) FROM t",
		// AVG over an integer divides as an integer in T-SQL, so the rollups average a float.
		"SELECT CAST(AVG(CAST(m AS FLOAT)) AS BIGINT) FROM t": "SELECT CAST(AVG(CAST(m AS FLOAT)) AS BIGINT) FROM t",
		// An alias that happens to spell a type is not a cast target.
		"SELECT x AS text FROM t":                    "SELECT x AS text FROM t",
		"SELECT group_concat(name, char(31)) FROM t": "SELECT STRING_AGG(CAST(name AS nvarchar(max)), '\x1f') FROM t",
		"SELECT group_concat(name) FROM t":           "SELECT STRING_AGG(CAST(name AS nvarchar(max)), ',') FROM t",
		"SELECT char(31) FROM t":                     "SELECT NCHAR(31) FROM t",
		"SELECT random() FROM t":                     "SELECT CHECKSUM(NEWID()) FROM t",
		"SELECT hex(randomblob(8)) FROM t":           "SELECT CONVERT(nvarchar(16), CRYPT_GEN_RANDOM(8), 2) FROM t",
		"SELECT printf('%02d', n) FROM t":            "SELECT RIGHT('0' + CONVERT(nvarchar(12), n), 2) FROM t",
		"SELECT a FROM t WHERE ts GLOB '[0-9]*'":     "SELECT a FROM t WHERE ts LIKE '[0-9]%'",
		"SELECT json_extract(meta, '$.kind') FROM t": "SELECT JSON_VALUE(meta, '$.kind') FROM t",
		"SELECT json_extract(meta, '$') FROM t":      "SELECT meta FROM t",
		"SELECT name COLLATE NOCASE FROM t":          "SELECT lower(name) FROM t",
		"SELECT m.rowid FROM m":                      "SELECT m.id FROM m",
		// Two-argument max() is a scalar in SQLite and an aggregate in T-SQL; the
		// one-argument form stays an aggregate.
		"UPDATE t SET e = MAX(t.e, 1)": "UPDATE t SET e = (CASE WHEN (t.e) >= (1) THEN (t.e) ELSE (1) END)",
		"SELECT MAX(e) FROM t":         "SELECT MAX(e) FROM t",
		// A DATETIME(3) type modifier shares its spelling with the function.
		"SELECT CAST(x AS DATETIME(3)) FROM t": "SELECT CAST(x AS DATETIME(3)) FROM t",
	})
}

// TestMSSQLTimeFunctions pins the three time rewrites. Every timestamp column in this
// schema is text, so whatever comes out has to be comparable with an RFC3339 string.
func TestMSSQLTimeFunctions(t *testing.T) {
	check(t, "mssqlRules", mssqlRules, map[string]string{
		"UPDATE t SET u = datetime('now')":                    "UPDATE t SET u = CONVERT(nvarchar(19), SYSUTCDATETIME(), 120)",
		"UPDATE t SET u = datetime('now', '-1 day')":          "UPDATE t SET u = CONVERT(nvarchar(19), DATEADD(day, -1, SYSUTCDATETIME()), 120)",
		"SELECT strftime('%s', occurred_at) FROM t":           "SELECT DATEDIFF_BIG(second, '1970-01-01', CAST(REPLACE(REPLACE(occurred_at, 'T', ' '), 'Z', '') AS DATETIME2(3))) FROM t",
		"SELECT strftime('%Y-%m-%dT%H:%M:%SZ', 'now') FROM t": "SELECT (CONVERT(nvarchar(19), SYSUTCDATETIME(), 126) + 'Z') FROM t",
		"SELECT julianday('now') FROM t":                      "SELECT (DATEDIFF_BIG(microsecond, '1970-01-01', SYSUTCDATETIME()) / 86400000000.0 + 2440587.5) FROM t",
		// The runtime pruner builds its modifier from a bound argument.
		"SELECT julianday('now', '-' || ? || ' days') FROM t": "SELECT (DATEDIFF_BIG(microsecond, '1970-01-01', DATEADD(day, -(?), SYSUTCDATETIME())) / 86400000000.0 + 2440587.5) FROM t",
		// A modifier this translator cannot read keeps its call, so the server refuses
		// it rather than returning the wrong instant.
		"SELECT datetime('now', 'start of month') FROM t": "SELECT datetime('now', 'start of month') FROM t",
		// CURRENT_TIMESTAMP is a datetime in T-SQL and would store `Sep 27 2026 3:01AM`
		// in the nvarchar column these statements target.
		"INSERT INTO t (a) VALUES (CURRENT_TIMESTAMP)": "INSERT INTO t (a) VALUES ((CONVERT(nvarchar(23), SYSUTCDATETIME(), 126) + 'Z'))",
		"SELECT 'CURRENT_TIMESTAMP' FROM t":            "SELECT 'CURRENT_TIMESTAMP' FROM t",
	})
}

// TestMSSQLConcatOnlyOutsideLiterals guards the one rewrite that would corrupt stored
// data if it reached inside a string: the RBAC policy expressions carry a literal `||`.
func TestMSSQLConcatOnlyOutsideLiterals(t *testing.T) {
	check(t, "mssqlConcat", mssqlConcat, map[string]string{
		"SELECT 'a' || b || 'c' FROM t":        "SELECT 'a' + b + 'c' FROM t",
		"SELECT ':' || ? || ':' FROM t":        "SELECT ':' + ? + ':' FROM t",
		"UPDATE p SET expr = 'a || b' WHERE 1": "UPDATE p SET expr = 'a || b' WHERE 1",
		"SELECT a FROM t -- x || y\nWHERE 1":   "SELECT a FROM t -- x || y\nWHERE 1",
		"SELECT a FROM t":                      "SELECT a FROM t",
	})
}

// TestMSSQLUpserts pins the four SQLite upsert spellings. `DO UPDATE` becomes a MERGE;
// every "ignore the duplicate" form becomes a TRY/CATCH, because the unique index decides
// there and a NOT EXISTS predicate would suppress the NULL-keyed rows SQLite inserts.
func TestMSSQLUpserts(t *testing.T) {
	check(t, "MSSQL", MSSQL, map[string]string{
		"INSERT INTO settings (id, key, value) VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value": "MERGE settings WITH (HOLDLOCK) AS tgt USING (VALUES (@p1, @p2, @p3)) AS src (id, [key], value) ON tgt.[key] = src.[key] WHEN MATCHED THEN UPDATE SET value = src.value WHEN NOT MATCHED THEN INSERT (id, [key], value) VALUES (src.id, src.[key], src.value);",
		"INSERT INTO bg (id, user_id, note) VALUES (?, ?, ?) ON CONFLICT (user_id) DO UPDATE SET note = EXCLUDED.note": "MERGE bg WITH (HOLDLOCK) AS tgt USING (VALUES (@p1, @p2, @p3)) AS src (id, user_id, note) ON tgt.user_id = src.user_id WHEN MATCHED THEN UPDATE SET note = src.note WHEN NOT MATCHED THEN INSERT (id, user_id, note) VALUES (src.id, src.user_id, src.note);",
		"INSERT OR IGNORE INTO group_members (id, group_id, user_id) VALUES (?, ?, ?)":                                 "BEGIN TRY INSERT INTO group_members (id, group_id, user_id) VALUES (@p1, @p2, @p3); END TRY BEGIN CATCH IF ERROR_NUMBER() NOT IN (2601, 2627) THROW; END CATCH;",
		"INSERT INTO lifecycle_events (id, action) VALUES (?, ?) ON CONFLICT (id) DO NOTHING":                          "BEGIN TRY INSERT INTO lifecycle_events (id, action) VALUES (@p1, @p2); END TRY BEGIN CATCH IF ERROR_NUMBER() NOT IN (2601, 2627) THROW; END CATCH;",
		"INSERT INTO table_views (id, user_id, kind) VALUES (?, ?, 'layout') ON CONFLICT DO NOTHING":                   "BEGIN TRY INSERT INTO table_views (id, user_id, kind) VALUES (@p1, @p2, 'layout'); END TRY BEGIN CATCH IF ERROR_NUMBER() NOT IN (2601, 2627) THROW; END CATCH;",
		"INSERT OR REPLACE INTO workflow_kv (key, value) VALUES (?, ?)":                                                "MERGE workflow_kv WITH (HOLDLOCK) AS tgt USING (VALUES (@p1, @p2)) AS src ([key], value) ON tgt.[key] = src.[key] WHEN MATCHED THEN UPDATE SET value = src.value WHEN NOT MATCHED THEN INSERT ([key], value) VALUES (src.[key], src.value);",
		// A SELECT-driven DO NOTHING stays a TRY/CATCH; only DO UPDATE needs the MERGE.
		"INSERT INTO permission_catalog (key, scope) SELECT permission_key, scope FROM plugin_permissions WHERE permission_key = ? LIMIT 1 ON CONFLICT (key) DO NOTHING": "BEGIN TRY INSERT INTO permission_catalog ([key], scope) SELECT TOP (1) permission_key, scope FROM plugin_permissions WHERE permission_key = @p1; END TRY BEGIN CATCH IF ERROR_NUMBER() NOT IN (2601, 2627) THROW; END CATCH;",
	})
	// The target table's own name in the SET body is what the MERGE alias replaces; a
	// missed one would resolve to nothing and the statement would not compile.
	got := MSSQL("INSERT INTO settings_catalogs (id, enabled) VALUES (?, ?) ON CONFLICT(id) DO UPDATE SET enabled = MAX(settings_catalogs.enabled, excluded.enabled)")
	if !strings.Contains(got, "SET enabled = (CASE WHEN (tgt.enabled) >= (src.enabled)") {
		t.Errorf("target qualifier not rewritten: %q", got)
	}
	if strings.Contains(got, "settings_catalogs.enabled") {
		t.Errorf("target qualifier survived: %q", got)
	}
}

// TestMSSQLUpsertFromSelect guards TOFIX-06: a SELECT-driven DO UPDATE becomes a MERGE
// over a derived table, with SQLite's mandatory `WHERE true` dropped and a LIMIT inside
// the source still turned into TOP, instead of reaching the server as ON CONFLICT.
func TestMSSQLUpsertFromSelect(t *testing.T) {
	check(t, "MSSQL", MSSQL, map[string]string{
		"INSERT INTO settings (id, key, value) SELECT id, name, note FROM drafts WHERE owner = ? ON CONFLICT(key) DO UPDATE SET value = excluded.value":         "MERGE settings WITH (HOLDLOCK) AS tgt USING (SELECT id, name, note FROM drafts WHERE owner = @p1) AS src (id, [key], value) ON tgt.[key] = src.[key] WHEN MATCHED THEN UPDATE SET value = src.value WHEN NOT MATCHED THEN INSERT (id, [key], value) VALUES (src.id, src.[key], src.value);",
		"INSERT INTO bg (id, user_id, note) SELECT id, user_id, note FROM staging WHERE true ON CONFLICT (user_id) DO UPDATE SET note = excluded.note":          "MERGE bg WITH (HOLDLOCK) AS tgt USING (SELECT id, user_id, note FROM staging) AS src (id, user_id, note) ON tgt.user_id = src.user_id WHEN MATCHED THEN UPDATE SET note = src.note WHEN NOT MATCHED THEN INSERT (id, user_id, note) VALUES (src.id, src.user_id, src.note);",
		"INSERT INTO bg (id, user_id, note) SELECT id, user_id, note FROM staging ORDER BY id LIMIT 1 ON CONFLICT (user_id) DO UPDATE SET note = excluded.note": "MERGE bg WITH (HOLDLOCK) AS tgt USING (SELECT TOP (1) id, user_id, note FROM staging ORDER BY id) AS src (id, user_id, note) ON tgt.user_id = src.user_id WHEN MATCHED THEN UPDATE SET note = src.note WHEN NOT MATCHED THEN INSERT (id, user_id, note) VALUES (src.id, src.user_id, src.note);",
	})
	// A VALUES source followed by anything is still refused, so the SELECT branch did not
	// widen what the VALUES branch accepts.
	multi := MSSQL("INSERT INTO bg (id, user_id) VALUES (?, ?), (?, ?) ON CONFLICT (user_id) DO UPDATE SET id = excluded.id")
	if !strings.Contains(multi, "ON CONFLICT") {
		t.Errorf("multi-row VALUES must keep ON CONFLICT so the server refuses it: %q", multi)
	}
}

// TestMSSQLNullSafeEquality guards SQLite's `col IS ?`, which T-SQL has no operator for.
func TestMSSQLNullSafeEquality(t *testing.T) {
	check(t, "mssqlNullSafeEquality", mssqlNullSafeEquality, map[string]string{
		"SELECT a FROM t WHERE environment_id IS @p1 AND day >= @p2": "SELECT a FROM t WHERE (environment_id = @p1 OR (environment_id IS NULL AND @p1 IS NULL)) AND day >= @p2",
		"SELECT a FROM t WHERE s.env IS @p1":                         "SELECT a FROM t WHERE (s.env = @p1 OR (s.env IS NULL AND @p1 IS NULL))",
		"SELECT a FROM t WHERE prev IS NOT NULL":                     "SELECT a FROM t WHERE prev IS NOT NULL",
		"SELECT a FROM t WHERE prev IS NULL":                         "SELECT a FROM t WHERE prev IS NULL",
		"SELECT 'x IS @p9' FROM t":                                   "SELECT 'x IS @p9' FROM t",
	})
}

// TestMSSQLBracketsReservedNamesExactly pins the reserved-word bracketing: a word is
// bracketed where it names something and left alone where it is the keyword.
func TestMSSQLBracketsReservedNamesExactly(t *testing.T) {
	cases := map[string]string{
		"SELECT value FROM settings WHERE key = @p1":                 "SELECT value FROM settings WHERE [key] = @p1",
		"INSERT INTO plugin_widgets (origin, key) VALUES (@p1, @p2)": "INSERT INTO plugin_widgets (origin, [key]) VALUES (@p1, @p2)",
		"SELECT s.key, s.value FROM settings s ORDER BY s.key":       "SELECT s.[key], s.value FROM settings s ORDER BY s.[key]",
		"SELECT id, trigger FROM workflows":                          "SELECT id, [trigger] FROM workflows",
		// A genuine keyword, not a name.
		"INSERT INTO t (a) VALUES (@p1) ON DUPLICATE KEY UPDATE a = a": "INSERT INTO t (a) VALUES (@p1) ON DUPLICATE KEY UPDATE a = a",
		"SELECT 'key = order' AS note FROM settings WHERE key = 'x'":   "SELECT 'key = order' AS note FROM settings WHERE [key] = 'x'",
		"SELECT a FROM t -- key\nWHERE b = @p1":                        "SELECT a FROM t -- key\nWHERE b = @p1",
		// DDL is the migration translator's business, and already-bracketed names are
		// left alone so a second pass changes nothing.
		"CREATE TABLE x (id TEXT PRIMARY KEY, key TEXT)": "CREATE TABLE x (id TEXT PRIMARY KEY, key TEXT)",
		"SELECT [key] FROM settings":                     "SELECT [key] FROM settings",
	}
	for in, want := range cases {
		got := quoteReservedMSSQL(in)
		if got != want {
			t.Errorf("quoteReservedMSSQL(%q)\n got %q\nwant %q", in, got, want)
		}
		if again := quoteReservedMSSQL(got); again != got {
			t.Errorf("not idempotent: %q -> %q", got, again)
		}
	}
	// PRIMARY KEY inside a DML statement is not a name either.
	if got := quoteReservedMSSQL("SELECT a FROM t WHERE k = @p1"); got != "SELECT a FROM t WHERE k = @p1" {
		t.Errorf("unrelated statement changed: %q", got)
	}
}

// TestMSSQLAliasesDerivedTables guards the alias T-SQL requires on every derived table;
// the users and groups role-name subqueries have none.
func TestMSSQLAliasesDerivedTables(t *testing.T) {
	check(t, "mssqlAliasDerivedTables", mssqlAliasDerivedTables, map[string]string{
		"SELECT a FROM (SELECT a FROM t) WHERE a > 1":        "SELECT a FROM (SELECT a FROM t) AS mch_d1 WHERE a > 1",
		"SELECT a FROM (SELECT a FROM t) s":                  "SELECT a FROM (SELECT a FROM t) s",
		"SELECT a FROM (SELECT a FROM t) AS s":               "SELECT a FROM (SELECT a FROM t) AS s",
		"SELECT a FROM (SELECT a FROM t), (SELECT b FROM u)": "SELECT a FROM (SELECT a FROM t) AS mch_d1, (SELECT b FROM u)",
		"SELECT (SELECT max(a) FROM t) FROM u":               "SELECT (SELECT max(a) FROM t) FROM u",
		"SELECT a FROM t JOIN (SELECT b FROM u) ON 1 = 1":    "SELECT a FROM t JOIN (SELECT b FROM u) AS mch_d1 ON 1 = 1",
	})
}

// TestMSSQLArgsAndValue guard the boolean coercion: every flag in this schema is an
// integer column, and database/sql will not convert a Go bool into an int either way.
func TestMSSQLArgsAndValue(t *testing.T) {
	args := []driver.NamedValue{
		{Ordinal: 1, Value: true},
		{Ordinal: 2, Value: false},
		{Ordinal: 3, Value: "keep"},
	}
	MSSQLArgs("INSERT INTO t (a, b, c) VALUES (@p1, @p2, @p3)", args)
	if args[0].Value != int64(1) || args[1].Value != int64(0) || args[2].Value != "keep" {
		t.Fatalf("args = %+v", args)
	}
	if MSSQLValue(true) != int64(1) || MSSQLValue(false) != int64(0) {
		t.Fatal("a BIT result column must scan as an integer")
	}
	if MSSQLValue("x") != "x" {
		t.Fatal("a non-boolean value must pass through")
	}
}

// TestMSSQLDSNNamesTheApplication guards that the DSN gains the application name once and
// that an operator's own setting is never overwritten.
func TestMSSQLDSNNamesTheApplication(t *testing.T) {
	got := MSSQLDSN("sqlserver://sa:pw@host:1433?database=mcharbor")
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parsing %q: %v", got, err)
	}
	if u.Query().Get("app") != "McHarbor" || u.Query().Get("database") != "mcharbor" {
		t.Fatalf("query = %v", u.Query())
	}
	pinned := "sqlserver://sa:pw@host:1433?app=Custom&database=mcharbor"
	if MSSQLDSN(pinned) != pinned {
		t.Fatalf("an explicit app name was overwritten: %q", MSSQLDSN(pinned))
	}
}

// TestMSSQLEndToEnd pins two whole statements from the application, so a change to the
// order the rules run in shows up as a diff rather than as a query that no longer parses.
func TestMSSQLEndToEnd(t *testing.T) {
	check(t, "MSSQL", MSSQL, map[string]string{
		"SELECT u.id, (SELECT GROUP_CONCAT(name, char(31)) FROM (SELECT DISTINCT r.name AS name FROM user_roles ur WHERE ur.user_id = u.id)) AS role_names FROM users u ORDER BY u.username ASC LIMIT ? OFFSET ?": "SELECT u.id, (SELECT STRING_AGG(CAST(name AS nvarchar(max)), '\x1f') FROM (SELECT DISTINCT r.name AS name FROM user_roles ur WHERE ur.user_id = u.id) AS mch_d1) AS role_names FROM users u ORDER BY u.username ASC OFFSET @p2 ROWS FETCH NEXT @p1 ROWS ONLY",
		"DELETE FROM e WHERE id IN (SELECT id FROM e WHERE julianday(occurred_at) < julianday('now', '-' || ? || ' days') LIMIT ?)":                                                                               "DELETE FROM e WHERE id IN (SELECT TOP (@p2) id FROM e WHERE (DATEDIFF_BIG(microsecond, '1970-01-01', CAST(REPLACE(REPLACE(occurred_at, 'T', ' '), 'Z', '') AS DATETIME2(3))) / 86400000000.0 + 2440587.5) < (DATEDIFF_BIG(microsecond, '1970-01-01', DATEADD(day, -(@p1), SYSUTCDATETIME())) / 86400000000.0 + 2440587.5))",
	})
}
