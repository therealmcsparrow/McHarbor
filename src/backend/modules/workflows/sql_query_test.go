// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

var sqlNodeSecrets = []string{"SECRETHASH", "APIKEYHASH", "SESSIONHASH", "password_hash", "key_hash", "CREATE TABLE"}

// openSQLNodeHostileDB is a file-backed database holding the allow-listed workflow tables
// beside tables the node must never read, each seeded with a recognisable secret.
func openSQLNodeHostileDB(t *testing.T, maxConns int) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "hostile.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(maxConns)
	t.Cleanup(func() { closeutil.Report(db.Close(), "db") })

	for _, stmt := range []string{
		`CREATE TABLE workflows (id TEXT PRIMARY KEY, name TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'active')`,
		`CREATE TABLE workflow_runs (id TEXT PRIMARY KEY, workflow_id TEXT NOT NULL, status TEXT NOT NULL, trigger TEXT NOT NULL DEFAULT 'manual', duration_ms INTEGER DEFAULT 0, started_at TEXT NOT NULL)`,
		`CREATE INDEX idx_workflow_runs_workflow_id ON workflow_runs(workflow_id)`,
		`CREATE TABLE workflow_kv (id TEXT PRIMARY KEY, key TEXT NOT NULL UNIQUE, value TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE workflow_link_messages (id TEXT PRIMARY KEY, workflow_id TEXT NOT NULL, node_id TEXT NOT NULL, msg TEXT DEFAULT '{}')`,
		`CREATE UNIQUE INDEX idx_wlm_workflow_node ON workflow_link_messages(workflow_id, node_id)`,
		`CREATE TABLE workflow_metrics (id TEXT PRIMARY KEY, workflow_id TEXT, metric_name TEXT NOT NULL, numeric_value REAL)`,
		`CREATE TABLE users (id TEXT PRIMARY KEY, username TEXT, password_hash TEXT)`,
		`CREATE TABLE api_keys (id TEXT PRIMARY KEY, key_hash TEXT)`,
		`CREATE TABLE sessions (id TEXT PRIMARY KEY, user_id TEXT)`,
		`INSERT INTO users VALUES ('u1', 'admin', '$argon2id$v=19$m=65536$SECRETHASH')`,
		`INSERT INTO api_keys VALUES ('k1', 'APIKEYHASH')`,
		`INSERT INTO sessions VALUES ('SESSIONHASH', 'u1')`,
		`INSERT INTO workflows VALUES ('wf-1', 'Nightly', 'active'), ('wf-2', 'Weekly', 'paused')`,
		`INSERT INTO workflow_runs VALUES ('r1', 'wf-1', 'completed', 'manual', 10, '2026-10-01'), ('r2', 'wf-1', 'failed', 'cron', 30, '2026-10-02'), ('r3', 'wf-2', 'completed', 'cron', 20, '2026-10-03')`,
		`INSERT INTO workflow_kv VALUES ('kv1', 'last_backup', 'ok')`,
		`INSERT INTO workflow_link_messages VALUES ('l1', 'wf-1', 'n1', '{}')`,
		`INSERT INTO workflow_metrics VALUES ('m1', 'wf-1', 'latency', 1.5), ('m2', 'wf-1', 'latency', 2.5)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed %q: %v", stmt, err)
		}
	}
	return db
}

func runSQLNode(t *testing.T, db *sql.DB, query string) (string, Msg) {
	t.Helper()
	svc := NewService(db, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, nil, "", nil)
	node := &CanvasNode{Config: map[string]interface{}{"query": query, "output_property": "payload"}}
	port, out, err := svc.executeSQLQuery(t.Context(), node, NewMsg(nil))
	if err != nil {
		t.Fatalf("execute sql query %q: %v", query, err)
	}
	return port, out
}

// The first block was executed by the regex allow-list and leaked; the rest it already
// refused, and they must stay refused now that the regex is no longer the boundary.
var sqlNodeHostilePayloads = []string{
	`SELECT * FROM workflow_runs, users`,
	`SELECT * FROM "users"`,
	`SELECT * FROM [users]`,
	"SELECT * FROM `users`",
	`SELECT * FROM/**/users`,
	`SELECT * FROM "main".users`,
	`SELECT * FROM workflow_runs JOIN"users"`,
	`SELECT id FROM workflow_runs UNION SELECT password_hash FROM "users"`,
	`WITH x AS (SELECT password_hash FROM "users") SELECT * FROM "x"`,
	`WITH x AS (SELECT password_hash FROM "users") SELECT * FROM x`,
	`WITH x AS (SELECT 1) SELECT * FROM x, (SELECT * FROM "users")`,
	`SELECT name, sql FROM "sqlite_master"`,
	`SELECT * FROM workflow_runs, sqlite_master`,
	`SELECT * FROM workflow_runs, "sqlite_schema"`,
	`SELECT * FROM workflow_runs, pragma_table_info('users')`,
	`SELECT * FROM workflow_runs, pragma_database_list`,
	`SELECT * FROM workflow_runs CROSS JOIN "api_keys"`,
	`SELECT * FROM workflow_runs, sessions`,
	`SELECT (SELECT key_hash FROM "api_keys") k, (SELECT id FROM "sessions") s`,
	`SELECT * FROM workflow_runs WHERE 0 OR EXISTS(SELECT 1 FROM "main"."users" WHERE password_hash LIKE '$argon%')`,
	`SELECT * FROM workflow_runs WHERE id IN (SELECT "id" FROM "users")`,
	`SELECT * FROM users`,
	`SELECT * FROM main.users`,
	`SELECT * FROM UsErS`,
	`SELECT (SELECT password_hash FROM users) AS h`,
	`WITH x AS (SELECT * FROM users) SELECT * FROM x`,
	`SELECT name, sql FROM sqlite_master`,
	`SELECT * FROM sqlite_temp_master`,
	`SELECT * FROM pragma_table_info('users')`,
	`ATTACH 'x.db' AS x`,
	`SELECT sqlite_version(), load_extension('x')`,
	`WITH d AS (DELETE FROM users RETURNING *) SELECT * FROM d`,
	`WITH x AS (SELECT 1) UPDATE "users" SET password_hash='pwn'`,
	`WITH x AS (SELECT 1) INSERT INTO "users" VALUES('u2','evil','x')`,
	`SELECT 1; DELETE FROM users`,
}

func TestExecuteSQLQueryRefusesEveryHostilePayload(t *testing.T) {
	db := openSQLNodeHostileDB(t, 4)

	for _, query := range sqlNodeHostilePayloads {
		t.Run(query, func(t *testing.T) {
			port, out := runSQLNode(t, db, query)
			encoded, err := json.Marshal(out["payload"])
			if err != nil {
				t.Fatalf("marshal payload: %v", err)
			}
			for _, secret := range sqlNodeSecrets {
				if strings.Contains(string(encoded), secret) {
					t.Fatalf("payload leaked %q: %s", secret, encoded)
				}
			}
			if port != "error" {
				t.Fatalf("expected the error port, got %q with %s", port, encoded)
			}
		})
	}

	var users, runs int
	if err := db.QueryRow(`SELECT count(*) FROM users WHERE password_hash LIKE '%SECRETHASH'`).Scan(&users); err != nil || users != 1 {
		t.Fatalf("users table was modified: count=%d err=%v", users, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM workflow_runs`).Scan(&runs); err != nil || runs != 3 {
		t.Fatalf("workflow_runs was modified: count=%d err=%v", runs, err)
	}
}

func TestWorkflowSQLGuardNamesTheRefusal(t *testing.T) {
	db := openSQLNodeHostileDB(t, 4)
	cases := map[string]error{
		`SELECT * FROM "users"`:                                         ErrSQLQueryTableNotAllowed,
		`SELECT * FROM workflow_runs, users`:                            ErrSQLQueryTableNotAllowed,
		`SELECT name FROM "sqlite_master"`:                              ErrSQLQueryTableNotAllowed,
		`SELECT * FROM "main".users`:                                    ErrSQLQueryTableNotAllowed,
		`SELECT * FROM workflow_runs, pragma_table_info('users')`:       ErrSQLQueryVirtualTableRefused,
		`SELECT * FROM workflow_runs, pragma_database_list`:             ErrSQLQueryVirtualTableRefused,
		`SELECT * FROM workflow_runs, json_each('[1,2]')`:               ErrSQLQueryVirtualTableRefused,
		`WITH x AS (SELECT password_hash FROM "users") SELECT * FROM x`: ErrSQLQueryTableNotAllowed,
	}
	for query, want := range cases {
		err := runGuardedWorkflowSQL(t, db, query)
		if err == nil {
			t.Fatalf("%q: expected %v, query ran", query, want)
		}
		if !errors.Is(err, want) {
			t.Fatalf("%q: expected %v, got %v", query, want, err)
		}
	}
}

func runGuardedWorkflowSQL(t *testing.T, db *sql.DB, query string) error {
	t.Helper()
	rows, cleanup, err := openReadOnlyWorkflowSQLRows(t.Context(), db, nil, query)
	if err != nil {
		return err
	}
	defer cleanup()
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	for rows.Next() {
		t.Logf("%q returned a row", query)
	}
	return rows.Err()
}

func TestExecuteSQLQueryAnswersLegitimateQueries(t *testing.T) {
	db := openSQLNodeHostileDB(t, 4)
	cases := map[string]int{
		`SELECT id FROM workflow_runs ORDER BY id`:                                                                              3,
		`SELECT * FROM "workflow_runs";`:                                                                                        3,
		`SELECT w.name, r.status FROM workflows w JOIN workflow_runs r ON r.workflow_id = w.id`:                                 3,
		`SELECT w.name FROM workflows w, workflow_runs r WHERE r.workflow_id = w.id AND r.status = 'failed'`:                    1,
		`SELECT w.name, count(r.id) AS runs FROM workflows w LEFT JOIN workflow_runs r ON r.workflow_id = w.id GROUP BY w.name`: 2,
		`SELECT status, count(*) AS n, avg(duration_ms) AS avg_ms FROM workflow_runs GROUP BY status`:                           2,
		`WITH recent AS (SELECT * FROM workflow_runs WHERE status = 'completed') SELECT count(*) AS n FROM recent`:              1,
		`WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x + 1 FROM n WHERE x < 5) SELECT x FROM n`:                           5,
		`SELECT * FROM workflows WHERE id IN (SELECT workflow_id FROM workflow_runs WHERE status = 'failed')`:                   1,
		`SELECT key AS v FROM workflow_kv UNION SELECT name FROM workflows`:                                                     3,
		`SELECT DISTINCT workflow_id FROM workflow_runs WHERE workflow_id = 'wf-1'`:                                             1,
		`SELECT value FROM workflow_kv WHERE key = 'last_backup'`:                                                               1,
		`SELECT msg FROM workflow_link_messages WHERE workflow_id = 'wf-1' AND node_id = 'n1'`:                                  1,
		`SELECT metric_name, max(numeric_value) AS peak FROM workflow_metrics GROUP BY metric_name`:                             1,
		`SELECT 1 AS one`: 1,
	}
	for query, want := range cases {
		port, out := runSQLNode(t, db, query)
		if port != "output" {
			t.Fatalf("%q: expected the output port, got %q: %#v", query, port, out["payload"])
		}
		rows, ok := out["payload"].([]interface{})
		if !ok || len(rows) != want {
			t.Fatalf("%q: expected %d rows, got %#v", query, want, out["payload"])
		}
	}
}

func TestWorkflowSQLConnectionIsResetAfterAQuery(t *testing.T) {
	db := openSQLNodeHostileDB(t, 1)
	if port, out := runSQLNode(t, db, `SELECT id FROM workflow_runs`); port != "output" {
		t.Fatalf("expected the output port, got %q: %#v", port, out["payload"])
	}
	if _, err := db.Exec(`INSERT INTO workflow_kv VALUES ('kv2', 'after', 'x')`); err != nil {
		t.Fatalf("pooled connection still read-only after a query: %v", err)
	}
}

func TestWorkflowSQLConnectionIsDiscardedWhenResetFails(t *testing.T) {
	db := openSQLNodeHostileDB(t, 1)
	ctx := t.Context()

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("conn: %v", err)
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA query_only = ON"); err != nil {
		t.Fatalf("query_only: %v", err)
	}
	if _, err := conn.ExecContext(ctx, "BEGIN"); err != nil {
		t.Fatalf("begin: %v", err)
	}

	releaseWorkflowSQLConn(ctx, conn, nil, []string{"SELEC this reset cannot run"})

	var queryOnly int
	if err := db.QueryRow(`PRAGMA query_only`).Scan(&queryOnly); err != nil {
		t.Fatalf("read query_only: %v", err)
	}
	if queryOnly != 0 {
		t.Fatalf("the pool handed back the read-only connection (query_only=%d)", queryOnly)
	}
	if _, err := db.Exec(`INSERT INTO workflow_kv VALUES ('kv3', 'discarded', 'x')`); err != nil {
		t.Fatalf("write after discarded connection: %v", err)
	}
}
