// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
)

// DB holds the open connection. The Driver is recorded so the
// coordinator (and any future code that needs to know which
// backend is in use) can make the right decision.
var instance *sql.DB
var instanceDriver Driver

// Config bundles the inputs Open() needs. Either SQLite (Path)
// or a network backend (DSN) is used depending on Driver.
type Config struct {
	// Driver is one of DriverSQLite, DriverPostgres, or
	// DriverMySQL. When empty, the package falls back to
	// DriverSQLite so single-node installs keep working without
	// any new env var.
	Driver Driver
	// Path is the SQLite file path (used when Driver == DriverSQLite).
	Path string
	// DSN is the database connection string for the network
	// drivers. Postgres expects a libpq URL; MySQL / MariaDB
	// expects a go-sql-driver/mysql DSN.
	DSN string
}

// Open initializes the database connection for the configured
// driver. The function is backward compatible: when Driver is
// empty (existing single-node installs), the legacy SQLite file
// at Path is opened. Setting MCHARBOR_DB_DRIVER=postgres +
// MCHARBOR_DB_DSN=postgres://... switches to Postgres without any
// other change. Setting MCHARBOR_DB_DRIVER=mysql switches to
// MySQL / MariaDB.
func Open(cfg Config) (*sql.DB, error) {
	if cfg.Driver == "" {
		cfg.Driver = DriverSQLite
	}

	var (
		database *sql.DB
		err      error
	)
	switch cfg.Driver {
	case DriverSQLite:
		if cfg.Path == "" {
			return nil, fmt.Errorf("sqlite driver requires a non-empty path")
		}
		database, err = openSQLite(cfg.Path)
	case DriverPostgres:
		if cfg.DSN == "" {
			return nil, fmt.Errorf("postgres driver requires MCHARBOR_DB_DSN")
		}
		database, err = openPostgres(cfg.DSN)
	case DriverMySQL:
		if cfg.DSN == "" {
			return nil, fmt.Errorf("mysql driver requires MCHARBOR_DB_DSN")
		}
		database, err = openMySQL(cfg.DSN)
	default:
		return nil, fmt.Errorf("unknown database driver %q (expected 'sqlite', 'postgres', or 'mysql')", cfg.Driver)
	}
	if err != nil {
		return nil, err
	}

	instance = database
	instanceDriver = cfg.Driver
	slog.Info("database connection ready", "driver", cfg.Driver)
	return database, nil
}

// OpenSQLite is a thin shim kept for backward compatibility with
// the old single-argument signature (it predates the Config struct).
// New code should use Open(Config{...}). Internal callers (CLI
// entrypoint, tests) can use this when they don't have a Config
// object handy.
func OpenSQLite(dbPath string) (*sql.DB, error) {
	return Open(Config{Driver: DriverSQLite, Path: dbPath})
}

// Get returns the current database instance, or nil if Open has
// not been called.
func Get() *sql.DB {
	return instance
}

// DriverOf returns the driver the current connection is using.
// Returns the empty string before Open() has been called.
func DriverOf() Driver {
	return instanceDriver
}

// Close closes the database connection. Safe to call when no
// connection is open.
func Close() error {
	if instance != nil {
		err := instance.Close()
		instance = nil
		instanceDriver = ""
		return err
	}
	return nil
}

// RowsAffected safely extracts the rows-affected count from a
// sql.Result. SQLite's driver never returns an error here, but
// this centralizes the handling.
func RowsAffected(r sql.Result) int64 {
	n, _ := r.RowsAffected()
	return n
}

// Tx runs a function within a database transaction. The driver
// doesn't matter — both SQLite (WAL) and Postgres handle the
// nested transactions the call sites use today.
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

// reserved for the coordinator + module init paths.
var _ = os.Getenv
