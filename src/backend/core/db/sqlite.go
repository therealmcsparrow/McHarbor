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
func openSQLite(dbPath string) (*sql.DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating database directory: %w", err)
	}

	dsn := fmt.Sprintf(
		"file:%s?_journal_mode=WAL&_busy_timeout=60000&_foreign_keys=ON&_synchronous=NORMAL",
		dbPath,
	)
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database: %w", err)
	}

	database.SetMaxOpenConns(16)
	database.SetMaxIdleConns(8)
	database.SetConnMaxIdleTime(5 * time.Minute)

	if err := database.Ping(); err != nil {
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
	if err := warmConnectionPool(database, pragmas); err != nil {
		return nil, fmt.Errorf("warming sqlite connections: %w", err)
	}

	slog.Info("database opened", "driver", DriverSQLite, "path", dbPath)
	return database, nil
}

// warmConnectionPool opens up to MaxOpenConns connections, applies
// each statement (a PRAGMA in our case) on every connection, and
// closes them back to the pool. After this call every connection
// database/sql hands out has the statements applied.
func warmConnectionPool(database *sql.DB, pragmas []string) error {
	maxOpen := database.Stats().MaxOpenConnections
	if maxOpen <= 0 {
		maxOpen = 16
	}
	conns := make([]*sql.Conn, 0, maxOpen)
	defer func() {
		for _, c := range conns {
			_ = c.Close()
		}
	}()
	for i := 0; i < maxOpen; i++ {
		c, err := database.Conn(context.Background())
		if err != nil {
			return err
		}
		conns = append(conns, c)
		for _, p := range pragmas {
			if _, err := c.ExecContext(context.Background(), p); err != nil {
				return fmt.Errorf("applying statement %q: %w", p, err)
			}
		}
	}
	return nil
}
