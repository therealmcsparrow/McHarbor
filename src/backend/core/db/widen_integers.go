// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
)

// migrationQuerier is what a Go migration step needs: the executor the SQL body
// runs through, plus reads, because a step decides what to do from the live
// schema. Both *sql.Tx and *sql.DB satisfy it.
type migrationQuerier interface {
	migrationExecutor
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// migrationStep is Go work that runs inside a migration's transaction, after the
// file's SQL, for a change no single SQL file can express on every driver.
type migrationStep func(ctx context.Context, q migrationQuerier, driver Driver) error

// migrationSteps binds a Go step to a migration file by name, so the step runs
// exactly once, recorded under the same schema_migrations row as its file.
var migrationSteps = map[string]migrationStep{
	"119_widen_integer_columns.sql": widenIntegerColumns,
}

// integerColumn is one 32-bit column the widening step found, with what MySQL
// needs to restate the column in a MODIFY.
type integerColumn struct {
	table, column  string
	nullable       bool
	dflt           sql.NullString
	extra, colType string
}

// widenIntegerColumns turns every 32-bit integer column into BIGINT on an
// install migrated before the dialect widened INTEGER. The schema's contract is
// SQLite's 64-bit INTEGER, and capacity_samples.used_bytes past 2 GiB is what
// made Settings → System → Database refuse to copy into PostgreSQL.
//
// Every base-table column is widened rather than a hand-picked list: 306 of
// them exist, byte counts and counters are spread across dozens of tables, and
// a list would silently miss the next one. SQLite and SQL Server have nothing
// to widen — SQLite's INTEGER is already 64-bit and the SQL Server translation
// has always emitted BIGINT.
func widenIntegerColumns(ctx context.Context, q migrationQuerier, driver Driver) error {
	switch driver {
	case DriverPostgres, DriverMySQL:
	default:
		return nil
	}
	cols, skipped, err := narrowIntegerColumns(ctx, q, driver)
	if err != nil {
		return err
	}
	byTable := map[string][]integerColumn{}
	for _, c := range cols {
		byTable[c.table] = append(byTable[c.table], c)
	}
	tables := make([]string, 0, len(byTable))
	for table := range byTable {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	for _, table := range tables {
		// One ALTER per table, not per column: both engines rewrite the table
		// for a type change, and a table with six counters should pay once.
		stmt := widenTableStatement(driver, table, byTable[table])
		if _, err := q.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("widening integer columns of %s: %w", table, err)
		}
	}
	for _, s := range skipped {
		slog.Warn("integer column left 32-bit: a view or foreign key depends on it",
			"driver", driver, "table", s.table, "column", s.column)
	}
	slog.Info("integer columns widened to bigint", "driver", driver, "tables", len(tables), "columns", len(cols), "skipped", len(skipped))
	return nil
}

// narrowIntegerColumns lists the 32-bit integer columns of the current schema's
// base tables, separating out those a view or a foreign key depends on: Postgres
// refuses to retype a column a view reads, MySQL one a foreign key names.
func narrowIntegerColumns(ctx context.Context, q migrationQuerier, driver Driver) ([]integerColumn, []integerColumn, error) {
	var query string
	if driver == DriverPostgres {
		query = `SELECT c.table_name, c.column_name, c.is_nullable, c.column_default, '', c.data_type,
			       CASE WHEN EXISTS (SELECT 1 FROM information_schema.view_column_usage v
			                         WHERE v.table_schema = c.table_schema AND v.table_name = c.table_name
			                           AND v.column_name = c.column_name) THEN 1 ELSE 0 END
			FROM information_schema.columns c
			JOIN information_schema.tables t ON t.table_schema = c.table_schema AND t.table_name = c.table_name
			WHERE c.table_schema = current_schema() AND t.table_type = 'BASE TABLE' AND c.data_type = 'integer'
			ORDER BY c.table_name, c.ordinal_position
			LIMIT 10000`
	} else {
		query = `SELECT c.TABLE_NAME, c.COLUMN_NAME, c.IS_NULLABLE, c.COLUMN_DEFAULT, c.EXTRA, c.COLUMN_TYPE,
			       CASE WHEN EXISTS (SELECT 1 FROM information_schema.KEY_COLUMN_USAGE k
			                         WHERE k.TABLE_SCHEMA = c.TABLE_SCHEMA AND k.REFERENCED_TABLE_NAME IS NOT NULL
			                           AND ((k.TABLE_NAME = c.TABLE_NAME AND k.COLUMN_NAME = c.COLUMN_NAME)
			                             OR (k.REFERENCED_TABLE_NAME = c.TABLE_NAME AND k.REFERENCED_COLUMN_NAME = c.COLUMN_NAME)))
			            THEN 1 ELSE 0 END
			FROM information_schema.COLUMNS c
			JOIN information_schema.TABLES t ON t.TABLE_SCHEMA = c.TABLE_SCHEMA AND t.TABLE_NAME = c.TABLE_NAME
			WHERE c.TABLE_SCHEMA = DATABASE() AND t.TABLE_TYPE = 'BASE TABLE' AND c.DATA_TYPE = 'int'
			ORDER BY c.TABLE_NAME, c.ORDINAL_POSITION
			LIMIT 10000`
	}
	rows, err := q.QueryContext(ctx, query)
	if err != nil {
		return nil, nil, fmt.Errorf("listing integer columns: %w", err)
	}
	defer CloseRows(rows)
	var cols, skipped []integerColumn
	for rows.Next() {
		var c integerColumn
		var nullable string
		var blocked int
		if err := rows.Scan(&c.table, &c.column, &nullable, &c.dflt, &c.extra, &c.colType, &blocked); err != nil {
			return nil, nil, fmt.Errorf("scanning integer column: %w", err)
		}
		c.nullable = strings.EqualFold(nullable, "YES")
		if blocked != 0 {
			skipped = append(skipped, c)
			continue
		}
		cols = append(cols, c)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("reading integer columns: %w", err)
	}
	return cols, skipped, nil
}

// widenTableStatement renders one ALTER TABLE that widens every listed column.
// Postgres changes the type alone and keeps nullability and default; MySQL's
// MODIFY replaces the whole definition, so both are restated from the catalog.
func widenTableStatement(driver Driver, table string, cols []integerColumn) string {
	parts := make([]string, 0, len(cols))
	for _, c := range cols {
		name := QuoteIdent(driver, c.column)
		if driver == DriverPostgres {
			parts = append(parts, "ALTER COLUMN "+name+" TYPE BIGINT")
			continue
		}
		def := "MODIFY COLUMN " + name + " BIGINT"
		if strings.Contains(strings.ToLower(c.colType), "unsigned") {
			def += " UNSIGNED"
		}
		if !c.nullable {
			def += " NOT NULL"
		}
		if c.dflt.Valid {
			def += " DEFAULT " + mysqlDefault(c.dflt.String, c.extra)
		}
		if strings.Contains(strings.ToLower(c.extra), "auto_increment") {
			def += " AUTO_INCREMENT"
		}
		parts = append(parts, def)
	}
	return "ALTER TABLE " + QuoteIdent(driver, table) + " " + strings.Join(parts, ", ")
}

// mysqlDefault restates a column default as MySQL's catalog reports it: an
// expression default (EXTRA carries DEFAULT_GENERATED) goes back in parentheses,
// a number as-is, anything else as a quoted literal.
func mysqlDefault(value, extra string) string {
	if strings.Contains(strings.ToUpper(extra), "DEFAULT_GENERATED") {
		return "(" + value + ")"
	}
	if _, err := strconv.ParseInt(value, 10, 64); err == nil {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
