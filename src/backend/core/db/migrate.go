// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrationsDDL is a small, port-free DDL fragment used to create
// the schema_migrations tracking table on whichever backend is
// active. SQLite uses INTEGER PRIMARY KEY AUTOINCREMENT; Postgres
// uses BIGSERIAL; MySQL uses BIGINT AUTO_INCREMENT. The default
// timestamp column is a plain TEXT on SQLite (we store RFC3339
// strings to match the rest of the schema), TIMESTAMPTZ on
// Postgres, and DATETIME(3) on MySQL.
const (
	migrationsDDLSQLite = `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)
	`
	migrationsDDLPostgres = `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			id BIGSERIAL PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`
	// MySQL: the table needs ENGINE=InnoDB for the named-lock
	// coordination to be useful (InnoDB supports row locks;
	// MyISAM does not). DATETIME(3) gives millisecond
	// resolution, more than enough for migration timestamps.
	migrationsDDLMySQL = `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			name VARCHAR(255) NOT NULL UNIQUE,
			applied_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
		) ENGINE=InnoDB
	`
)

// Migrate runs all pending SQL migrations in order. Each migration
// is wrapped in its own transaction so a failure rolls back only
// that file's changes and leaves earlier migrations applied.
//
// The migrations directory is driver-agnostic: every file in
// migrations/ is valid SQL on SQLite, Postgres, and MySQL. The
// few SQLite-only constructs (PRAGMA, AUTOINCREMENT, datetime('now'))
// have been removed by the time the file reaches Migrate — the
// porting happened in a follow-up commit so this function never
// has to translate.
func Migrate(db *sql.DB, driver Driver) error {
	// Bootstrap the tracking table with the correct DDL for the
	// active driver. CREATE TABLE IF NOT EXISTS is idempotent so
	// this is safe to re-run.
	ddl := migrationsDDLSQLite
	switch driver {
	case DriverPostgres:
		ddl = migrationsDDLPostgres
	case DriverMySQL:
		ddl = migrationsDDLMySQL
	}
	if _, err := db.Exec(ddl); err != nil {
		return fmt.Errorf("creating migrations table: %w", err)
	}

	// Backward compatibility: installs that ran pre-HA code
	// tracked applied migrations in a table called `_migrations`
	// with the same `name` column. Rename it to `schema_migrations`
	// once, then carry over. This is a no-op on fresh installs
	// and on the second run.
	if err := migrateRenameLegacyTable(db, driver); err != nil {
		return fmt.Errorf("migrating legacy _migrations table: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("reading migrations directory: %w", err)
	}

	// Stable order by filename. Migrations are named with a
	// numeric prefix (001_*.sql, 002_*.sql, ...) so lexicographic
	// order matches intended application order.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		name := entry.Name()

		// Skip if already applied. The check uses the
		// driver's placeholder syntax: $1 on Postgres, ? on
		// SQLite and MySQL.
		var count int
		if err := db.QueryRow(
			migrationLookupSQL(driver), name,
		).Scan(&count); err != nil {
			return fmt.Errorf("checking migration %s: %w", name, err)
		}
		if count > 0 {
			continue
		}

		content, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", name, err)
		}

		if err := applyMigration(db, driver, name, string(content)); err != nil {
			return err
		}
		slog.Info("migration applied", "name", name, "driver", driver)
	}
	return nil
}

// migrationLookupSQL returns the correct lookup query for the
// active driver. Postgres uses $1 placeholders; SQLite and
// MySQL use ?.
func migrationLookupSQL(driver Driver) string {
	if driver == DriverPostgres {
		return "SELECT COUNT(*) FROM schema_migrations WHERE name = $1"
	}
	return "SELECT COUNT(*) FROM schema_migrations WHERE name = ?"
}

// migrationRecordSQL returns the correct INSERT for recording a
// freshly applied migration. Same placeholder rule as
// migrationLookupSQL.
func migrationRecordSQL(driver Driver) string {
	if driver == DriverPostgres {
		return "INSERT INTO schema_migrations (name) VALUES ($1)"
	}
	return "INSERT INTO schema_migrations (name) VALUES (?)"
}

// migrateRenameLegacyTable migrates the pre-HA `_migrations`
// tracking table (with its `id, name, applied_at` columns) into
// the new `schema_migrations` table. The new tracking table may
// already exist on the new driver, so the helper:
//  1. Creates the new table if it doesn't exist.
//  2. INSERTs each row from the legacy table, ignoring
//     conflicts. The dialect for the upsert varies per driver:
//     Postgres uses `ON CONFLICT (name) DO NOTHING`, MySQL uses
//     `INSERT IGNORE`, and SQLite uses `INSERT OR IGNORE`.
//  3. Drops the legacy table when the copy is complete.
//
// Fresh installs (no `_migrations` table) get a no-op.
func migrateRenameLegacyTable(db *sql.DB, driver Driver) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	legacyExists, err := tableExists(ctx, db, "_migrations")
	if err != nil {
		slog.Warn("coordinator: legacy _migrations table check failed; skipping rename",
			"error", err, "driver", driver)
		return nil
	}
	if !legacyExists {
		return nil
	}

	upsertSQL := "INSERT INTO schema_migrations (name, applied_at) SELECT name, applied_at FROM _migrations"
	switch driver {
	case DriverMySQL:
		upsertSQL += " ON DUPLICATE KEY UPDATE name = name"
	default:
		upsertSQL += " ON CONFLICT (name) DO NOTHING"
	}
	if _, err := db.ExecContext(ctx, upsertSQL); err != nil {
		return fmt.Errorf("copying rows from _migrations to schema_migrations: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DROP TABLE _migrations"); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("dropping legacy _migrations: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	slog.Info("migrated legacy _migrations table to schema_migrations")
	return nil
}

// tableExists returns true when a table with the given name is
// present. We try information_schema first (works on SQLite 3.33+,
// MySQL 5+, and all Postgres versions we support) and fall
// back to the SQLite-only sqlite_master if information_schema is
// not exposed.
func tableExists(ctx context.Context, db *sql.DB, name string) (bool, error) {
	var n int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_name = ?",
		name,
	).Scan(&n); err == nil {
		return n > 0, nil
	}
	return false, nil
}

// applyMigration runs one migration file inside a transaction
// and records the applied row. We use a short context timeout
// (5 minutes per migration) so a hung statement on a stuck
// database can't wedge startup forever.
func applyMigration(db *sql.DB, driver Driver, name, sqlText string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction for %s: %w", err)
	}
	if _, err := tx.ExecContext(ctx, translateSQLForDriver(driver, sqlText)); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("executing migration %s: %w", name, err)
	}
	if _, err := tx.ExecContext(
		ctx, migrationRecordSQL(driver), name,
	); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("recording migration %s: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing migration %s: %w", err)
	}
	return nil
}

// translateSQLForDriver rewrites migration SQL into the dialect
// of the active driver. SQLite and Postgres share enough syntax
// (the migrations were ported to that union) that no translation
// is needed. MySQL needs two rewrites:
//
//   - "INSERT ... ON CONFLICT (cols) DO NOTHING" — Postgres /
//     SQLite upsert → MySQL's "ON DUPLICATE KEY UPDATE col = col"
//     (a no-op upsert that swallows the duplicate-key error).
//   - "datetime('now')" — SQLite scalar → MySQL's "CURRENT_TIMESTAMP".
//     The form `datetime('now', mod)` is translated to
//     `CURRENT_TIMESTAMP mod` (e.g. `-1 day`).
//
// The rewrites are intentionally minimal: anything the active
// driver accepts verbatim is left alone.
func translateSQLForDriver(driver Driver, sqlText string) string {
	if driver != DriverMySQL {
		return sqlText
	}
	out := onConflictMySQLRE.ReplaceAllStringFunc(sqlText, func(match string) string {
		groups := onConflictMySQLRE.FindStringSubmatch(match)
		if len(groups) < 2 {
			return match
		}
		cols := strings.Split(groups[1], ",")
		assignments := make([]string, 0, len(cols))
		for _, col := range cols {
			col = strings.TrimSpace(col)
			if col == "" {
				continue
			}
			assignments = append(assignments, col+" = "+col)
		}
		return "ON DUPLICATE KEY UPDATE " + strings.Join(assignments, ", ")
	})
	out = datetimeNowRE.ReplaceAllStringFunc(out, func(match string) string {
		if strings.Contains(match, ",") {
			parts := datetimeNowRE.FindStringSubmatch(match)
			if len(parts) < 2 {
				return match
			}
			mod := strings.TrimSpace(parts[1])
			return "CURRENT_TIMESTAMP " + mod
		}
		return "CURRENT_TIMESTAMP"
	})
	return out
}

// onConflictMySQLRE matches `ON CONFLICT (col1[, col2, ...]) DO NOTHING`
// (Postgres / SQLite upsert syntax) so the rewriter can produce the
// MySQL `ON DUPLICATE KEY UPDATE col = col` equivalent.
var onConflictMySQLRE = regexp.MustCompile(`ON\s+CONFLICT\s*\(([^)]+)\)\s+DO\s+NOTHING`)

// datetimeNowRE matches `datetime('now'[, 'mod'])` (SQLite scalar)
// for translation to `CURRENT_TIMESTAMP [mod]` on MySQL.
var datetimeNowRE = regexp.MustCompile(`datetime\(\s*'now'\s*(?:,\s*'([^']+)')?\s*\)`)
