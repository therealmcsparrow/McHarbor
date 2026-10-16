// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

// TestOpenSQLiteRefreshesPlannerStatistics guards that opening the database
// runs PRAGMA optimize: a populated, never-analyzed file gains sqlite_stat1 rows
// on the next open, which is the state the live instance was measured without.
func TestOpenSQLiteRefreshesPlannerStatistics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "optimize.db")
	database, err := openSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE events (id TEXT PRIMARY KEY, env TEXT, at TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE INDEX idx_events_env ON events(env, at)`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 500; i++ {
		if _, err := database.Exec(`INSERT INTO events (id, env, at) VALUES (?, ?, ?)`,
			fmt.Sprintf("e%d", i), fmt.Sprintf("env-%d", i%5), fmt.Sprintf("2026-09-27T00:%02d:00Z", i%60)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.Exec(`DROP TABLE IF EXISTS sqlite_stat1`); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := openSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := reopened.Close(); err != nil {
			t.Logf("closing reopened: %v", err)
		}
	}()
	var n int
	if err := reopened.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM sqlite_stat1 WHERE tbl = 'events'`).Scan(&n); err != nil {
		t.Fatalf("reading sqlite_stat1 after reopening: %v", err)
	}
	if n == 0 {
		t.Fatal("opening a populated database left no planner statistics for it")
	}
}

// TestOptimizeIsANoOpOffSQLite keeps the statement away from servers that
// would reject a PRAGMA, and tolerates a nil handle.
func TestOptimizeIsANoOpOffSQLite(t *testing.T) {
	for _, d := range []Driver{DriverPostgres, DriverMySQL, DriverMSSQL} {
		if err := Optimize(context.Background(), nil, d); err != nil {
			t.Fatalf("%s: %v", d, err)
		}
	}
}
