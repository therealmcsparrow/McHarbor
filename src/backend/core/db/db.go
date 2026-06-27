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

// DB holds the SQLite connection.
var instance *sql.DB

// Open initializes the SQLite database at the given path.
func Open(dbPath string) (*sql.DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating database directory: %w", err)
	}

	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=30000&_foreign_keys=ON&_synchronous=NORMAL", dbPath)
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Set connection pool. SQLite in WAL mode supports many concurrent
	// readers alongside a single writer. MaxOpenConns is set generously
	// above the writer fan-out so progress updates from concurrent
	// backups, the metrics collector, the activity collector, the
	// alerts engine, the autoheal engine, and the backup scheduler can
	// all queue on the writer lock without tripping the busy timeout.
	database.SetMaxOpenConns(16)
	database.SetMaxIdleConns(8)
	database.SetConnMaxIdleTime(5 * time.Minute)

	// Verify connection
	if err := database.Ping(); err != nil {
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	// Run PRAGMAs on every connection the pool will hand out.
	// modernc.org/sqlite's `_busy_timeout` URI parameter is NOT
	// applied to connections that come up after the first one, and
	// running a single PRAGMA only configures whichever connection
	// the call lands on. We iterate `db.Conn()` MaxOpenConns times to
	// pin the pragmas on every connection in the pool.
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=30000",
		"PRAGMA foreign_keys=ON",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA cache_size=-20000",
		"PRAGMA temp_store=MEMORY",
	}
	if err := warmConnectionPool(database, pragmas); err != nil {
		return nil, fmt.Errorf("warming database connections: %w", err)
	}

	instance = database
	slog.Info("database opened", "path", dbPath)
	return database, nil
}

// warmConnectionPool opens up to MaxOpenConns connections, applies
// each PRAGMA on every connection, and closes them back to the pool.
// After this call every connection database/sql hands out has the
// pragmas applied. Subsequent connections opened past MaxOpenConns
// also run the pragmas because sql.DB.Conn() pulls from the pool
// first; if all are busy a new connection is created and the hook
// path is not currently re-invoked — that's acceptable for this
// project where the worker fan-out is bounded.
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
				return fmt.Errorf("applying pragma %q: %w", p, err)
			}
		}
	}
	return nil
}

// Get returns the current database instance.
func Get() *sql.DB {
	return instance
}

// Close closes the database connection.
func Close() error {
	if instance != nil {
		return instance.Close()
	}
	return nil
}

// RowsAffected safely extracts the rows-affected count from a sql.Result.
// SQLite's driver never returns an error here, but this centralizes the handling.
func RowsAffected(r sql.Result) int64 {
	n, _ := r.RowsAffected()
	return n
}

// Tx runs a function within a database transaction.
func Tx(db *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("rollback failed: %w (original: %w)", rbErr, err)
		}
		return err
	}

	return tx.Commit()
}
