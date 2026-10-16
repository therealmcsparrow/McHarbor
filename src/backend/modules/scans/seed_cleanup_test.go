// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scans

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

func migratedScansDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := coredb.OpenSQLite(filepath.Join(t.TempDir(), "scans.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Log(err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	return database
}

func scannerCount(t *testing.T, database *sql.DB) int {
	t.Helper()
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM scanner_configs`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestFreshInstallHasNoPhantomScanners guards the fresh install: migration 079 seeds Trivy and
// Grype rows on every database, and with no scanner plugin installed they showed as
// "Plugin removed", Trivy as the default.
func TestFreshInstallHasNoPhantomScanners(t *testing.T) {
	database := migratedScansDB(t)
	if scannerCount(t, database) == 0 {
		t.Fatal("precondition: the migration should have seeded scanner rows")
	}
	removed, err := NewConfigService(database, nil, nil).pruneUnusedSeedRows(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if removed != 2 || scannerCount(t, database) != 0 {
		t.Errorf("removed %d, %d rows left; want both seeded rows gone", removed, scannerCount(t, database))
	}
}

// TestSeededScannersSurviveWhereTheyMeanSomething keeps an upgraded install's migrated row and
// any row an operator configured.
func TestSeededScannersSurviveWhereTheyMeanSomething(t *testing.T) {
	ctx := context.Background()

	upgraded := migratedScansDB(t)
	if _, err := upgraded.Exec(`INSERT INTO settings (category, key, value) VALUES ('scanners', 'scanner_trivy_enabled', 'true')`); err != nil {
		t.Fatal(err)
	}
	if removed, err := NewConfigService(upgraded, nil, nil).pruneUnusedSeedRows(ctx); err != nil || removed != 0 {
		t.Errorf("upgraded install lost %d rows (%v); legacy settings mean they were in use", removed, err)
	}

	configured := migratedScansDB(t)
	if _, err := configured.Exec(`UPDATE scanner_configs SET config = '{"severity":"HIGH"}' WHERE id = 'scannercfggrype0000000000'`); err != nil {
		t.Fatal(err)
	}
	if _, err := NewConfigService(configured, nil, nil).pruneUnusedSeedRows(ctx); err != nil {
		t.Fatal(err)
	}
	if scannerCount(t, configured) != 1 {
		t.Errorf("a configured row was removed; %d rows left, want 1", scannerCount(t, configured))
	}
}
