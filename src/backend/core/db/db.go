// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

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

	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON&_synchronous=NORMAL", dbPath)
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Set connection pool (SQLite is single-writer)
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)

	// Verify connection
	if err := database.Ping(); err != nil {
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	// Run PRAGMAs
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA cache_size=-20000",
		"PRAGMA temp_store=MEMORY",
	}
	for _, p := range pragmas {
		if _, err := database.Exec(p); err != nil {
			slog.Warn("pragma failed", "pragma", p, "error", err)
		}
	}

	instance = database
	slog.Info("database opened", "path", dbPath)
	return database, nil
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
