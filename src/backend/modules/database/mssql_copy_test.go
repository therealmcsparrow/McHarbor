// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package database

import (
	"context"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/config"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
)

// mssqlCopyDSNEnv names the SQL Server the copy test writes into. The database it
// names is emptied and refilled, so it must be a throwaway one; a freshly created one
// also exercises the rows Migrate seeds, which the copy has to clear around.
const mssqlCopyDSNEnv = "MCHARBOR_TEST_MSSQL_DSN"

// TestMigrationCopiesSQLiteIntoMSSQL runs the job POST /api/database/migrate starts,
// from a seeded SQLite file into a real SQL Server, and reads the rows back.
func TestMigrationCopiesSQLiteIntoMSSQL(t *testing.T) {
	dsn := os.Getenv(mssqlCopyDSNEnv)
	if dsn == "" {
		t.Skip(mssqlCopyDSNEnv + " is not set")
	}

	dir := t.TempDir()
	srcPath := filepath.Join(dir, "source.db")
	src, err := sql.Open("sqlite", "file:"+srcPath+"?_foreign_keys=ON")
	if err != nil {
		t.Fatalf("opening the source: %v", err)
	}
	defer closeOrFail(t, src)
	if err := coredb.Migrate(src, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrating the source: %v", err)
	}

	const bigBytes int64 = 5_000_000_000
	seed := []string{
		`INSERT INTO environments (id, name) VALUES ('env-fixture', 'Fixture ☃ omgeving')`,
		`INSERT INTO users (id, username, email, password_hash, display_name, last_login)
		 VALUES ('user-fixture', 'fixture', NULL, 'hash', 'Zoë “quotes”', '2026-09-27T21:10:00.123Z')`,
	}
	for _, stmt := range seed {
		if _, err := src.Exec(stmt); err != nil {
			t.Fatalf("seeding %q: %v", stmt, err)
		}
	}
	for i := 0; i < 1200; i++ {
		if _, err := src.Exec(`INSERT INTO capacity_samples (id, environment_id, subject_kind, subject_id, sampled_at, used_bytes, total_bytes)
			VALUES (?, 'env-fixture', 'volume', 'vol', ?, ?, ?)`,
			"cap-"+time.Unix(int64(i), 0).UTC().Format("150405"), time.Unix(int64(i)*3600, 0).UTC().Format(time.RFC3339), bigBytes+int64(i), bigBytes*2); err != nil {
			t.Fatalf("seeding capacity sample %d: %v", i, err)
		}
	}

	enc, err := encryption.New(dir, "")
	if err != nil {
		t.Fatalf("building the encryption service: %v", err)
	}
	cfg := &config.Config{DataDir: dir, DatabasePath: srcPath}
	svc := NewService(cfg, enc, src, nil)
	migrator := NewMigrator(svc, src, nil, nil)

	restart := false
	if _, err := migrator.Start(context.Background(), MigrationRequest{Target: ConnectionInput{Driver: driverMSSQL, DSN: dsn}, RestartAfter: &restart}); err != nil {
		t.Fatalf("starting the migration: %v", err)
	}
	deadline := time.Now().Add(10 * time.Minute)
	var status *MigrationStatus
	for time.Now().Before(deadline) {
		status = migrator.Status()
		if status != nil && !status.Running {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if status == nil || status.Phase != phaseDone {
		t.Fatalf("migration did not finish: %+v", status)
	}
	t.Logf("copied %d rows across %d tables; warnings: %v", status.RowsCopied, status.TableTotal, status.Warnings)

	stored, err := svc.Current(context.Background())
	if err != nil {
		t.Fatalf("reading the source config: %v", err)
	}
	if stored.Driver != driverSQLite {
		t.Fatalf("the running driver changed under the copy: %s", stored.Driver)
	}

	target, err := openMSSQLTarget(dsn)
	if err != nil {
		t.Fatalf("reopening the target: %v", err)
	}
	defer closeOrFail(t, target)

	// The restart into the new database runs Migrate again; it must find nothing to do.
	if err := coredb.Migrate(target, coredb.DriverMSSQL); err != nil {
		t.Fatalf("re-running Migrate as the restarted instance would: %v", err)
	}

	var count, maxUsed int64
	if err := target.QueryRow(`SELECT COUNT(*), MAX(used_bytes) FROM capacity_samples`).Scan(&count, &maxUsed); err != nil {
		t.Fatalf("reading capacity samples back: %v", err)
	}
	if count != 1200 || maxUsed != bigBytes+1199 {
		t.Fatalf("capacity samples: got %d rows, max %d", count, maxUsed)
	}

	var name, lastLogin string
	var email sql.NullString
	if err := target.QueryRow(`SELECT display_name, last_login, email FROM users WHERE id = ?`, "user-fixture").Scan(&name, &lastLogin, &email); err != nil {
		t.Fatalf("reading the user back: %v", err)
	}
	if name != "Zoë “quotes”" || lastLogin != "2026-09-27T21:10:00.123Z" || email.Valid {
		t.Fatalf("user row changed in transit: %q %q %v", name, lastLogin, email)
	}

	var envName string
	if err := target.QueryRow(`SELECT name FROM environments WHERE id = ?`, "env-fixture").Scan(&envName); err != nil || envName != "Fixture ☃ omgeving" {
		t.Fatalf("environment row: %q %v", envName, err)
	}

	var identities int
	if err := target.QueryRow(`SELECT COUNT(*) FROM sys.identity_columns ic JOIN sys.tables tb ON tb.object_id = ic.object_id
		WHERE tb.name <> 'schema_migrations'`).Scan(&identities); err != nil {
		t.Fatalf("listing identity columns: %v", err)
	}
	if identities != 0 {
		t.Fatalf("%d copied tables carry an IDENTITY column; a copy back out would leave the new backend's sequence behind the copied ids", identities)
	}

	persisted, err := os.ReadFile(filepath.Join(dir, "database.json"))
	if err != nil || len(persisted) == 0 {
		t.Fatalf("the connection secret was not written: %v", err)
	}
}

// closeOrFail closes c and fails the test when the close errors, so a leaked or broken connection is visible instead of swallowed.
func closeOrFail(t *testing.T, c io.Closer) {
	t.Helper()
	if err := c.Close(); err != nil {
		t.Errorf("close: %v", err)
	}
}
