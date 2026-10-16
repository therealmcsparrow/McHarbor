// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// CopyAll moves every row McHarbor owns from one SQL backend to another.
//
// It works because the schema is already portable: all 100-plus migration
// files run unchanged on SQLite, Postgres and MySQL, no table uses an
// auto-incrementing key (every id is an xid stored as TEXT), and there are
// no sequences to reset afterwards. That reduces the problem to "read every
// row, write every row, in an order the foreign keys accept".
//
// The destination is expected to have had Migrate() run on it already, so
// its tables exist and are empty.

// skippedTables are never copied. schema_migrations belongs to the
// destination — Migrate() writes it there as it applies each file, and
// copying the source's rows over it would claim migrations that were never
// run against this backend.
var skippedTables = map[string]bool{
	"schema_migrations": true,
	"_migrations":       true,
}

// CopyProgress is one progress tick. It is delivered on the goroutine
// running the copy, so the callback must not block.
type CopyProgress struct {
	// Table is the table currently being copied.
	Table string
	// TableIndex is 1-based; TableTotal is how many tables will be copied.
	TableIndex int
	TableTotal int
	// RowsCopied / RowsTotal track the current table.
	RowsCopied int64
	RowsTotal  int64
	// TotalRowsCopied / TotalRows track the whole run.
	TotalRowsCopied int64
	TotalRows       int64
}

// CopyOptions configures a run. Every field is optional.
type CopyOptions struct {
	// BatchRows caps how many rows go into one INSERT. Zero picks a value
	// from the destination's bind-parameter ceiling and the column count.
	BatchRows int
	// OnProgress, when set, is called at the start of each table and after
	// each batch.
	OnProgress func(CopyProgress)
	// Logger receives the non-fatal notes: skipped columns, a destination
	// that would not suspend its foreign-key checks, drift on the source.
	Logger *slog.Logger
}

// TableReport is the per-table outcome of a copy.
type TableReport struct {
	Table string `json:"table"`
	// Rows is how many rows were written to the destination.
	Rows int64 `json:"rows"`
	// SourceRows is the count taken from the source after the copy
	// finished. It differs from Rows when the running application wrote to
	// the source mid-copy — see Report.Drifted.
	SourceRows int64 `json:"sourceRows"`
	// SkippedColumns are columns present on one side only.
	SkippedColumns []string `json:"skippedColumns,omitempty"`
	// Duration is how long this table took.
	Duration   time.Duration `json:"-"`
	DurationMS int64         `json:"durationMs"`
}

// Report is the outcome of a whole copy.
type Report struct {
	Tables []TableReport `json:"tables"`
	// TotalRows is the number of rows written across every table.
	TotalRows int64 `json:"totalRows"`
	// SkippedTables are tables that exist on one side only.
	SkippedTables []string `json:"skippedTables,omitempty"`
	// Drifted names the tables whose source count changed during the copy.
	// This is expected on a live install — the metrics collector, the event
	// collector and the audit log all keep writing while the copy runs — and
	// is reported rather than prevented, because preventing it would mean
	// taking the application down for the duration.
	Drifted []string `json:"drifted,omitempty"`
	// ForeignKeysRelaxed says whether the destination agreed to suspend
	// foreign-key enforcement for the copy connection. When false the copy
	// relied entirely on the topological order.
	ForeignKeysRelaxed bool `json:"foreignKeysRelaxed"`
	// UntrustedTables names the SQL Server tables whose copied rows failed the
	// foreign-key re-validation on restore (orphans the source let through); their
	// constraints are enforced again, but for new writes only.
	UntrustedTables []string      `json:"untrustedTables,omitempty"`
	Duration        time.Duration `json:"-"`
	DurationMS      int64         `json:"durationMs"`
}

// CopyAll copies src into dst. Both must already have the McHarbor schema.
//
// All destination writes go through a single dedicated connection, because
// the statements that suspend foreign-key enforcement are per-connection on
// SQLite, Postgres and MySQL (SQL Server's are per table; see
// suspendConstraints). Rows are streamed out of the source and
// written in multi-row batches sized to stay under the destination's bind
// parameter ceiling.
//
// The destination's tables are emptied first, in reverse dependency order,
// so re-running a failed copy replaces rather than duplicates.
func CopyAll(ctx context.Context, src *sql.DB, srcDriver Driver, dst *sql.DB, dstDriver Driver, opts CopyOptions) (report Report, err error) {
	started := time.Now()
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	srcTables, err := ListTables(ctx, src, srcDriver)
	if err != nil {
		return report, fmt.Errorf("listing source tables: %w", err)
	}
	dstTables, err := ListTables(ctx, dst, dstDriver)
	if err != nil {
		return report, fmt.Errorf("listing destination tables: %w", err)
	}
	dstSet := make(map[string]bool, len(dstTables))
	for _, t := range dstTables {
		dstSet[strings.ToLower(t)] = true
	}

	var tables []string
	for _, t := range srcTables {
		if skippedTables[strings.ToLower(t)] {
			continue
		}
		if !dstSet[strings.ToLower(t)] {
			report.SkippedTables = append(report.SkippedTables, t)
			logger.Warn("db copy: table missing on destination, skipping", "table", t)
			continue
		}
		tables = append(tables, t)
	}
	if len(tables) == 0 {
		return report, fmt.Errorf("no tables in common between the source and the destination")
	}

	deps := make(map[string][]string, len(tables))
	for _, t := range tables {
		refs, err := TableReferences(ctx, src, srcDriver, t)
		if err != nil {
			return report, err
		}
		deps[t] = refs
	}
	ordered := TopoSort(tables, deps)

	conn, err := dst.Conn(ctx)
	if err != nil {
		return report, fmt.Errorf("opening destination connection: %w", err)
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil {
			logger.Warn("db copy: closing destination connection", "error", closeErr)
		}
	}()

	// Every destination table, not only the shared ones: on SQL Server the rows
	// Migrate seeded into a destination-only table still reference rows the clear
	// below deletes. Restored on every exit, because SQL Server's switch outlives
	// the connection.
	suspension := suspendConstraints(ctx, conn, dstDriver, dstTables, logger)
	report.ForeignKeysRelaxed = suspension.relaxed()
	defer func() {
		untrusted, restoreErr := suspension.restore(ctx)
		report.UntrustedTables = untrusted
		if restoreErr != nil && err == nil {
			err = restoreErr
		}
	}()

	// Clear the destination in reverse dependency order so children go
	// before parents even when foreign keys are still being enforced.
	for i := len(ordered) - 1; i >= 0; i-- {
		stmt := "DELETE FROM " + QuoteIdent(dstDriver, ordered[i])
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return report, fmt.Errorf("clearing destination table %s: %w", ordered[i], err)
		}
	}

	// Inventory first so the progress modal can show a real percentage
	// instead of a spinner.
	counts := make(map[string]int64, len(ordered))
	for _, t := range ordered {
		n, err := RowCount(ctx, src, srcDriver, t)
		if err != nil {
			return report, err
		}
		counts[t] = n
		report.TotalRows += n
	}
	plannedTotal := report.TotalRows
	report.TotalRows = 0

	var copied int64
	for i, table := range ordered {
		if err := ctx.Err(); err != nil {
			return report, err
		}

		tableStarted := time.Now()
		if opts.OnProgress != nil {
			opts.OnProgress(CopyProgress{
				Table: table, TableIndex: i + 1, TableTotal: len(ordered),
				RowsCopied: 0, RowsTotal: counts[table],
				TotalRowsCopied: copied, TotalRows: plannedTotal,
			})
		}

		tr, err := copyTable(ctx, src, srcDriver, conn, dstDriver, table, copyTableParams{
			expectedRows: counts[table],
			tableIndex:   i + 1,
			tableTotal:   len(ordered),
			copiedSoFar:  copied,
			plannedTotal: plannedTotal,
			batchRows:    opts.BatchRows,
			onProgress:   opts.OnProgress,
			logger:       logger,
		})
		if err != nil {
			return report, err
		}

		after, err := RowCount(ctx, src, srcDriver, table)
		if err != nil {
			return report, err
		}
		tr.SourceRows = after
		if after != tr.Rows {
			report.Drifted = append(report.Drifted, table)
			logger.Info("db copy: source table changed during the copy",
				"table", table, "copied", tr.Rows, "sourceNow", after)
		}
		tr.Duration = time.Since(tableStarted)
		tr.DurationMS = tr.Duration.Milliseconds()

		copied += tr.Rows
		report.TotalRows = copied
		report.Tables = append(report.Tables, tr)
	}

	report.Duration = time.Since(started)
	report.DurationMS = report.Duration.Milliseconds()
	return report, nil
}

// copyTableParams bundles the per-table inputs so copyTable keeps a
// readable signature.
type copyTableParams struct {
	expectedRows int64
	tableIndex   int
	tableTotal   int
	copiedSoFar  int64
	plannedTotal int64
	batchRows    int
	onProgress   func(CopyProgress)
	logger       *slog.Logger
}

// copyTable streams one table from the source into the destination.
func copyTable(ctx context.Context, src *sql.DB, srcDriver Driver, dst *sql.Conn, dstDriver Driver, table string, p copyTableParams) (TableReport, error) {
	tr := TableReport{Table: table}

	srcCols, err := TableColumns(ctx, src, srcDriver, table)
	if err != nil {
		return tr, err
	}
	dstColsRaw, err := columnsOnConn(ctx, dst, dstDriver, table)
	if err != nil {
		return tr, err
	}

	srcSet := make(map[string]string, len(srcCols))
	for _, c := range srcCols {
		srcSet[strings.ToLower(c)] = c
	}
	dstSet := make(map[string]bool, len(dstColsRaw))
	for _, c := range dstColsRaw {
		dstSet[strings.ToLower(c)] = true
	}

	// Destination order, so the INSERT column list matches the table as the
	// destination declares it.
	var cols []string
	for _, c := range dstColsRaw {
		if original, ok := srcSet[strings.ToLower(c)]; ok {
			cols = append(cols, original)
		} else {
			tr.SkippedColumns = append(tr.SkippedColumns, c)
		}
	}
	for _, c := range srcCols {
		if !dstSet[strings.ToLower(c)] {
			tr.SkippedColumns = append(tr.SkippedColumns, c)
		}
	}
	if len(cols) == 0 {
		return tr, fmt.Errorf("table %s has no columns in common between the source and the destination", table)
	}
	if len(tr.SkippedColumns) > 0 {
		p.logger.Warn("db copy: columns present on one side only", "table", table, "columns", tr.SkippedColumns)
	}

	batch := rowsPerStatement(dstDriver, len(cols), p.batchRows)

	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = QuoteIdent(srcDriver, c)
	}
	selectSQL := "SELECT " + strings.Join(quoted, ", ") + " FROM " + QuoteIdent(srcDriver, table)

	rows, err := src.QueryContext(ctx, selectSQL)
	if err != nil {
		return tr, fmt.Errorf("reading %s: %w", table, err)
	}
	defer CloseRows(rows)

	binary, err := binaryColumns(rows)
	if err != nil {
		return tr, fmt.Errorf("reading column types of %s: %w", table, err)
	}

	identityOff, err := identityInsertOn(ctx, dst, dstDriver, table, cols)
	if err != nil {
		return tr, err
	}
	defer identityOff()

	pending := make([]any, 0, batch*len(cols))
	pendingRows := 0

	flush := func() error {
		if pendingRows == 0 {
			return nil
		}
		stmt := insertStatement(dstDriver, table, cols, pendingRows)
		if _, err := dst.ExecContext(ctx, stmt, pending...); err != nil {
			return fmt.Errorf("writing %d rows into %s: %w", pendingRows, table, err)
		}
		tr.Rows += int64(pendingRows)
		pending = pending[:0]
		pendingRows = 0
		if p.onProgress != nil {
			p.onProgress(CopyProgress{
				Table: table, TableIndex: p.tableIndex, TableTotal: p.tableTotal,
				RowsCopied: tr.Rows, RowsTotal: p.expectedRows,
				TotalRowsCopied: p.copiedSoFar + tr.Rows, TotalRows: p.plannedTotal,
			})
		}
		return nil
	}

	scan := make([]any, len(cols))
	holders := make([]any, len(cols))
	for i := range scan {
		scan[i] = &holders[i]
	}

	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return tr, err
		}
		if err := rows.Scan(scan...); err != nil {
			return tr, fmt.Errorf("scanning a row of %s: %w", table, err)
		}
		for i := range holders {
			pending = append(pending, normalizeValue(holders[i], binary[i]))
		}
		pendingRows++
		if pendingRows >= batch {
			if err := flush(); err != nil {
				return tr, err
			}
		}
	}
	if err := rows.Err(); err != nil {
		return tr, fmt.Errorf("iterating %s: %w", table, err)
	}
	if err := flush(); err != nil {
		return tr, err
	}
	return tr, nil
}

// columnsOnConn is TableColumns against a dedicated connection, so the
// destination column list is read on the same session the writes use.
func columnsOnConn(ctx context.Context, conn *sql.Conn, driver Driver, table string) ([]string, error) {
	query := "SELECT * FROM " + QuoteIdent(driver, table) + " WHERE 1 = 0"
	rows, err := conn.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("reading destination columns of %s: %w", table, err)
	}
	defer CloseRows(rows)
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("listing destination columns of %s: %w", table, err)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating destination columns of %s: %w", table, err)
	}
	return cols, nil
}

// insertStatement builds a multi-row INSERT with the right placeholder
// syntax for the destination driver.
func insertStatement(driver Driver, table string, cols []string, rowCount int) string {
	var b strings.Builder
	b.WriteString("INSERT INTO ")
	b.WriteString(QuoteIdent(driver, table))
	b.WriteString(" (")
	for i, c := range cols {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(QuoteIdent(driver, c))
	}
	b.WriteString(") VALUES ")

	n := 0
	for r := 0; r < rowCount; r++ {
		if r > 0 {
			b.WriteString(", ")
		}
		b.WriteString("(")
		for c := range cols {
			if c > 0 {
				b.WriteString(", ")
			}
			n++
			b.WriteString(Placeholder(driver, n))
		}
		b.WriteString(")")
	}
	return b.String()
}

// binaryColumns reports, per column, whether the source declares it as a
// binary type.
//
// This matters because SQLite hands TEXT back as []byte for some values,
// and pgx binds a []byte as bytea — which a Postgres TEXT column rejects.
// Converting []byte to string is therefore right for every column except a
// genuinely binary one, and McHarbor's schema currently has none (randomblob
// appears only inside id-generating expressions). The check is the guard
// that keeps that true if a BLOB column is ever added.
func binaryColumns(rows *sql.Rows) ([]bool, error) {
	types, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	out := make([]bool, len(types))
	for i, t := range types {
		name := strings.ToUpper(t.DatabaseTypeName())
		out[i] = strings.Contains(name, "BLOB") ||
			strings.Contains(name, "BINARY") ||
			strings.Contains(name, "BYTEA")
	}
	return out, nil
}

// normalizeValue converts a scanned value into something every destination
// driver accepts. See binaryColumns for why []byte is decoded.
func normalizeValue(v any, isBinary bool) any {
	if b, ok := v.([]byte); ok && !isBinary {
		return string(b)
	}
	return v
}
