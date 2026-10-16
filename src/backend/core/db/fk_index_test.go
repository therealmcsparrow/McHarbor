// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// knownUnindexedForeignKeys is the allow-list for a foreign-key column that
// genuinely cannot carry an index; it is empty since migration 124 and every
// entry needs a reason. A listed column that gains an index fails the test.
var knownUnindexedForeignKeys = map[string]bool{}

// tableNames returns every user table in database, sqlite's own bookkeeping
// tables excluded.
func tableNames(t *testing.T, ctx context.Context, database *sql.DB) []string {
	t.Helper()
	rows, err := database.QueryContext(ctx,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatalf("listing tables: %v", err)
	}
	defer CloseRows(rows)
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scanning table name: %v", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating tables: %v", err)
	}
	return names
}

// foreignKeyFromColumns returns the "from" column of every foreign key on
// table — the column that must be indexed for a join or an ON DELETE check
// against the referenced table to avoid a full scan. table is always a name
// this file's own tableNames() produced.
func foreignKeyFromColumns(t *testing.T, ctx context.Context, database *sql.DB, table string) []string {
	t.Helper()
	rows, err := database.QueryContext(ctx, `PRAGMA foreign_key_list(`+QuoteIdent(DriverSQLite, table)+`)`) // SQL identifier: table, from tableNames()
	if err != nil {
		t.Fatalf("PRAGMA foreign_key_list(%s): %v", table, err)
	}
	defer CloseRows(rows)
	var cols []string
	for rows.Next() {
		var id, seq int
		var refTable, from, to string
		var onUpdate, onDelete, match string
		if err := rows.Scan(&id, &seq, &refTable, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			t.Fatalf("scanning foreign_key_list(%s): %v", table, err)
		}
		cols = append(cols, from)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating foreign_key_list(%s): %v", table, err)
	}
	return cols
}

// indexNames returns every index on table (including the implicit ones
// SQLite creates for UNIQUE/PRIMARY KEY). table is always a name this file's
// own tableNames() produced.
func indexNames(t *testing.T, ctx context.Context, database *sql.DB, table string) []string {
	t.Helper()
	rows, err := database.QueryContext(ctx, `PRAGMA index_list(`+QuoteIdent(DriverSQLite, table)+`)`) // SQL identifier: table, from tableNames()
	if err != nil {
		t.Fatalf("PRAGMA index_list(%s): %v", table, err)
	}
	defer CloseRows(rows)
	var indexes []string
	for rows.Next() {
		var seq int
		var name, origin string
		var unique, partial int
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			t.Fatalf("scanning index_list(%s): %v", table, err)
		}
		indexes = append(indexes, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating index_list(%s): %v", table, err)
	}
	return indexes
}

// indexLeadingColumn returns the name of idx's leading column — the only
// position that actually serves a lookup or join on that column without a
// full index scan — or "" if idx has none. idx is always a name this file's
// own indexNames() produced.
func indexLeadingColumn(t *testing.T, ctx context.Context, database *sql.DB, idx string) string {
	t.Helper()
	rows, err := database.QueryContext(ctx, `PRAGMA index_info(`+QuoteIdent(DriverSQLite, idx)+`)`) // SQL identifier: idx, from indexNames()
	if err != nil {
		t.Fatalf("PRAGMA index_info(%s): %v", idx, err)
	}
	defer CloseRows(rows)
	leading := ""
	for rows.Next() {
		var seqno, cid int
		var name sql.NullString
		if err := rows.Scan(&seqno, &cid, &name); err != nil {
			t.Fatalf("scanning index_info(%s): %v", idx, err)
		}
		if seqno == 0 && name.Valid {
			leading = name.String
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating index_info(%s): %v", idx, err)
	}
	return leading
}

// indexedLeadingColumns returns the set of column names that lead some index
// on table.
func indexedLeadingColumns(t *testing.T, ctx context.Context, database *sql.DB, table string) map[string]bool {
	t.Helper()
	leading := map[string]bool{}
	for _, idx := range indexNames(t, ctx, database, table) {
		if col := indexLeadingColumn(t, ctx, database, idx); col != "" {
			leading[col] = true
		}
	}
	return leading
}

// TestEveryForeignKeyIsIndexed fails when a foreign-key column has no index
// leading on it, which forces a full table scan on every join to the parent
// table and on every ON DELETE / ON UPDATE cascade check.
func TestEveryForeignKeyIsIndexed(t *testing.T) {
	ctx := t.Context()
	database, err := openSQLite(filepath.Join(t.TempDir(), "fk-index.db"))
	if err != nil {
		t.Fatalf("opening sqlite: %v", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			t.Logf("closing database: %v", err)
		}
	}()
	if err := Migrate(database, DriverSQLite); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	var unindexed []string
	stillUnindexed := map[string]bool{}
	for _, table := range tableNames(t, ctx, database) {
		leading := indexedLeadingColumns(t, ctx, database, table)
		for _, col := range foreignKeyFromColumns(t, ctx, database, table) {
			if leading[col] {
				continue
			}
			key := table + "." + col
			if knownUnindexedForeignKeys[key] {
				stillUnindexed[key] = true
				continue
			}
			unindexed = append(unindexed, key)
		}
	}
	for key := range knownUnindexedForeignKeys {
		if !stillUnindexed[key] {
			t.Errorf("%s is now indexed; remove it from knownUnindexedForeignKeys", key)
		}
	}
	sort.Strings(unindexed)
	if len(unindexed) > 0 {
		t.Errorf("%d foreign-key columns have no covering index:\n  %s",
			len(unindexed), strings.Join(unindexed, "\n  "))
	}
}
