// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// openSQLite opens the single-file database used by single-node
// installs. WAL mode is enabled so the event collector, metrics
// scraper, and concurrent backup progress writes can all hit the
// database in parallel without tripping the writer lock.
//
// The connection pool is tuned so progress updates from concurrent
// backup runs, the metrics collector, the activity collector, the
// alerts engine, the autoheal engine, and the backup scheduler can
// all queue on the writer lock without tripping the busy timeout.
//
// modernc.org/sqlite's `_busy_timeout` URI parameter only applies
// to the first connection the driver returns. Subsequent pool
// connections come up without the busy_timeout set, which has
// caused lock-wait failures in production. We iterate MaxOpenConns
// and re-apply the pragmas on every connection to keep them stable.
// connMaxIdleTime is how long a pooled connection may sit unused before it is
// closed. One policy for all three drivers: it is about the network and the
// server's own idle timeout, not about which SQL dialect is on the far end.
const connMaxIdleTime = 5 * time.Minute

func openSQLite(dbPath string) (*sql.DB, error) {
	return openSQLiteContext(context.Background(), dbPath)
}

// openSQLiteContext is openSQLite with the caller's context bounding the ping, pragma warm-up and planner refresh.
func openSQLiteContext(ctx context.Context, dbPath string) (*sql.DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating database directory: %w", err)
	}

	dsn := fmt.Sprintf(
		"file:%s?_journal_mode=WAL&_busy_timeout=60000&_foreign_keys=ON&_synchronous=NORMAL",
		dbPath,
	)
	database, err := openDriver("sqlite", dsn, DriverSQLite)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database: %w", err)
	}

	database.SetMaxOpenConns(16)
	database.SetMaxIdleConns(8)
	database.SetConnMaxIdleTime(connMaxIdleTime)

	if err := database.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("pinging sqlite database: %w", err)
	}

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=60000",
		"PRAGMA foreign_keys=ON",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA cache_size=-100000",
		"PRAGMA temp_store=MEMORY",
		"PRAGMA wal_autocheckpoint=1000",
	}
	if err := warmConnectionPool(ctx, database, pragmas); err != nil {
		return nil, fmt.Errorf("warming sqlite connections: %w", err)
	}
	// Before the first request, and once rather than per pooled connection: the
	// statistics live in the file, so every connection plans with them. A
	// failure only costs plan quality, so it is logged and the open goes on.
	if err := Optimize(ctx, database, DriverSQLite); err != nil {
		slog.Warn("sqlite planner statistics not refreshed at open", "path", dbPath, "error", err)
	}

	slog.Info("database opened", "driver", DriverSQLite, "path", dbPath)
	return database, nil
}

// warmConnectionPool opens up to MaxOpenConns connections, applies
// each statement (a PRAGMA in our case) on every connection, and
// closes them back to the pool. After this call every connection
// database/sql hands out has the statements applied.
func warmConnectionPool(ctx context.Context, database *sql.DB, pragmas []string) error {
	maxOpen := database.Stats().MaxOpenConnections
	if maxOpen <= 0 {
		maxOpen = 16
	}
	conns := make([]*sql.Conn, 0, maxOpen)
	defer func() {
		for _, c := range conns {
			if err := c.Close(); err != nil {
				slog.Warn("releasing warmed sqlite connection failed", "error", err)
			}
		}
	}()
	for i := 0; i < maxOpen; i++ {
		c, err := database.Conn(ctx)
		if err != nil {
			return err
		}
		conns = append(conns, c)
		for _, p := range pragmas {
			if _, err := c.ExecContext(ctx, p); err != nil {
				return fmt.Errorf("applying statement %q: %w", p, err)
			}
		}
	}
	return nil
}
