// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

// TestMigrateSQLite is the baseline: the embedded migration set must
// apply from scratch against an empty SQLite file. Always runs.
func TestMigrateSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conformance.db")
	database, err := openSQLite(path)
	if err != nil {
		t.Fatalf("opening sqlite: %v", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			t.Logf("closing database: %v", err)
		}
	}()

	if err := Migrate(database, DriverSQLite); err != nil {
		t.Fatalf("Migrate(sqlite): %v", err)
	}
}

// migrateAgainstDSN is the shared body of the external-driver
// conformance tests. Each one boots against an empty database and
// reports the exact first failure, which is what step 0.1 of the
// secrets-manager plan asks to record per driver.
func migrateAgainstDSN(t *testing.T, envVar string, driver Driver) {
	t.Helper()
	dsn := os.Getenv(envVar)
	if dsn == "" {
		t.Skipf("%s not set; skipping %s conformance run", envVar, driver)
	}

	var (
		database *sql.DB
		err      error
	)
	switch driver {
	case DriverPostgres:
		database, err = openPostgres(dsn)
	case DriverMySQL:
		database, err = openMySQL(dsn)
	case DriverMSSQL:
		database, err = openMSSQL(dsn)
	default:
		t.Fatalf("unsupported driver %q", driver)
	}
	if err != nil {
		t.Fatalf("opening %s: %v", driver, err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			t.Logf("closing database: %v", err)
		}
	}()

	if err := Migrate(database, driver); err != nil {
		t.Fatalf("Migrate(%s): %v", driver, err)
	}

	// Re-running must be a no-op: the tracking table records every
	// applied file, so a second pass touches nothing.
	if err := Migrate(database, driver); err != nil {
		t.Fatalf("Migrate(%s) second pass: %v", driver, err)
	}
}

func TestMigratePostgres(t *testing.T) {
	migrateAgainstDSN(t, "MCHARBOR_TEST_PG_DSN", DriverPostgres)
}

func TestMigrateMySQL(t *testing.T) {
	migrateAgainstDSN(t, "MCHARBOR_TEST_MYSQL_DSN", DriverMySQL)
}

func TestMigrateMariaDB(t *testing.T) {
	migrateAgainstDSN(t, "MCHARBOR_TEST_MARIADB_DSN", DriverMySQL)
}

func TestMigrateMSSQL(t *testing.T) {
	migrateAgainstDSN(t, "MCHARBOR_TEST_MSSQL_DSN", DriverMSSQL)
}
