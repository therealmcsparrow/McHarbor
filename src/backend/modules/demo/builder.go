// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// accountTables are copied from the running database into demo.db, parents
// before children, so every user can still sign in and every live session
// stays signed in across the restart into demo mode.
var accountTables = []string{
	"roles",
	"groups",
	"group_roles",
	"users",
	"user_roles",
	"user_permissions",
	"user_labels",
	"group_members",
	"user_mfa",
	"auth_settings",
}

// pluginTables carry which plugins are installed and their configuration; the
// plugin files themselves are shared on disk and loaded from the same place.
var pluginTables = []string{"appstore_plugins", "plugin_permissions", "plugin_config"}

// openDemoDB opens path without touching the process-wide database handle,
// which db.Open would replace.
func openDemoDB(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("creating demo database directory: %w", err)
	}
	conn, err := sql.Open("sqlite", "file:"+path+"?_journal_mode=WAL&_busy_timeout=60000&_foreign_keys=OFF")
	if err != nil {
		return nil, fmt.Errorf("opening demo database: %w", err)
	}
	conn.SetMaxOpenConns(1)
	return conn, nil
}

// buildDatabase writes a complete demo.db at path: the full schema, the
// accounts and sessions of source, and the installed plugins with their tables.
func (s *Service) buildDatabase(ctx context.Context, source *sql.DB, path string) error {
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("clearing old demo database: %w", err)
		}
	}
	target, err := openDemoDB(path)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := target.Close(); cerr != nil {
			s.logger.Warn("demo: closing demo database", "error", cerr)
		}
	}()
	if err := db.MigrateContext(ctx, target, db.DriverSQLite); err != nil {
		return fmt.Errorf("migrating demo database: %w", err)
	}
	if err := s.copyAccounts(ctx, source, target); err != nil {
		return err
	}
	for _, table := range pluginTables {
		if err := copyTable(ctx, source, target, table, ""); err != nil {
			return err
		}
	}
	if err := s.applyPluginMigrations(ctx, target); err != nil {
		return err
	}
	if _, err := target.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return fmt.Errorf("checkpointing demo database: %w", err)
	}
	return nil
}

// copyAccounts refreshes the account tables and live sessions in target from
// source; it also runs when an existing demo.db is re-entered, so a password
// changed since the last visit works in the demo too.
func (s *Service) copyAccounts(ctx context.Context, source, target *sql.DB) error {
	for _, table := range accountTables {
		if err := copyTable(ctx, source, target, table, ""); err != nil {
			return err
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return copyTable(ctx, source, target, "sessions", "revoked_at IS NULL AND expires_at > '"+now+"'")
}

func columnsOf(ctx context.Context, conn *sql.DB, table string) (map[string]bool, error) {
	rows, err := conn.QueryContext(ctx, "SELECT * FROM "+table+" WHERE 1 = 0") // SQL identifier: table is an accountTables constant or "sessions"
	if err != nil {
		return nil, err
	}
	defer func() { logCloseErr(rows.Close()) }()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(cols))
	for _, c := range cols {
		out[strings.ToLower(c)] = true
	}
	return out, rows.Err()
}

// copyTable copies every row of table the two schemas can both hold. A table
// missing on either side is skipped: the source may predate a migration.
func copyTable(ctx context.Context, source, target *sql.DB, table, where string) error {
	srcCols, err := columnsOf(ctx, source, table)
	if err != nil {
		return nil
	}
	dstCols, err := columnsOf(ctx, target, table)
	if err != nil {
		return nil
	}
	var cols []string
	for c := range srcCols {
		if dstCols[c] {
			cols = append(cols, c)
		}
	}
	if len(cols) == 0 {
		return nil
	}
	sort.Strings(cols)
	query := "SELECT " + strings.Join(cols, ", ") + " FROM " + table // SQL identifier: table is a constant, cols are read back from both schemas
	if where != "" {
		query += " WHERE " + where
	}
	query += " LIMIT 100000"
	rows, err := source.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("reading %s: %w", table, err)
	}
	defer func() { logCloseErr(rows.Close()) }()
	tx, err := target.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("copying %s: %w", table, err)
	}
	committed := false
	defer func() {
		if !committed {
			if rerr := tx.Rollback(); rerr != nil && !errors.Is(rerr, sql.ErrTxDone) {
				return
			}
		}
	}()
	insert := "INSERT OR REPLACE INTO " + table + " (" + strings.Join(cols, ", ") + ") VALUES (" + strings.TrimSuffix(strings.Repeat("?, ", len(cols)), ", ") + ")" // SQL identifier: table is a constant, cols are read back from both schemas
	stmt, err := tx.PrepareContext(ctx, insert)
	if err != nil {
		return fmt.Errorf("preparing %s copy: %w", table, err)
	}
	defer func() { logCloseErr(stmt.Close()) }()
	values := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range values {
		ptrs[i] = &values[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return fmt.Errorf("scanning %s: %w", table, err)
		}
		for i, v := range values {
			if t, ok := v.(time.Time); ok {
				values[i] = t.UTC().Format(time.RFC3339)
			}
		}
		if _, err := stmt.ExecContext(ctx, values...); err != nil {
			return fmt.Errorf("writing %s: %w", table, err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("reading %s: %w", table, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing %s: %w", table, err)
	}
	committed = true
	return nil
}

// applyPluginMigrations runs every installed plugin's migration files into
// target from the install directory the loader will dlopen, and records them
// in the ledger, which is what an install would have done on this database.
func (s *Service) applyPluginMigrations(ctx context.Context, target *sql.DB) error {
	installs, err := installedPlugins(ctx, target)
	if err != nil {
		return err
	}
	// bounded by the plugin migration files shipped in the install directory
	for _, in := range installs {
		dir := filepath.Join(s.dataDir, "plugins", in.slug+"@"+in.version, "migrations")
		entries, err := os.ReadDir(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("reading %s migrations: %w", in.slug, err)
		}
		// bounded by the plugin migration files shipped in the install directory
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
				continue
			}
			body, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				return fmt.Errorf("reading %s/%s: %w", in.slug, e.Name(), err)
			}
			if _, err := target.ExecContext(ctx, string(body)); err != nil {
				return fmt.Errorf("applying %s/%s: %w", in.slug, e.Name(), err)
			}
			now := time.Now().UTC().Format(time.RFC3339)
			if _, err := target.ExecContext(ctx,
				`INSERT OR IGNORE INTO plugin_migrations (id, origin, filename, applied_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
				uuid.NewString(), in.slug, e.Name(), now, now, now); err != nil {
				return fmt.Errorf("recording %s/%s: %w", in.slug, e.Name(), err)
			}
		}
	}
	return nil
}

type pluginInstall struct{ slug, version string }

// installedPlugins lists the plugins recorded in target and closes the cursor before migrations write to the same database.
func installedPlugins(ctx context.Context, target *sql.DB) ([]pluginInstall, error) {
	rows, err := target.QueryContext(ctx, "SELECT slug, version FROM appstore_plugins LIMIT 1000")
	if err != nil {
		return nil, fmt.Errorf("listing installed plugins: %w", err)
	}
	defer func() { logCloseErr(rows.Close()) }()
	var installs []pluginInstall
	for rows.Next() {
		var in pluginInstall
		if err := rows.Scan(&in.slug, &in.version); err != nil {
			return nil, fmt.Errorf("reading installed plugins: %w", err)
		}
		installs = append(installs, in)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading installed plugins: %w", err)
	}
	return installs, nil
}
