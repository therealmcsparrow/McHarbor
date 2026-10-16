// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sort"
	"strings"
)

// The delta pass — the second half of "nothing is lost".
//
// core/quiesce stops the background workers before the bulk copy, but a gate
// cannot un-write a tick that was already in flight when it closed, and it
// covers only the workers that have been wired to it. A worker added later
// without that line would be silently outside the guarantee. A pass that asks
// the destination what it is missing does not depend on knowing who the
// writers are, which is why both exist.

// PrimaryKeyColumn returns the single-column primary key of a table, or ""
// when the table has none or has a composite one.
//
// The delta pass needs a column that identifies a row so it can ask "which of
// these does the destination not have yet". McHarbor's schema makes that
// easy: every table carries an id, and every id is an xid minted by the
// application rather than a value the server assigns. Join tables that key on
// a pair fall back to a full re-copy instead.
func PrimaryKeyColumn(ctx context.Context, database *sql.DB, driver Driver, table string) (string, error) {
	var (
		rows *sql.Rows
		err  error
	)
	switch driver {
	case DriverPostgres:
		rows, err = database.QueryContext(ctx, `
			SELECT kcu.column_name
			  FROM information_schema.table_constraints tc
			  JOIN information_schema.key_column_usage kcu
			    ON kcu.constraint_name = tc.constraint_name
			   AND kcu.constraint_schema = tc.constraint_schema
			 WHERE tc.constraint_type = 'PRIMARY KEY'
			   AND tc.table_schema = current_schema()
			   AND tc.table_name = $1`, table)
	case DriverMySQL:
		rows, err = database.QueryContext(ctx, `
			SELECT column_name
			  FROM information_schema.key_column_usage
			 WHERE table_schema = DATABASE()
			   AND table_name = ?
			   AND constraint_name = 'PRIMARY'`, table)
	case DriverMSSQL:
		rows, err = database.QueryContext(ctx, `
			SELECT c.name
			  FROM sys.indexes i
			  JOIN sys.index_columns ic ON ic.object_id = i.object_id AND ic.index_id = i.index_id
			  JOIN sys.columns c ON c.object_id = ic.object_id AND c.column_id = ic.column_id
			 WHERE i.object_id = OBJECT_ID(?)
			   AND i.is_primary_key = 1`, table)
	default:
		return sqlitePrimaryKey(ctx, database, driver, table)
	}
	if err != nil {
		return "", fmt.Errorf("reading primary key of %s: %w", table, err)
	}
	defer CloseRows(rows)

	var cols []string
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			return "", fmt.Errorf("scanning primary key of %s: %w", table, err)
		}
		cols = append(cols, col)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterating primary key of %s: %w", table, err)
	}
	if len(cols) != 1 {
		return "", nil
	}
	return cols[0], nil
}

// sqlitePrimaryKey reads the key off PRAGMA table_info, where a non-zero pk
// column marks a key member and its value is the position within the key.
func sqlitePrimaryKey(ctx context.Context, database *sql.DB, driver Driver, table string) (string, error) {
	rows, err := database.QueryContext(ctx, "PRAGMA table_info("+QuoteIdent(driver, table)+")")
	if err != nil {
		return "", fmt.Errorf("reading primary key of %s: %w", table, err)
	}
	defer CloseRows(rows)

	names, err := rows.Columns()
	if err != nil {
		return "", fmt.Errorf("listing table_info columns of %s: %w", table, err)
	}
	nameIdx, pkIdx := -1, -1
	for i, n := range names {
		switch strings.ToLower(n) {
		case "name":
			nameIdx = i
		case "pk":
			pkIdx = i
		}
	}
	if nameIdx < 0 || pkIdx < 0 {
		return "", fmt.Errorf("PRAGMA table_info(%s) has no name/pk columns", table)
	}

	var cols []string
	for rows.Next() {
		cells := make([]any, len(names))
		for i := range cells {
			cells[i] = new(sql.NullString)
		}
		if err := rows.Scan(cells...); err != nil {
			return "", fmt.Errorf("scanning table_info of %s: %w", table, err)
		}
		pk, _ := cells[pkIdx].(*sql.NullString)
		name, _ := cells[nameIdx].(*sql.NullString)
		if pk != nil && pk.Valid && pk.String != "0" && name != nil && name.Valid {
			cols = append(cols, name.String)
		}
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterating table_info of %s: %w", table, err)
	}
	if len(cols) != 1 {
		return "", nil
	}
	return cols[0], nil
}

// DeltaOptions configures a delta pass.
type DeltaOptions struct {
	// Tables limits the pass to these tables. Empty means every shared table.
	Tables []string
	// BatchRows and OnProgress behave as in CopyOptions.
	BatchRows  int
	OnProgress func(CopyProgress)
	Logger     *slog.Logger
}

// DeltaReport is the outcome of one delta pass.
type DeltaReport struct {
	// Copied counts the rows added, per table. A table with nothing to add
	// does not appear.
	Copied map[string]int64 `json:"copied"`
	// Total is the sum of Copied.
	Total int64 `json:"total"`
	// Unkeyed names the tables re-copied wholesale because they have no
	// single-column primary key to compare on.
	Unkeyed []string `json:"unkeyed,omitempty"`
	// UntrustedTables is Report.UntrustedTables for the tables this pass wrote.
	UntrustedTables []string `json:"untrustedTables,omitempty"`
}

// CopyDelta copies rows that exist on the source but not on the destination.
//
// The comparison is by primary key rather than by timestamp: an xid is minted
// by the application and never reused, so "the destination does not have this
// id" is an exact answer, while a created_at window would have to guess at
// clock skew between two database servers.
func CopyDelta(ctx context.Context, src *sql.DB, srcDriver Driver, dst *sql.DB, dstDriver Driver, opts DeltaOptions) (report DeltaReport, err error) {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	report = DeltaReport{Copied: map[string]int64{}}

	tables := opts.Tables
	if len(tables) == 0 {
		shared, err := SharedTables(ctx, src, srcDriver, dst, dstDriver)
		if err != nil {
			return report, err
		}
		tables = shared
	}

	conn, err := dst.Conn(ctx)
	if err != nil {
		return report, fmt.Errorf("opening destination connection: %w", err)
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil {
			logger.Warn("db delta: closing destination connection", "error", closeErr)
		}
	}()
	// SQL Server's suspension is per table and re-validated on restore, so it is
	// applied only to the tables this pass actually writes, not to every table.
	suspension := suspendConstraints(ctx, conn, dstDriver, nil, logger)
	defer func() {
		untrusted, restoreErr := suspension.restore(ctx)
		report.UntrustedTables = untrusted
		if restoreErr != nil && err == nil {
			err = restoreErr
		}
	}()

	for i, table := range tables {
		if err := ctx.Err(); err != nil {
			return report, err
		}

		key, err := PrimaryKeyColumn(ctx, src, srcDriver, table)
		if err != nil {
			return report, err
		}
		if key == "" {
			suspension.include(ctx, table)
			// No single-column key to compare on. Clearing and re-copying is
			// the honest fallback: more work, but it cannot leave a
			// half-matched row behind.
			n, err := recopyTable(ctx, src, srcDriver, conn, dstDriver, table, i+1, len(tables), opts, logger)
			if err != nil {
				return report, err
			}
			report.Unkeyed = append(report.Unkeyed, table)
			if n > 0 {
				report.Copied[table] = n
				report.Total += n
			}
			continue
		}

		missing, err := missingKeys(ctx, src, srcDriver, conn, dstDriver, table, key)
		if err != nil {
			return report, err
		}
		if len(missing) == 0 {
			continue
		}

		suspension.include(ctx, table)
		n, err := copyRowsByKey(ctx, src, srcDriver, conn, dstDriver, table, key, missing, opts, i+1, len(tables), logger)
		if err != nil {
			return report, err
		}
		report.Copied[table] = n
		report.Total += n
		logger.Info("db delta: caught up rows written during the copy", "table", table, "rows", n)
	}
	return report, nil
}

// recopyTable clears a table on the destination and copies it again, for the
// tables no single-column key can address.
func recopyTable(ctx context.Context, src *sql.DB, srcDriver Driver, dst *sql.Conn, dstDriver Driver, table string, index, total int, opts DeltaOptions, logger *slog.Logger) (int64, error) {
	if _, err := dst.ExecContext(ctx, "DELETE FROM "+QuoteIdent(dstDriver, table)); err != nil {
		return 0, fmt.Errorf("clearing %s for a full delta re-copy: %w", table, err)
	}
	tr, err := copyTable(ctx, src, srcDriver, dst, dstDriver, table, copyTableParams{
		tableIndex: index,
		tableTotal: total,
		batchRows:  opts.BatchRows,
		onProgress: opts.OnProgress,
		logger:     logger,
	})
	if err != nil {
		return 0, err
	}
	return tr.Rows, nil
}

// SharedTables is the intersection CopyAll works over, in dependency order.
// Exported so the delta pass and the bulk copy cannot disagree about which
// tables are in scope.
func SharedTables(ctx context.Context, src *sql.DB, srcDriver Driver, dst *sql.DB, dstDriver Driver) ([]string, error) {
	srcTables, err := ListTables(ctx, src, srcDriver)
	if err != nil {
		return nil, fmt.Errorf("listing source tables: %w", err)
	}
	dstTables, err := ListTables(ctx, dst, dstDriver)
	if err != nil {
		return nil, fmt.Errorf("listing destination tables: %w", err)
	}
	dstSet := make(map[string]bool, len(dstTables))
	for _, t := range dstTables {
		dstSet[strings.ToLower(t)] = true
	}

	var out []string
	deps := map[string][]string{}
	for _, t := range srcTables {
		if skippedTables[strings.ToLower(t)] || !dstSet[strings.ToLower(t)] {
			continue
		}
		refs, err := TableReferences(ctx, src, srcDriver, t)
		if err != nil {
			return nil, err
		}
		deps[t] = refs
		out = append(out, t)
	}
	return TopoSort(out, deps), nil
}

// missingKeys returns the primary keys present on the source and absent on
// the destination.
//
// Both sides are read as key sets and compared in memory rather than with a
// NOT IN subquery, because the two tables live in different databases and no
// single statement can see both.
func missingKeys(ctx context.Context, src *sql.DB, srcDriver Driver, dst *sql.Conn, dstDriver Driver, table, key string) ([]string, error) {
	have := map[string]bool{}
	dstRows, err := dst.QueryContext(ctx, "SELECT "+QuoteIdent(dstDriver, key)+" FROM "+QuoteIdent(dstDriver, table))
	if err != nil {
		return nil, fmt.Errorf("reading destination keys of %s: %w", table, err)
	}
	if err := scanKeys(dstRows, have); err != nil {
		return nil, fmt.Errorf("scanning destination keys of %s: %w", table, err)
	}

	want := map[string]bool{}
	srcRows, err := src.QueryContext(ctx, "SELECT "+QuoteIdent(srcDriver, key)+" FROM "+QuoteIdent(srcDriver, table))
	if err != nil {
		return nil, fmt.Errorf("reading source keys of %s: %w", table, err)
	}
	if err := scanKeys(srcRows, want); err != nil {
		return nil, fmt.Errorf("scanning source keys of %s: %w", table, err)
	}

	var missing []string
	for k := range want {
		if !have[k] {
			missing = append(missing, k)
		}
	}
	sort.Strings(missing)
	return missing, nil
}

// scanKeys drains a single-column result into a set, closing the rows.
func scanKeys(rows *sql.Rows, into map[string]bool) error {
	defer CloseRows(rows)
	for rows.Next() {
		var key sql.NullString
		if err := rows.Scan(&key); err != nil {
			return err
		}
		if key.Valid {
			into[key.String] = true
		}
	}
	return rows.Err()
}

// deltaChunkRows sizes one catch-up chunk, which is both a SELECT ... IN (...)
// on the source (one parameter per key) and one INSERT on the destination (one
// per cell), so it must fit both budgets and honour the caller's BatchRows.
func deltaChunkRows(srcDriver, dstDriver Driver, columns, requested int) int {
	chunk := rowsPerStatement(dstDriver, columns, requested)
	if read := MaxBindParameters(srcDriver); read < chunk {
		chunk = read
	}
	return chunk
}

// copyRowsByKey copies the named rows in chunks sized to fit inside the
// tighter of the two drivers' bind-parameter budgets: one chunk is a
// SELECT ... IN (...) on the source and one INSERT on the destination.
func copyRowsByKey(ctx context.Context, src *sql.DB, srcDriver Driver, dst *sql.Conn, dstDriver Driver, table, key string, keys []string, opts DeltaOptions, tableIndex, tableTotal int, logger *slog.Logger) (int64, error) {
	srcCols, err := TableColumns(ctx, src, srcDriver, table)
	if err != nil {
		return 0, err
	}
	dstCols, err := columnsOnConn(ctx, dst, dstDriver, table)
	if err != nil {
		return 0, err
	}
	srcSet := map[string]string{}
	for _, c := range srcCols {
		srcSet[strings.ToLower(c)] = c
	}
	var cols []string
	for _, c := range dstCols {
		if original, ok := srcSet[strings.ToLower(c)]; ok {
			cols = append(cols, original)
		}
	}
	if len(cols) == 0 {
		return 0, fmt.Errorf("table %s has no columns in common between the source and the destination", table)
	}

	chunk := deltaChunkRows(srcDriver, dstDriver, len(cols), opts.BatchRows)

	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = QuoteIdent(srcDriver, c)
	}

	identityOff, err := identityInsertOn(ctx, dst, dstDriver, table, cols)
	if err != nil {
		return 0, err
	}
	defer identityOff()

	var copied int64
	for start := 0; start < len(keys); start += chunk {
		end := start + chunk
		if end > len(keys) {
			end = len(keys)
		}
		batch := keys[start:end]

		placeholders := make([]string, len(batch))
		args := make([]any, len(batch))
		for i, k := range batch {
			placeholders[i] = Placeholder(srcDriver, i+1)
			args[i] = k
		}
		query := "SELECT " + strings.Join(quoted, ", ") +
			" FROM " + QuoteIdent(srcDriver, table) +
			" WHERE " + QuoteIdent(srcDriver, key) + " IN (" + strings.Join(placeholders, ", ") + ")"

		n, err := copyResultSet(ctx, src, query, args, dst, dstDriver, table, cols, logger)
		if err != nil {
			return copied, err
		}
		copied += n

		if opts.OnProgress != nil {
			opts.OnProgress(CopyProgress{
				Table: table, TableIndex: tableIndex, TableTotal: tableTotal,
				RowsCopied: copied, RowsTotal: int64(len(keys)),
				TotalRowsCopied: copied, TotalRows: int64(len(keys)),
			})
		}
	}
	return copied, nil
}

// copyResultSet runs one source query and writes everything it returns.
func copyResultSet(ctx context.Context, src *sql.DB, query string, args []any, dst *sql.Conn, dstDriver Driver, table string, cols []string, logger *slog.Logger) (int64, error) {
	rows, err := src.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("reading a delta batch of %s: %w", table, err)
	}
	defer CloseRows(rows)

	binary, err := binaryColumns(rows)
	if err != nil {
		return 0, fmt.Errorf("reading column types of %s: %w", table, err)
	}

	values := make([]any, 0, len(cols)*8)
	pending := 0
	scan := make([]any, len(cols))
	holders := make([]any, len(cols))
	for i := range scan {
		scan[i] = &holders[i]
	}

	for rows.Next() {
		if err := rows.Scan(scan...); err != nil {
			return 0, fmt.Errorf("scanning a delta row of %s: %w", table, err)
		}
		for i := range holders {
			values = append(values, normalizeValue(holders[i], binary[i]))
		}
		pending++
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterating a delta batch of %s: %w", table, err)
	}
	if pending == 0 {
		return 0, nil
	}

	stmt := insertStatement(dstDriver, table, cols, pending)
	if _, err := dst.ExecContext(ctx, stmt, values...); err != nil {
		return 0, fmt.Errorf("writing %d delta rows into %s: %w", pending, table, err)
	}
	logger.Debug("db delta: batch written", "table", table, "rows", pending)
	return int64(pending), nil
}
