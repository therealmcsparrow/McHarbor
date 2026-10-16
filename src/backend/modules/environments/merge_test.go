// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package environments

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/secrets"
	"github.com/therealmcsparrow/mcharbor/core/sqldialect"
	"github.com/therealmcsparrow/mcharbor/core/telemetry"

	_ "modernc.org/sqlite"
)

const (
	mergeSrc = "envsrc"
	mergeTgt = "envtgt"
)

// mergeBackend is one database the merge is exercised on. SQLite always runs; each
// external backend runs when its DSN is set, in a database of its own, because the
// schema walk reads every table in the database it is pointed at.
type mergeBackend struct {
	name, envVar string
	driver       db.Driver
	dialect      telemetry.Dialect
	normalise    func(string) string
}

var mergeBackends = []mergeBackend{
	{name: "sqlite", driver: db.DriverSQLite},
	{name: "postgres", envVar: "MCHARBOR_TEST_PG_DSN", driver: db.DriverPostgres, normalise: sqldialect.PostgresDSN,
		dialect: telemetry.Dialect{Rewrite: sqldialect.Postgres, Args: sqldialect.PostgresArgs, Value: sqldialect.PostgresValue}},
	{name: "mysql", envVar: "MCHARBOR_TEST_MYSQL_DSN", driver: db.DriverMySQL, normalise: sqldialect.MySQLDSN,
		dialect: telemetry.Dialect{Rewrite: sqldialect.MySQL, Args: sqldialect.MySQLArgs, Retry: sqldialect.MySQLRetry}},
	{name: "mariadb", envVar: "MCHARBOR_TEST_MARIADB_DSN", driver: db.DriverMySQL, normalise: sqldialect.MySQLDSN,
		dialect: telemetry.Dialect{Rewrite: sqldialect.MySQL, Args: sqldialect.MySQLArgs, Retry: sqldialect.MySQLRetry}},
	{name: "mssql", envVar: "MCHARBOR_TEST_MSSQL_DSN", driver: db.DriverMSSQL, normalise: sqldialect.MSSQLDSN,
		dialect: telemetry.Dialect{Rewrite: sqldialect.MSSQL, Args: sqldialect.MSSQLArgs, Value: sqldialect.MSSQLValue}},
}

// forEachMergeBackend runs fn once per available backend, each on a fresh schema.
func forEachMergeBackend(t *testing.T, fn func(t *testing.T, database *sql.DB, driverName db.Driver)) {
	for _, backend := range mergeBackends {
		t.Run(backend.name, func(t *testing.T) {
			fn(t, openMergeDB(t, backend), backend.driver)
		})
	}
}

// mergeDatabaseName is the scratch database each external backend's merge runs in.
const mergeDatabaseName = "mcharbor_merge_test"

// openMergeDB returns a database holding only the merge fixture's tables, opened
// through the dialect wrapper the host installs, so placeholders and quoting reach
// the server exactly as they do in production.
func openMergeDB(t *testing.T, backend mergeBackend) *sql.DB {
	t.Helper()
	if backend.driver == db.DriverSQLite {
		database, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		database.SetMaxOpenConns(1)
		t.Cleanup(func() {
			if err := database.Close(); err != nil {
				t.Errorf("closing test database: %v", err)
			}
		})
		createMergeFixture(t, database, backend.driver)
		return database
	}
	dsn := os.Getenv(backend.envVar)
	if dsn == "" {
		t.Skip(backend.envVar + " not set")
	}
	scratch := scratchMergeDSN(t, backend.driver, dsn)
	db.SetConnectorWrapper(func(c driver.Connector, driverName db.Driver) driver.Connector {
		return telemetry.WrapDBConnectorWithDialect(c, string(driverName), backend.dialect)
	})
	t.Cleanup(func() { db.SetConnectorWrapper(nil) })
	database, err := db.Open(db.Config{Driver: backend.driver, DSN: backend.normalise(scratch)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing test database: %v", err)
		}
	})
	for _, table := range []string{"alert_states", "secrets", "samples", "stack_links", "env_settings", "pools", "environments"} {
		mustExec(t, database, "DROP TABLE IF EXISTS "+table)
	}
	createMergeFixture(t, database, backend.driver)
	return database
}

// scratchMergeDSN creates the scratch database on the server dsn points at and
// returns a DSN naming it instead.
func scratchMergeDSN(t *testing.T, driverName db.Driver, dsn string) string {
	t.Helper()
	var sqlDriver, create, scratch string
	switch driverName {
	case db.DriverPostgres:
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		sqlDriver, create = "pgx", "CREATE DATABASE "+mergeDatabaseName
		u.Path = "/" + mergeDatabaseName
		scratch = u.String()
	case db.DriverMySQL:
		slash, query := strings.Index(dsn, ")/"), strings.Index(dsn, "?")
		if slash < 0 {
			t.Fatalf("mysql DSN %q has no database part", dsn)
		}
		if query < 0 {
			query = len(dsn)
		}
		sqlDriver, create = "mysql", "CREATE DATABASE IF NOT EXISTS "+mergeDatabaseName
		scratch = dsn[:slash+2] + mergeDatabaseName + dsn[query:]
	case db.DriverMSSQL:
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		sqlDriver = "sqlserver"
		create = "IF DB_ID('" + mergeDatabaseName + "') IS NULL CREATE DATABASE " + mergeDatabaseName
		q := u.Query()
		q.Set("database", mergeDatabaseName)
		u.RawQuery = q.Encode()
		scratch = u.String()
	}
	admin, err := sql.Open(sqlDriver, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := admin.Close(); err != nil {
			t.Errorf("closing admin connection: %v", err)
		}
	}()
	if driverName == db.DriverPostgres {
		var exists bool
		if err := admin.QueryRow("SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", mergeDatabaseName).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if exists {
			return scratch
		}
	}
	if _, err := admin.Exec(create); err != nil {
		t.Fatalf("creating the scratch database: %v", err)
	}
	return scratch
}

// createMergeFixture builds the fixture schema. SQLite takes TEXT keys; the external
// backends need a bounded type to index, which changes nothing the merge reads.
func createMergeFixture(t *testing.T, database *sql.DB, driverName db.Driver) {
	t.Helper()
	text := "TEXT"
	if driverName != db.DriverSQLite {
		text = "VARCHAR(191)"
	}
	for _, stmt := range []string{
		`CREATE TABLE environments (id ` + text + ` PRIMARY KEY, name ` + text + `, is_default INTEGER NOT NULL DEFAULT 0, updated_at ` + text + `)`,
		`CREATE TABLE pools (id ` + text + ` PRIMARY KEY, environment_id ` + text + `, name ` + text + `, UNIQUE (environment_id, name))`,
		`CREATE TABLE env_settings (environment_id ` + text + ` PRIMARY KEY, value ` + text + `)`,
		`CREATE TABLE stack_links (environment_id ` + text + ` NOT NULL, stack ` + text + ` NOT NULL, PRIMARY KEY (environment_id, stack))`,
		`CREATE TABLE samples (id ` + text + ` PRIMARY KEY, environment_id ` + text + `, v INTEGER)`,
		`CREATE TABLE secrets (id ` + text + ` PRIMARY KEY, name ` + text + ` NOT NULL, stack_name ` + text + `, environment_id ` + text + `, scope_key ` + text + ` NOT NULL)`,
		`CREATE UNIQUE INDEX idx_secrets_scope_key ON secrets(scope_key)`,
		`CREATE TABLE alert_states (state_key ` + text + ` PRIMARY KEY, alert_id ` + text + ` NOT NULL)`,
		`INSERT INTO environments (id, name) VALUES ('envsrc', 'source'), ('envtgt', 'target')`,
	} {
		if _, err := database.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
}

func mustExec(t *testing.T, database *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := database.Exec(query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func column(t *testing.T, database *sql.DB, query string, args ...any) []string {
	t.Helper()
	rows, err := database.Query(query, args...)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("closing rows: %v", err)
		}
	}()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	sort.Strings(out)
	return out
}

func runMerge(t *testing.T, database *sql.DB, driverName db.Driver) *MergeResult {
	t.Helper()
	schema, err := discoverMergeSchema(context.Background(), database, driverName)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	result := &MergeResult{SourceID: mergeSrc, TargetID: mergeTgt}
	err = db.Tx(database, func(tx *sql.Tx) error {
		return applyMerge(ctx, tx, schema, &Environment{ID: mergeSrc}, &Environment{ID: mergeTgt}, result)
	})
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}
	return result
}

func TestMergeSchemaFindsTheKeysThatDecideCollisions(t *testing.T) {
	forEachMergeBackend(t, mergeSchemaFindsTheKeys)
}

func mergeSchemaFindsTheKeys(t *testing.T, database *sql.DB, driverName db.Driver) {
	schema, err := discoverMergeSchema(context.Background(), database, driverName)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]envColumn{}
	for _, col := range schema.columns {
		got[col.table] = col
	}
	want := map[string]envColumn{
		"env_settings": {table: "env_settings", column: "environment_id", keys: [][]string{{"environment_id"}}},
		"pools":        {table: "pools", column: "environment_id", keys: [][]string{{"environment_id", "name"}}, hasID: true},
		"samples":      {table: "samples", column: "environment_id", hasID: true},
		"secrets":      {table: "secrets", column: "environment_id", hasID: true},
		"stack_links":  {table: "stack_links", column: "environment_id", keys: [][]string{{"environment_id", "stack"}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("schema walk\n got %+v\nwant %+v", got, want)
	}
	if !schema.tables["alert_states"] || schema.tables["environments"] != true {
		t.Fatalf("tables %v", schema.tables)
	}
}

// TestMergeSkipsOnlyTheRowsTheTargetAlreadyHas is the behaviour UPDATE OR IGNORE
// used to give on SQLite alone: every colliding row stays behind, every other row
// moves, and the report names what was left.
func TestMergeSkipsOnlyTheRowsTheTargetAlreadyHas(t *testing.T) {
	forEachMergeBackend(t, mergeSkipsOnlyColliding)
}

func mergeSkipsOnlyColliding(t *testing.T, database *sql.DB, driverName db.Driver) {
	mustExec(t, database, `INSERT INTO pools VALUES ('p1', ?, 'default'), ('p2', ?, 'gpu'), ('p3', ?, 'default')`,
		mergeSrc, mergeSrc, mergeTgt)
	mustExec(t, database, `INSERT INTO env_settings VALUES (?, 'src'), (?, 'tgt')`, mergeSrc, mergeTgt)
	mustExec(t, database, `INSERT INTO stack_links VALUES (?, 'web'), (?, 'db'), (?, 'web')`, mergeSrc, mergeSrc, mergeTgt)
	mustExec(t, database, `INSERT INTO samples VALUES ('m1', ?, 1), ('m2', ?, 2)`, mergeSrc, mergeSrc)

	src, tgt := mergeSrc, mergeTgt
	mustExec(t, database, `INSERT INTO secrets VALUES ('s1', 'token', NULL, ?, ?), ('s2', 'token', NULL, ?, ?), ('s3', 'api', NULL, ?, ?)`,
		mergeSrc, secrets.ScopeKey("token", &src, nil),
		mergeTgt, secrets.ScopeKey("token", &tgt, nil),
		mergeSrc, secrets.ScopeKey("api", &src, nil))
	mustExec(t, database, `INSERT INTO alert_states VALUES (?, 'a1'), (?, 'a1'), (?, 'a2')`,
		"a1:"+mergeSrc+":c1", "a1:"+mergeTgt+":c1", "a2:"+mergeSrc+":c2")

	result := runMerge(t, database, driverName)

	if got := column(t, database, `SELECT id FROM pools WHERE environment_id = ?`, mergeTgt); !reflect.DeepEqual(got, []string{"p2", "p3"}) {
		t.Errorf("target pools %v, want the target's own default plus the moved gpu pool", got)
	}
	if got := column(t, database, `SELECT value FROM env_settings WHERE environment_id = ?`, mergeTgt); !reflect.DeepEqual(got, []string{"tgt"}) {
		t.Errorf("target settings %v, want the target's own row kept", got)
	}
	if got := column(t, database, `SELECT stack FROM stack_links WHERE environment_id = ?`, mergeTgt); !reflect.DeepEqual(got, []string{"db", "web"}) {
		t.Errorf("target stack links %v", got)
	}
	if got := column(t, database, `SELECT id FROM samples WHERE environment_id = ?`, mergeTgt); !reflect.DeepEqual(got, []string{"m1", "m2"}) {
		t.Errorf("target samples %v, want every row of a table with no unique key moved", got)
	}
	if got := column(t, database, `SELECT id FROM secrets WHERE environment_id = ?`, mergeTgt); !reflect.DeepEqual(got, []string{"s2", "s3"}) {
		t.Errorf("target secrets %v", got)
	}
	if got := column(t, database, `SELECT scope_key FROM secrets WHERE id = 's3'`); !reflect.DeepEqual(got, []string{secrets.ScopeKey("api", &tgt, nil)}) {
		t.Errorf("moved secret kept a scope key for the dead environment: %v", got)
	}
	if got := column(t, database, `SELECT state_key FROM alert_states`); !reflect.DeepEqual(got, []string{
		"a1:" + mergeSrc + ":c1", "a1:" + mergeTgt + ":c1", "a2:" + mergeTgt + ":c2",
	}) {
		t.Errorf("alert states %v", got)
	}

	discarded := map[string]int{}
	for _, entry := range result.Discarded {
		discarded[entry.Table+"."+entry.Column] = entry.Rows
	}
	wantDiscarded := map[string]int{
		"pools.environment_id":        1,
		"env_settings.environment_id": 1,
		"stack_links.environment_id":  1,
		"secrets.environment_id":      1,
		"alert_states.state_key":      1,
	}
	if !reflect.DeepEqual(discarded, wantDiscarded) {
		t.Errorf("report discarded %v, want %v", discarded, wantDiscarded)
	}
}

// TestMergeSkipsAcrossPages drives the offset paging with collisions spread over
// several pages, which is where a skipped row could be re-read or an unread one missed.
func TestMergeSkipsAcrossPages(t *testing.T) {
	previous := mergePageSize
	mergePageSize = 2
	t.Cleanup(func() { mergePageSize = previous })

	for _, backend := range mergeBackends {
		for _, table := range []string{"pools", "stack_links"} {
			t.Run(backend.name+"/"+table, func(t *testing.T) {
				mergeSkipsAcrossPages(t, openMergeDB(t, backend), backend.driver, table)
			})
		}
	}
}

func mergeSkipsAcrossPages(t *testing.T, database *sql.DB, driverName db.Driver, table string) {
	for i := 0; i < 9; i++ {
		name := fmt.Sprintf("n%d", i)
		if table == "pools" {
			mustExec(t, database, `INSERT INTO pools VALUES (?, ?, ?)`, "s"+name, mergeSrc, name)
		} else {
			mustExec(t, database, `INSERT INTO stack_links VALUES (?, ?)`, mergeSrc, name)
		}
	}
	for _, i := range []int{0, 3, 4, 8} {
		name := fmt.Sprintf("n%d", i)
		if table == "pools" {
			mustExec(t, database, `INSERT INTO pools VALUES (?, ?, ?)`, "t"+name, mergeTgt, name)
		} else {
			mustExec(t, database, `INSERT INTO stack_links VALUES (?, ?)`, mergeTgt, name)
		}
	}

	nameCol := "name"
	if table == "stack_links" {
		nameCol = "stack"
	}
	result := runMerge(t, database, driverName)

	left := column(t, database, `SELECT `+nameCol+` FROM `+table+` WHERE environment_id = ?`, mergeSrc)
	if !reflect.DeepEqual(left, []string{"n0", "n3", "n4", "n8"}) {
		t.Errorf("left behind %v, want exactly the four colliding rows", left)
	}
	moved := column(t, database, `SELECT `+nameCol+` FROM `+table+` WHERE environment_id = ?`, mergeTgt)
	if len(moved) != 9 {
		t.Errorf("target holds %v, want all nine names once", moved)
	}
	if len(result.Discarded) != 1 || result.Discarded[0].Rows != 4 {
		t.Errorf("report discarded %+v, want four", result.Discarded)
	}
}

// TestMergeAlertStatesTwoKeysRekeyToOneTarget covers the case a batched
// existence check gets wrong: two source keys that re-key to the same new key,
// where the second must see the first's write and be left behind.
func TestMergeAlertStatesTwoKeysRekeyToOneTarget(t *testing.T) {
	forEachMergeBackend(t, mergeAlertStatesRekeyClash)
}

func mergeAlertStatesRekeyClash(t *testing.T, database *sql.DB, driverName db.Driver) {
	first := "a:" + mergeSrc + ":" + mergeTgt + ":x"
	second := "a:" + mergeTgt + ":" + mergeSrc + ":x"
	mustExec(t, database, `INSERT INTO alert_states VALUES (?, 'a'), (?, 'a')`, first, second)

	result := runMerge(t, database, driverName)

	rekeyed := "a:" + mergeTgt + ":" + mergeTgt + ":x"
	if got := column(t, database, `SELECT state_key FROM alert_states WHERE state_key = ?`, rekeyed); len(got) != 1 {
		t.Errorf("re-keyed state rows %v, want exactly one", got)
	}
	if got := column(t, database, `SELECT state_key FROM alert_states`); len(got) != 2 {
		t.Errorf("alert states %v, want both rows kept (one moved, one left behind)", got)
	}
	for _, entry := range result.Discarded {
		if entry.Table == "alert_states" && entry.Rows == 1 {
			return
		}
	}
	t.Errorf("report %v does not name the one alert state left behind", result.Discarded)
}
