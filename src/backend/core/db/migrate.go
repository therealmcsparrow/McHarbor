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
	// SQL Server has no CREATE TABLE IF NOT EXISTS, so the existence test is
	// the statement. NVARCHAR(255) rather than NVARCHAR(MAX) because the column
	// carries a UNIQUE constraint and SQL Server refuses to index a MAX type.
	migrationsDDLMSSQL = `
		IF OBJECT_ID(N'dbo.schema_migrations', N'U') IS NULL
		CREATE TABLE schema_migrations (
			id BIGINT IDENTITY(1,1) PRIMARY KEY,
			name NVARCHAR(255) NOT NULL UNIQUE,
			applied_at DATETIME2(3) NOT NULL DEFAULT SYSUTCDATETIME()
		)
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
	return MigrateContext(context.Background(), db, driver)
}

// MigrateContext is Migrate bound to the caller's context, so a cancelled start-up stops applying further migrations.
func MigrateContext(ctx context.Context, db *sql.DB, driver Driver) error {
	// Bootstrap the tracking table with the correct DDL for the
	// active driver. CREATE TABLE IF NOT EXISTS is idempotent so
	// this is safe to re-run.
	ddl := migrationsDDLSQLite
	switch driver {
	case DriverPostgres:
		ddl = migrationsDDLPostgres
	case DriverMySQL:
		ddl = migrationsDDLMySQL
	case DriverMSSQL:
		ddl = migrationsDDLMSSQL
	}
	if _, err := db.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("creating migrations table: %w", err)
	}

	// Backward compatibility: installs that ran pre-HA code
	// tracked applied migrations in a table called `_migrations`
	// with the same `name` column. Rename it to `schema_migrations`
	// once, then carry over. This is a no-op on fresh installs
	// and on the second run.
	if err := migrateRenameLegacyTable(ctx, db, driver); err != nil {
		return fmt.Errorf("migrating legacy _migrations table: %w", err)
	}

	// Reconcile the migration squash: a database that already ran the files
	// written after 2.0.8 has no record of the merged files that replaced
	// them, and would otherwise re-run all of them. See squash.go. No-op on
	// 2.0.8 and fresh installs.
	if err := reconcileSquashedMigrations(ctx, db, driver); err != nil {
		return err
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

	// One applied-check per migration, bounded by the embedded migration set
	// and run once at start-up.
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		name := entry.Name()

		// Skip if already applied. The check uses the
		// driver's placeholder syntax: $1 on Postgres, ? on
		// SQLite and MySQL.
		var count int
		if err := db.QueryRowContext(
			ctx,
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

		if err := applyMigration(ctx, db, driver, name, string(content)); err != nil {
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
	switch driver {
	case DriverPostgres:
		return "SELECT COUNT(*) FROM schema_migrations WHERE name = $1"
	case DriverMSSQL:
		return "SELECT COUNT(*) FROM schema_migrations WHERE name = @p1"
	}
	return "SELECT COUNT(*) FROM schema_migrations WHERE name = ?"
}

// migrationRecordSQL returns the correct INSERT for recording a
// freshly applied migration. Same placeholder rule as
// migrationLookupSQL.
func migrationRecordSQL(driver Driver) string {
	switch driver {
	case DriverPostgres:
		return "INSERT INTO schema_migrations (name) VALUES ($1)"
	case DriverMSSQL:
		return "INSERT INTO schema_migrations (name) VALUES (@p1)"
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
func migrateRenameLegacyTable(parent context.Context, db *sql.DB, driver Driver) error {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()

	legacyExists, err := tableExists(ctx, db, driver, "_migrations")
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
	case DriverMSSQL:
		// SQL Server has neither ON CONFLICT nor INSERT IGNORE. A NOT EXISTS
		// predicate on the source is the portable equivalent and, unlike MERGE,
		// carries none of MERGE's concurrency caveats.
		upsertSQL = "INSERT INTO schema_migrations (name, applied_at) SELECT l.name, l.applied_at FROM _migrations l" +
			" WHERE NOT EXISTS (SELECT 1 FROM schema_migrations m WHERE m.name = l.name)"
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
		RollbackTx(tx)
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
func tableExists(ctx context.Context, db *sql.DB, driver Driver, name string) (bool, error) {
	var n int
	// SQL Server exposes information_schema but binds parameters as @p1; every
	// other driver here takes `?`. Postgres reaches this through the dialect
	// rewriter, which renumbers `?` for it.
	query := "SELECT COUNT(*) FROM information_schema.tables WHERE table_name = ?"
	if driver == DriverMSSQL {
		query = "SELECT COUNT(*) FROM information_schema.tables WHERE table_name = @p1"
	}
	if err := db.QueryRowContext(ctx, query, name).Scan(&n); err == nil {
		return n > 0, nil
	}
	return false, nil
}

// applyMigration runs one migration file inside a transaction
// and records the applied row. We use a short context timeout
// (5 minutes per migration) so a hung statement on a stuck
// database can't wedge startup forever.
func applyMigration(parent context.Context, db *sql.DB, driver Driver, name, sqlText string) error {
	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	defer cancel()
	if driver == DriverSQLite && strings.Contains(sqlText, noTransactionMarker) {
		return applyMigrationNoTx(ctx, db, driver, name, sqlText)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction for %s: %w", name, err)
	}
	if err := execMigrationBody(ctx, tx, driver, name, sqlText); err != nil {
		RollbackTx(tx)
		return err
	}
	if err := runMigrationStep(ctx, tx, driver, name); err != nil {
		RollbackTx(tx)
		return err
	}
	if _, err := tx.ExecContext(
		ctx, migrationRecordSQL(driver), name,
	); err != nil {
		RollbackTx(tx)
		return fmt.Errorf("recording migration %s: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing migration %s: %w", name, err)
	}
	return nil
}

// execMigrationBody runs one migration file's contents.
//
// SQLite takes the whole file in one Exec, exactly as it always has: it is the
// dialect the files are written in and it supports every construct in them, so
// there is nothing to gain from splitting and a real risk in changing what the
// default driver does.
//
// Every other driver is executed statement by statement, which is what makes
// three things possible at once: MySQL does not need the `multiStatements`
// connection flag (which would widen the blast radius of any future dynamic
// query), a `CREATE INDEX IF NOT EXISTS` can be emulated by tolerating the
// duplicate-key error, and a SQLite-only construct can be skipped without
// taking the rest of the file with it.
func execMigrationBody(ctx context.Context, tx migrationExecutor, driver Driver, name, sqlText string) error {
	if driver == DriverSQLite {
		if _, err := tx.ExecContext(ctx, sqlText); err != nil {
			return fmt.Errorf("executing migration %s: %w", name, err)
		}
		return nil
	}

	for _, stmt := range SplitStatements(sqlText) {
		switch ClassifyStatement(driver, stmt) {
		case SkipStatement:
			continue

		case SkipStatementLoudly:
			slog.Warn("migration statement skipped: no equivalent on this driver",
				"migration", name, "driver", driver, "statement", firstLine(stmt))
			continue

		case TolerateDuplicate:
			if _, err := tx.ExecContext(ctx, TranslateStatement(driver, stmt)); err != nil {
				if isDuplicateObjectError(err) {
					continue
				}
				return fmt.Errorf("executing migration %s (%s): %w", name, firstLine(stmt), err)
			}

		case TolerateMissing:
			if _, err := tx.ExecContext(ctx, TranslateStatement(driver, stmt)); err != nil {
				if isMissingObjectError(err) {
					continue
				}
				return fmt.Errorf("executing migration %s (%s): %w", name, firstLine(stmt), err)
			}

		default:
			// SQLite applies these migrations with foreign-key enforcement off,
			// which is what lets the table-rebuild pattern — create new, copy,
			// drop old, rename — drop a table other tables still reference.
			// MySQL enforces foreign keys and rejects exactly that DROP.
			//
			// The checks come off for the DROP alone rather than for the whole
			// migration: MariaDB refuses to *add* a foreign key while they are
			// disabled, and several migrations drop one table and add a
			// constraint to another in the same file.
			//
			// Postgres needs no equivalent — translatePostgres adds CASCADE to
			// the DROP instead, which is both narrower and reversible.
			guard := driver == DriverMySQL && dropTableRE.MatchString(stmt)
			if guard {
				if _, err := tx.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 0"); err != nil {
					return fmt.Errorf("disabling foreign key checks for %s: %w", name, err)
				}
			}
			translated := TranslateStatement(driver, stmt)
			var followUps []string
			if driver == DriverMSSQL {
				// SQL Server refuses a second cascade path; the actions it would
				// refuse become triggers, which need the live foreign-key graph.
				var resolveErr error
				translated, followUps, resolveErr = mssqlResolveCascadePaths(ctx, tx, translated)
				if resolveErr != nil {
					return fmt.Errorf("executing migration %s (%s): %w", name, firstLine(stmt), resolveErr)
				}
			}
			_, err := tx.ExecContext(ctx, translated)
			for _, followUp := range followUps {
				if err != nil {
					break
				}
				_, err = tx.ExecContext(ctx, followUp)
			}
			if guard {
				if _, restoreErr := tx.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 1"); restoreErr != nil {
					slog.Warn("restoring foreign key checks failed",
						"migration", name, "error", restoreErr)
				}
			}
			if err != nil {
				return fmt.Errorf("executing migration %s (%s): %w", name, firstLine(stmt), err)
			}
		}
	}
	return nil
}

// runMigrationStep runs the Go step bound to a migration file, if it has one,
// on the same transaction as the file, so a failed step rolls the file back too.
func runMigrationStep(ctx context.Context, q migrationQuerier, driver Driver, name string) error {
	step, ok := migrationSteps[name]
	if !ok {
		return nil
	}
	if err := step(ctx, q, driver); err != nil {
		return fmt.Errorf("executing migration %s (Go step): %w", name, err)
	}
	return nil
}

// firstLine renders a statement's opening line for an error or a log entry,
// so a failure points at readable SQL rather than at 40 lines of DDL.
func firstLine(stmt string) string {
	for _, line := range strings.Split(stmt, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		if len(trimmed) > 90 {
			return trimmed[:90] + "…"
		}
		return trimmed
	}
	return "(empty)"
}

// onConflictMySQLRE matches `ON CONFLICT (col1[, col2, ...]) DO NOTHING`
// (Postgres / SQLite upsert syntax) so the rewriter can produce the
// MySQL `ON DUPLICATE KEY UPDATE col = col` equivalent.
var onConflictMySQLRE = regexp.MustCompile(`ON\s+CONFLICT\s*\(([^)]+)\)\s+DO\s+NOTHING`)

// onConflictUpdateRE matches `ON CONFLICT (cols) DO UPDATE SET ...`, the
// upsert sibling of DO NOTHING, capturing the assignment list so that
// `excluded.x` can become MySQL's `VALUES(x)`.
var onConflictUpdateRE = regexp.MustCompile(`(?is)ON\s+CONFLICT\s*\(([^)]+)\)\s+DO\s+UPDATE\s+SET\s+(.*?)(;|$)`)

// excludedRE matches the `excluded.<column>` pseudo-table Postgres and SQLite
// expose inside an upsert.
var excludedRE = regexp.MustCompile(`(?i)\bexcluded\.([a-z_][a-z0-9_]*)`)

// datetimeNowRE matches `datetime('now'[, 'mod'])` (SQLite scalar)
// for translation to `CURRENT_TIMESTAMP [mod]` on MySQL.
var datetimeNowRE = regexp.MustCompile(`datetime\(\s*'now'\s*(?:,\s*'([^']+)')?\s*\)`)

// noTransactionMarker opts a migration out of the transaction applyMigration
// would otherwise open. It exists for one case: `PRAGMA foreign_keys = OFF` is
// a documented no-op inside a SQLite transaction, so a table rebuild that has
// to drop a table other tables reference cannot be expressed in the normal
// path. A file carrying the marker opens and commits its own transaction, and
// is expected to restore any pragma it changed.
const noTransactionMarker = "-- mcharbor:no-transaction"

// migrationExecutor is what execMigrationBody needs, satisfied by both *sql.Tx
// and *sql.DB, so a no-transaction migration reuses the same body.
type migrationExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// applyMigrationNoTx runs a marked migration without the runner's transaction.
// The file owns its own atomicity; a failure part-way leaves whatever it had
// committed, which is why the marker is deliberately hard to reach for.
func applyMigrationNoTx(ctx context.Context, db *sql.DB, driver Driver, name, sqlText string) error {
	if err := execMigrationBody(ctx, db, driver, name, sqlText); err != nil {
		return err
	}
	if err := runMigrationStep(ctx, db, driver, name); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, migrationRecordSQL(driver), name); err != nil {
		return fmt.Errorf("recording migration %s: %w", name, err)
	}
	return nil
}

// MigrationNames returns every embedded migration file name, sorted. The
// health check compares it against the schema_migrations ledger.
func MigrationNames() ([]string, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("reading embedded migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}

// MigrationSQL returns one embedded migration's contents.
func MigrationSQL(name string) (string, error) {
	content, err := migrationsFS.ReadFile("migrations/" + name)
	if err != nil {
		return "", fmt.Errorf("reading embedded migration %s: %w", name, err)
	}
	return string(content), nil
}
