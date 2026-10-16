// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package database

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/config"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/sqldialect"
	"github.com/therealmcsparrow/mcharbor/core/telemetry"
)

// Copy-out targets. Each names a server the test may create and drop a scratch
// database on; the database named in the DSN itself is never written.
const (
	copyOutPostgresEnv = "MCHARBOR_TEST_PG_DSN"
	copyOutMySQLEnv    = "MCHARBOR_TEST_MYSQL_DSN"
	copyOutScratchName = "mcharbor_copyout_test"
)

// copyOutBigBytes is past 2 GiB, so a 32-bit column anywhere on the way truncates it.
const copyOutBigBytes int64 = 5_000_000_000

// syncBuffer is a log sink the migration goroutine and the test can share.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestMigrationCopiesOffMSSQL fills SQL Server from a SQLite fixture, boots on it the
// way main.go does, and copies it out to SQLite, PostgreSQL and MySQL through the job
// POST /api/database/migrate starts, comparing every table's count and sample values.
func TestMigrationCopiesOffMSSQL(t *testing.T) {
	dsn := os.Getenv(mssqlCopyDSNEnv)
	if dsn == "" {
		t.Skip(mssqlCopyDSNEnv + " is not set")
	}
	ctx := context.Background()
	dir := t.TempDir()

	src := openCopyOutFixture(t, filepath.Join(dir, "fixture.db"))
	logs := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	enc, err := encryption.New(dir, "")
	if err != nil {
		t.Fatalf("building the encryption service: %v", err)
	}
	inSvc := NewService(&config.Config{DataDir: filepath.Join(dir, "in"), DatabasePath: filepath.Join(dir, "fixture.db")}, enc, src, nil)
	if err := os.MkdirAll(filepath.Join(dir, "in"), 0o700); err != nil {
		t.Fatal(err)
	}
	status := runCopyMigration(t, NewMigrator(inSvc, src, logger, nil), ConnectionInput{Driver: driverMSSQL, DSN: dsn})
	if strings.Contains(logs.String(), "would not suspend") {
		t.Fatalf("copying into SQL Server logged a suspension failure:\n%s", logs.String())
	}
	for _, w := range status.Warnings {
		if strings.Contains(w, "would not suspend") {
			t.Fatalf("copying into SQL Server warned: %v", status.Warnings)
		}
	}

	target, err := openMSSQLTarget(dsn)
	if err != nil {
		t.Fatalf("opening the SQL Server target: %v", err)
	}
	defer closeOrFail(t, target)
	assertCatchUpFitsSQLServer(t, src, target)
	assertIdentityColumnsCopy(t, target)

	coredb.SetConnectorWrapper(func(c driver.Connector, backend coredb.Driver) driver.Connector {
		return telemetry.WrapDBConnectorWithDialect(c, string(backend), telemetry.Dialect{
			Rewrite: sqldialect.MSSQL, Args: sqldialect.MSSQLArgs, Value: sqldialect.MSSQLValue})
	})
	source, err := coredb.Open(coredb.Config{Driver: coredb.DriverMSSQL, DSN: sqldialect.MSSQLDSN(dsn)})
	if err != nil {
		coredb.SetConnectorWrapper(nil)
		t.Fatalf("booting on SQL Server: %v", err)
	}
	defer func() {
		if err := coredb.Close(); err != nil {
			t.Errorf("closing the SQL Server source: %v", err)
		}
		coredb.SetConnectorWrapper(nil)
	}()

	version, err := coredb.ServerVersion(ctx, source, coredb.DriverMSSQL)
	if err != nil || !strings.Contains(version, "Microsoft SQL Server") || strings.Contains(version, "\n") {
		t.Fatalf("server version %q, %v", version, err)
	}
	refs, err := coredb.TableReferences(ctx, source, coredb.DriverMSSQL, "container_backup_runs")
	if err != nil {
		t.Fatalf("reading container_backup_runs references: %v", err)
	}
	if !containsFold(refs, "environments") || !containsFold(refs, "container_backup_plans") {
		t.Fatalf("container_backup_runs references %v; the trigger-carried cascade edge is missing", refs)
	}
	if key, err := coredb.PrimaryKeyColumn(ctx, source, coredb.DriverMSSQL, "capacity_samples"); err != nil || key != "id" {
		t.Fatalf("capacity_samples key %q, %v", key, err)
	}

	targets := []struct {
		name  string
		input func(t *testing.T) (ConnectionInput, *sql.DB)
	}{
		{"sqlite", func(t *testing.T) (ConnectionInput, *sql.DB) {
			path := filepath.Join(dir, "out", "mcharbor.db")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			verify, err := sql.Open("sqlite", "file:"+path)
			if err != nil {
				t.Fatal(err)
			}
			return ConnectionInput{Driver: driverSQLite, Path: path}, verify
		}},
		{"postgres", func(t *testing.T) (ConnectionInput, *sql.DB) {
			scratch := copyOutScratch(t, driverPostgres, os.Getenv(copyOutPostgresEnv))
			verify, err := sql.Open("pgx", scratch)
			if err != nil {
				t.Fatal(err)
			}
			return ConnectionInput{Driver: driverPostgres, DSN: scratch}, verify
		}},
		{"mysql", func(t *testing.T) (ConnectionInput, *sql.DB) {
			scratch := copyOutScratch(t, driverMySQL, os.Getenv(copyOutMySQLEnv))
			sep := "&"
			if strings.HasSuffix(scratch, "?") {
				sep = ""
			}
			verify, err := sql.Open("mysql", scratch+sep+"parseTime=true")
			if err != nil {
				t.Fatal(err)
			}
			return ConnectionInput{Driver: driverMySQL, DSN: scratch}, verify
		}},
	}
	for _, tc := range targets {
		t.Run(tc.name, func(t *testing.T) {
			if (tc.name == "postgres" && os.Getenv(copyOutPostgresEnv) == "") || (tc.name == "mysql" && os.Getenv(copyOutMySQLEnv) == "") {
				t.Skip("no DSN for " + tc.name)
			}
			input, verify := tc.input(t)
			defer closeOrFail(t, verify)
			outDir := filepath.Join(dir, "out-"+tc.name)
			if err := os.MkdirAll(outDir, 0o700); err != nil {
				t.Fatal(err)
			}
			svc := NewService(&config.Config{DataDir: outDir, DatabasePath: filepath.Join(outDir, "unused.db")}, enc, source, nil)
			status := runCopyMigration(t, NewMigrator(svc, source, logger, nil), input)
			t.Logf("%s: %d rows across %d tables; warnings: %v", tc.name, status.RowsCopied, status.TableTotal, status.Warnings)
			assertCopiedOut(t, source, verify, coredb.Driver(tc.name))
		})
	}
}

// openCopyOutFixture seeds the SQLite the round trip starts from: Unicode, NULLs,
// integers past 2 GiB, text and DATETIME timestamps, and a table whose cascade SQL
// Server can only carry in a trigger.
func openCopyOutFixture(t *testing.T, path string) *sql.DB {
	t.Helper()
	src, err := sql.Open("sqlite", "file:"+path+"?_foreign_keys=ON")
	if err != nil {
		t.Fatalf("opening the fixture: %v", err)
	}
	t.Cleanup(func() { closeOrFail(t, src) })
	if err := coredb.Migrate(src, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrating the fixture: %v", err)
	}
	for _, stmt := range []string{
		`INSERT INTO environments (id, name) VALUES ('env-out', 'Omgeving ☃ “out”')`,
		`INSERT INTO users (id, username, email, password_hash, display_name, last_login)
		 VALUES ('user-out', 'outbound', NULL, 'hash', 'Zoë 日本語', '2026-09-27T21:40:00.123Z')`,
		`INSERT INTO container_backup_plans (id, name, environment_id, container_id, created_at, updated_at)
		 VALUES ('plan-out', 'nightly', 'env-out', 'c1', '2026-09-27T21:40:00Z', '2026-09-27T21:40:00Z')`,
		`INSERT INTO container_backup_runs (id, plan_id, environment_id, container_id, status, archive_size, error, started_at, created_at, updated_at)
		 VALUES ('run-out', 'plan-out', 'env-out', 'c1', 'success', 3000000000, NULL, '2026-09-27T21:40:01Z', '2026-09-27T21:40:01Z', '2026-09-27T21:40:02Z')`,
		`INSERT INTO workflow_kv (id, key, value, created_at, updated_at)
		 VALUES ('kv-out', 'copy.out', 'waarde ✓', '2026-09-27 21:40:03', '2026-09-27 21:40:04')`,
	} {
		if _, err := src.Exec(stmt); err != nil {
			t.Fatalf("seeding %q: %v", stmt, err)
		}
	}
	for i := 0; i < 1200; i++ {
		if _, err := src.Exec(`INSERT INTO capacity_samples (id, environment_id, subject_kind, subject_id, sampled_at, used_bytes, total_bytes)
			VALUES (?, 'env-out', 'volume', 'vol', ?, ?, ?)`,
			fmt.Sprintf("cap-out-%04d", i), time.Unix(int64(i)*3600, 0).UTC().Format(time.RFC3339), copyOutBigBytes+int64(i), copyOutBigBytes*2); err != nil {
			t.Fatalf("seeding capacity sample %d: %v", i, err)
		}
	}
	return src
}

// runCopyMigration starts a migration and waits for it, failing the test unless it ends done.
func runCopyMigration(t *testing.T, migrator *Migrator, target ConnectionInput) *MigrationStatus {
	t.Helper()
	restart := false
	if _, err := migrator.Start(context.Background(), MigrationRequest{Target: target, RestartAfter: &restart}); err != nil {
		t.Fatalf("starting the migration: %v", err)
	}
	deadline := time.Now().Add(10 * time.Minute)
	var status *MigrationStatus
	for time.Now().Before(deadline) {
		status = migrator.Status()
		if status != nil && !status.Running {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if status == nil || status.Phase != phaseDone {
		t.Fatalf("migration to %s did not finish: %+v", target.Driver, status)
	}
	return status
}

// assertCatchUpFitsSQLServer adds more late rows than one 2,100-parameter statement
// holds and runs the catch-up pass the migrator runs, with no BatchRows given.
func assertCatchUpFitsSQLServer(t *testing.T, src, target *sql.DB) {
	t.Helper()
	for i := 0; i < 700; i++ {
		if _, err := src.Exec(`INSERT INTO capacity_samples (id, environment_id, subject_kind, subject_id, sampled_at, used_bytes, total_bytes)
			VALUES (?, 'env-out', 'volume', 'late', ?, ?, 0)`,
			fmt.Sprintf("cap-late-%04d", i), time.Unix(int64(i)*60, 0).UTC().Format(time.RFC3339), copyOutBigBytes); err != nil {
			t.Fatalf("seeding late sample %d: %v", i, err)
		}
	}
	report, err := coredb.CopyDelta(context.Background(), src, coredb.DriverSQLite, target, coredb.DriverMSSQL, coredb.DeltaOptions{})
	if err != nil {
		t.Fatalf("catch-up into SQL Server: %v", err)
	}
	if report.Copied["capacity_samples"] != 700 {
		t.Fatalf("catch-up copied %v, want 700 capacity samples", report.Copied)
	}
}

// assertIdentityColumnsCopy copies into a SQL Server table with an IDENTITY key,
// which is refused without IDENTITY_INSERT, and checks the explicit ids survive.
func assertIdentityColumnsCopy(t *testing.T, target *sql.DB) {
	t.Helper()
	const table = "copyout_identity_probe"
	if _, err := target.Exec(`IF OBJECT_ID('` + table + `') IS NOT NULL DROP TABLE ` + table); err != nil {
		t.Fatal(err)
	}
	if _, err := target.Exec(`CREATE TABLE ` + table + ` (id BIGINT IDENTITY(1,1) PRIMARY KEY, label NVARCHAR(100))`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := execIgnore(target, `DROP TABLE `+table); err != nil {
			t.Logf("dropping %s: %v", table, err)
		}
	}()

	probe, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer closeOrFail(t, probe)
	probe.SetMaxOpenConns(1)
	for _, stmt := range []string{
		`CREATE TABLE ` + table + ` (id INTEGER PRIMARY KEY AUTOINCREMENT, label TEXT)`,
		`INSERT INTO ` + table + ` (id, label) VALUES (7, 'seven'), (3000000000, 'big')`,
	} {
		if _, err := probe.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	report, err := coredb.CopyAll(context.Background(), probe, coredb.DriverSQLite, target, coredb.DriverMSSQL, coredb.CopyOptions{})
	if err != nil {
		t.Fatalf("copying into an IDENTITY table: %v", err)
	}
	if report.TotalRows != 2 || !report.ForeignKeysRelaxed {
		t.Fatalf("identity copy report %+v", report)
	}
	var label string
	if err := target.QueryRow(`SELECT label FROM `+table+` WHERE id = ?`, int64(3000000000)).Scan(&label); err != nil || label != "big" {
		t.Fatalf("identity row: %q, %v", label, err)
	}
}

func execIgnore(database *sql.DB, stmt string) error {
	_, err := database.Exec(stmt)
	return err
}

// copyOutScratch recreates the scratch database on the server dsn names and returns
// a DSN pointing at it.
func copyOutScratch(t *testing.T, driverName, dsn string) string {
	t.Helper()
	switch driverName {
	case driverPostgres:
		admin, err := sql.Open("pgx", dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer closeOrFail(t, admin)
		for _, stmt := range []string{"DROP DATABASE IF EXISTS " + copyOutScratchName + " WITH (FORCE)", "CREATE DATABASE " + copyOutScratchName} {
			if _, err := admin.Exec(stmt); err != nil {
				t.Fatalf("%s: %v", stmt, err)
			}
		}
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		u.Path = "/" + copyOutScratchName
		return u.String()
	default:
		admin, err := sql.Open("mysql", dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer closeOrFail(t, admin)
		for _, stmt := range []string{"DROP DATABASE IF EXISTS " + copyOutScratchName, "CREATE DATABASE " + copyOutScratchName + " CHARACTER SET utf8mb4 COLLATE utf8mb4_bin"} {
			if _, err := admin.Exec(stmt); err != nil {
				t.Fatalf("%s: %v", stmt, err)
			}
		}
		slash, query := strings.Index(dsn, ")/"), strings.Index(dsn, "?")
		if slash < 0 {
			t.Fatalf("mysql DSN %q has no database part", dsn)
		}
		rest := "?"
		if query >= 0 {
			rest = dsn[query:]
		}
		return dsn[:slash+2] + copyOutScratchName + rest
	}
}

// assertCopiedOut compares every table's count with SQL Server and reads the fixture's
// awkward values back in the target's own dialect.
func assertCopiedOut(t *testing.T, source, verify *sql.DB, targetDriver coredb.Driver) {
	t.Helper()
	ctx := context.Background()
	tables, err := coredb.ListTables(ctx, source, coredb.DriverMSSQL)
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range tables {
		if table == "schema_migrations" || table == "_migrations" {
			continue
		}
		want, err := coredb.RowCount(ctx, source, coredb.DriverMSSQL, table)
		if err != nil {
			t.Fatal(err)
		}
		got, err := coredb.RowCount(ctx, verify, targetDriver, table)
		if err != nil {
			t.Fatalf("counting %s on %s: %v", table, targetDriver, err)
		}
		if got != want {
			t.Errorf("%s: %d rows on %s, %d on SQL Server", table, got, targetDriver, want)
		}
	}

	var count, maxUsed int64
	if err := verify.QueryRow(`SELECT COUNT(*), MAX(used_bytes) FROM capacity_samples`).Scan(&count, &maxUsed); err != nil {
		t.Fatal(err)
	}
	if count != 1900 || maxUsed != copyOutBigBytes+1199 {
		t.Fatalf("capacity samples: %d rows, max %d", count, maxUsed)
	}

	var name, lastLogin string
	var email sql.NullString
	if err := verify.QueryRow(`SELECT display_name, last_login, email FROM users WHERE id = 'user-out'`).Scan(&name, &lastLogin, &email); err != nil {
		t.Fatal(err)
	}
	if name != "Zoë 日本語" || lastLogin != "2026-09-27T21:40:00.123Z" || email.Valid {
		t.Fatalf("user row changed in transit: %q %q %v", name, lastLogin, email)
	}

	var envName string
	if err := verify.QueryRow(`SELECT name FROM environments WHERE id = 'env-out'`).Scan(&envName); err != nil || envName != "Omgeving ☃ “out”" {
		t.Fatalf("environment row: %q %v", envName, err)
	}

	var planID sql.NullString
	var size int64
	var runErr sql.NullString
	if err := verify.QueryRow(`SELECT plan_id, archive_size, error FROM container_backup_runs WHERE id = 'run-out'`).Scan(&planID, &size, &runErr); err != nil {
		t.Fatal(err)
	}
	if planID.String != "plan-out" || size != 3000000000 || runErr.Valid {
		t.Fatalf("backup run: %v %d %v", planID, size, runErr)
	}

	var value string
	var created time.Time
	if err := verify.QueryRow(`SELECT value, created_at FROM workflow_kv WHERE id = 'kv-out'`).Scan(&value, &created); err != nil {
		t.Fatalf("reading workflow_kv on %s: %v", targetDriver, err)
	}
	if value != "waarde ✓" || !created.UTC().Equal(time.Date(2026, 9, 27, 21, 40, 3, 0, time.UTC)) {
		t.Fatalf("workflow_kv: %q %s", value, created)
	}
}

func containsFold(list []string, want string) bool {
	for _, v := range list {
		if strings.EqualFold(v, want) {
			return true
		}
	}
	return false
}
