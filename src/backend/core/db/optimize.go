// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// OptimizeStatement is SQLite's recommended maintenance call for a long-lived
// connection: 0x10000 checks every table, not only those this connection
// queried, and 0x02 analyzes only the ones whose statistics are missing or stale.
const OptimizeStatement = "PRAGMA optimize=0x10002"

// optimizeTimeout bounds one run. SQLite applies its own temporary analysis
// limit inside PRAGMA optimize, so a run is milliseconds; the deadline only
// matters if the writer lock is held for longer than that.
const optimizeTimeout = 60 * time.Second

// Optimize refreshes SQLite's planner statistics (sqlite_stat1), which a
// database that has never been analyzed does not have at all. It is a no-op on
// every other driver, whose servers keep statistics themselves (autovacuum on
// PostgreSQL, InnoDB persistent stats on MySQL).
func Optimize(ctx context.Context, database *sql.DB, driver Driver) error {
	if driver != DriverSQLite || database == nil {
		return nil
	}
	runCtx, cancel := context.WithTimeout(ctx, optimizeTimeout)
	defer cancel()
	started := time.Now()
	if _, err := database.ExecContext(runCtx, OptimizeStatement); err != nil {
		return fmt.Errorf("running %s: %w", OptimizeStatement, err)
	}
	slog.Info("sqlite planner statistics refreshed", "statement", OptimizeStatement, "duration", time.Since(started).Round(time.Millisecond))
	return nil
}
