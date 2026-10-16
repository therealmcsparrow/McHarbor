// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package fleetmeasure

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

const (
	// envMeasure gates every harness in this package.
	envMeasure = "MCHARBOR_FLEET_MEASURE"
	// envTargetDB points the database harness at an existing copy: a SQLite
	// file path, or a postgres:// DSN. Unset means seed the synthetic fixture.
	envTargetDB = "MCHARBOR_FLEET_TARGET_DB"
	// envTargetDriver overrides the driver inferred from envTargetDB.
	envTargetDriver = "MCHARBOR_FLEET_TARGET_DRIVER"
	// envTargetIsCopy is the operator's assertion that envTargetDB is a copy.
	// The harness refuses to open a target without it, because a measurement
	// against a live instance's database is the one mistake with real cost.
	envTargetIsCopy = "MCHARBOR_FLEET_TARGET_IS_COPY"
)

// writerSettleWindow is how long the live-instance check waits to see whether
// a clock column advances. Two seconds covers the fastest writer McHarbor has
// (the 1s container-events collector) with margin.
const writerSettleWindow = 2 * time.Second

// target is the database a harness measures against, plus the driver, because
// every plan and every bind marker below differs between the two engines.
type target struct {
	db     *sql.DB
	driver coredb.Driver
	label  string
	// seeded is true for the synthetic fixture, which may be written to.
	seeded bool
}

// clockColumn names a table and the column a running instance stamps, used
// both to detect an advancing writer and to report the target's date range.
type clockColumn struct {
	table, column string
}

// clockColumns are the tables a live McHarbor writes to continuously, most
// frequent first, so the advancing-writer check finds a writer fastest.
var clockColumns = []clockColumn{
	{"container_metrics", "sampled_at"},
	{"host_metrics", "timestamp"},
	{"agent_logs", "logged_at"},
	{"container_events", "timestamp"},
	{"api_key_usage_events", "occurred_at"},
	{"lifecycle_events", "timestamp"},
	{"audit_logs", "timestamp"},
	{"in_app_notifications", "created_at"},
	{"container_backup_log", "created_at"},
	{"alert_events", "created_at"},
	{"workflow_runs", "started_at"},
	{"scans", "created_at"},
}

// openTarget returns the existing database named by envTargetDB, opened
// read-only, or nil when the variable is unset so the caller seeds the fixture.
func openTarget(t *testing.T) *target {
	t.Helper()
	spec := strings.TrimSpace(os.Getenv(envTargetDB))
	if spec == "" {
		return nil
	}
	if os.Getenv(envTargetIsCopy) != "1" {
		t.Fatalf("%s is set but %s is not 1: this harness runs only against a copy, never a live instance's database",
			envTargetDB, envTargetIsCopy)
	}

	driver := coredb.Driver(strings.TrimSpace(os.Getenv(envTargetDriver)))
	if driver == "" {
		driver = inferDriver(spec)
	}

	var tgt *target
	switch driver {
	case coredb.DriverSQLite:
		tgt = openSQLiteTarget(t, spec)
	case coredb.DriverPostgres:
		tgt = openPostgresTarget(t, spec)
	default:
		t.Fatalf("%s=%q is not supported by this harness (use %q or %q)",
			envTargetDriver, driver, coredb.DriverSQLite, coredb.DriverPostgres)
	}

	refuseIfLive(t, tgt)
	t.Logf("target: %s (%s), read-only", tgt.label, tgt.driver)
	return tgt
}

// inferDriver reads the driver out of the target spec; only Postgres DSNs
// carry a scheme, so anything else is a SQLite file path.
func inferDriver(spec string) coredb.Driver {
	lower := strings.ToLower(spec)
	if strings.HasPrefix(lower, "postgres://") || strings.HasPrefix(lower, "postgresql://") {
		return coredb.DriverPostgres
	}
	return coredb.DriverSQLite
}

// openSQLiteTarget opens the copy with mode=ro so SQLite itself rejects a
// write, and without the WAL and directory setup core/db performs — creating a
// -wal or -shm beside the copy would already be a mutation.
func openSQLiteTarget(t *testing.T, path string) *target {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("target database %s: %v", abs, err)
	}
	dsn := "file:" + filepath.ToSlash(abs) + "?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("opening %s read-only: %v", abs, err)
	}
	// One connection, so the query_only pragma cannot be bypassed by a second
	// connection the pool opens without it.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		closeutil.Report(db.Close(), "db")
		t.Fatalf("pinging %s: %v", abs, err)
	}
	t.Cleanup(func() { closeutil.Report(db.Close(), "db") })
	assertReadOnly(t, db, coredb.DriverSQLite)
	return &target{db: db, driver: coredb.DriverSQLite, label: abs}
}

// openPostgresTarget opens the copy with the session pinned read-only, on a
// single connection so every statement runs under that setting.
func openPostgresTarget(t *testing.T, dsn string) *target {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("opening postgres target: %v", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		closeutil.Report(db.Close(), "db")
		t.Fatalf("pinging postgres target: %v", err)
	}
	t.Cleanup(func() { closeutil.Report(db.Close(), "db") })
	if _, err := db.Exec("SET SESSION CHARACTERISTICS AS TRANSACTION READ ONLY"); err != nil {
		t.Fatalf("pinning the postgres session read-only: %v", err)
	}
	var name string
	if err := db.QueryRow("SELECT current_database()").Scan(&name); err != nil {
		t.Fatalf("reading current_database(): %v", err)
	}
	assertReadOnly(t, db, coredb.DriverPostgres)
	return &target{db: db, driver: coredb.DriverPostgres, label: "postgres/" + name}
}

// assertReadOnly proves the connection cannot write before any measurement
// runs, rather than trusting the DSN to have been honoured.
func assertReadOnly(t *testing.T, db *sql.DB, driver coredb.Driver) {
	t.Helper()
	probe := `CREATE TABLE mcharbor_fleetmeasure_write_probe (id integer)`
	if _, err := db.Exec(probe); err == nil {
		if _, dropErr := db.Exec(`DROP TABLE mcharbor_fleetmeasure_write_probe`); dropErr != nil {
			t.Logf("dropping the write probe table: %v", dropErr)
		}
		t.Fatalf("the %s target accepted a write: refusing to measure a database this harness could damage", driver)
	}
}

// refuseIfLive fails the test when the target shows any sign of being in use:
// another connection, a live install's data directory beside it, or a clock
// column that advances while the harness watches.
func refuseIfLive(t *testing.T, tgt *target) {
	t.Helper()
	switch tgt.driver {
	case coredb.DriverSQLite:
		refuseIfSQLiteLive(t, tgt.label)
	case coredb.DriverPostgres:
		refuseIfPostgresLive(t, tgt.db)
	}
	refuseIfWriterAdvancing(t, tgt)
}

// refuseIfSQLiteLive rejects a target whose WAL sidecars or neighbouring
// directories say it is a running install's database rather than a copy.
func refuseIfSQLiteLive(t *testing.T, path string) {
	t.Helper()
	if info, err := os.Stat(path + "-shm"); err == nil {
		t.Fatalf("%s-shm exists (%d bytes): a WAL shared-memory file means a connection is, or was, open on this database. Take a fresh copy with `sqlite3 <live.db> \"VACUUM INTO 'copy.db'\"` and point %s at that.",
			path, info.Size(), envTargetDB)
	}
	if info, err := os.Stat(path + "-wal"); err == nil && info.Size() > 0 {
		t.Fatalf("%s-wal is %d bytes: this database has uncheckpointed writes and is not a quiesced copy. Use `VACUUM INTO` to produce one.",
			path, info.Size())
	}
	dir := filepath.Dir(path)
	for _, sibling := range []string{"plugins", "secrets", "backups", "uploads"} {
		if info, err := os.Stat(filepath.Join(dir, sibling)); err == nil && info.IsDir() {
			t.Fatalf("%s sits next to a %s/ directory, which makes %s a live install's data directory. Copy the database somewhere else and point %s at the copy.",
				filepath.Base(path), sibling, dir, envTargetDB)
		}
	}
}

// refuseIfPostgresLive rejects a target another backend is connected to, which
// is the definitive test for a Postgres database still serving an instance.
func refuseIfPostgresLive(t *testing.T, db *sql.DB) {
	t.Helper()
	var others int
	err := db.QueryRow(`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND pid <> pg_backend_pid()`).Scan(&others)
	if err != nil {
		t.Fatalf("checking pg_stat_activity for other connections: %v", err)
	}
	if others > 0 {
		t.Fatalf("%d other connection(s) are open on this database: restore a dump into a database of its own and point %s at that", others, envTargetDB)
	}
}

// refuseIfWriterAdvancing samples the newest timestamp the target carries,
// waits, and samples again. A value that moved proves something is writing,
// which no filename or connection count can rule out on its own.
func refuseIfWriterAdvancing(t *testing.T, tgt *target) {
	t.Helper()
	before := newestStamp(t, tgt)
	if before.table == "" {
		return
	}
	time.Sleep(writerSettleWindow)
	after := newestStamp(t, tgt)
	if after.value != before.value {
		t.Fatalf("%s.%s advanced from %q to %q in %s: a writer is active on this database, so it is not a copy",
			before.table, before.column, before.value, after.value, writerSettleWindow)
	}
}

type stamp struct {
	table, column, value string
}

// newestStamp returns the newest value from the first clock column the target
// actually has, or a zero stamp when it has none of them.
func newestStamp(t *testing.T, tgt *target) stamp {
	t.Helper()
	present := targetTables(t, tgt)
	for _, cc := range clockColumns {
		if !present[cc.table] {
			continue
		}
		var v sql.NullString
		q := fmt.Sprintf("SELECT MAX(%s) FROM %s",
			coredb.QuoteIdent(tgt.driver, cc.column), coredb.QuoteIdent(tgt.driver, cc.table))
		if err := tgt.db.QueryRow(q).Scan(&v); err != nil {
			continue
		}
		if !v.Valid {
			continue
		}
		return stamp{table: cc.table, column: cc.column, value: v.String}
	}
	return stamp{}
}

var targetTableCache map[string]bool

// targetTables reports which tables the target has, so a copy from an older
// schema is measured for what it holds instead of failing on a missing table.
func targetTables(t *testing.T, tgt *target) map[string]bool {
	t.Helper()
	if targetTableCache != nil {
		return targetTableCache
	}
	names, err := coredb.ListTables(context.Background(), tgt.db, tgt.driver)
	if err != nil {
		t.Fatalf("listing tables on the target: %v", err)
	}
	targetTableCache = make(map[string]bool, len(names))
	for _, n := range names {
		targetTableCache[n] = true
	}
	return targetTableCache
}

// reportShape logs the target's row counts, date ranges and environment_id
// skew, which is what makes a real measurement interpretable: a plan that is
// right on uniform data can be wrong on a lopsided fleet.
func reportShape(t *testing.T, tgt *target) {
	t.Helper()
	present := targetTables(t, tgt)
	var lines []string
	for _, cc := range clockColumns {
		if !present[cc.table] {
			lines = append(lines, fmt.Sprintf("%-24s absent", cc.table))
			continue
		}
		col := coredb.QuoteIdent(tgt.driver, cc.column)
		tbl := coredb.QuoteIdent(tgt.driver, cc.table)
		var n int64
		var lo, hi sql.NullString
		q := fmt.Sprintf("SELECT COUNT(*), MIN(%s), MAX(%s) FROM %s", col, col, tbl)
		if err := tgt.db.QueryRow(q).Scan(&n, &lo, &hi); err != nil {
			lines = append(lines, fmt.Sprintf("%-24s error: %v", cc.table, err))
			continue
		}
		lines = append(lines, fmt.Sprintf("%-24s %10d rows  %s .. %s", cc.table, n, lo.String, hi.String))
		lines = append(lines, envSkew(tgt, cc.table)...)
	}
	t.Log("target shape\n" + strings.Join(lines, "\n"))
}

// envSkew lists the busiest environments for a table, the number the synthetic
// fixture cannot produce because it spreads rows evenly across all 20.
func envSkew(tgt *target, table string) []string {
	if !hasColumn(tgt, table, "environment_id") {
		return nil
	}
	q := fmt.Sprintf("SELECT %s, COUNT(*) AS c FROM %s GROUP BY 1 ORDER BY c DESC LIMIT 5",
		coredb.QuoteIdent(tgt.driver, "environment_id"), coredb.QuoteIdent(tgt.driver, table))
	rows, err := tgt.db.Query(q)
	if err != nil {
		return nil
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	var out []string
	for rows.Next() {
		var env sql.NullString
		var c int64
		if err := rows.Scan(&env, &c); err != nil {
			return out
		}
		out = append(out, fmt.Sprintf("%-24s   env %-20s %10d", "", env.String, c))
	}
	if err := rows.Err(); err != nil {
		return out
	}
	return out
}

// rebind renumbers ? placeholders for Postgres. The probe queries carry no ?
// inside a string literal, so a positional walk is sufficient here.
func rebind(driver coredb.Driver, q string) string {
	if driver != coredb.DriverPostgres {
		return q
	}
	var b strings.Builder
	n := 0
	for _, r := range q {
		if r == '?' {
			n++
			fmt.Fprintf(&b, "$%d", n)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// explainPlan returns the chosen plan as one line. The two engines disagree on
// both the statement and the shape of its result, so each is read separately.
func explainPlan(ctx context.Context, tgt *target, q string, args []any) (string, error) {
	if tgt.driver == coredb.DriverPostgres {
		rows, err := tgt.db.QueryContext(ctx, "EXPLAIN (COSTS OFF) "+rebind(tgt.driver, q), args...)
		if err != nil {
			return "", err
		}
		defer func() { closeutil.Report(rows.Close(), "rows") }()
		var steps []string
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				return "", err
			}
			steps = append(steps, strings.TrimSpace(line))
		}
		return strings.Join(steps, " | "), rows.Err()
	}
	rows, err := tgt.db.QueryContext(ctx, "EXPLAIN QUERY PLAN "+q, args...)
	if err != nil {
		return "", err
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	var steps []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			return "", err
		}
		steps = append(steps, detail)
	}
	return strings.Join(steps, " | "), rows.Err()
}

// scanFlags marks the two shapes worth looking for in a plan — a sort the
// index did not satisfy, and a table scan — in either engine's wording.
func scanFlags(driver coredb.Driver, qp string) []string {
	var flags []string
	if driver == coredb.DriverPostgres {
		if strings.Contains(qp, "Sort") {
			flags = append(flags, "SORT")
		}
		if strings.Contains(qp, "Seq Scan") {
			flags = append(flags, "FULL-SCAN")
		}
		return flags
	}
	if strings.Contains(qp, "USE TEMP B-TREE") {
		flags = append(flags, "SORT")
	}
	for _, step := range strings.Split(qp, " | ") {
		if strings.HasPrefix(step, "SCAN ") && !strings.Contains(step, "USING") && !strings.Contains(step, "rows") {
			flags = append(flags, "FULL-SCAN")
			break
		}
	}
	return flags
}
