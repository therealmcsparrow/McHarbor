// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// Introspection helpers shared by the cross-driver data copier and by the
// connection tester in modules/database. Everything here is read-only
// catalog access; nothing takes caller input into a SQL string except a
// table name, which is always run through QuoteIdent first and only ever
// comes from a catalog listing this file produced.

// QuoteIdent wraps an identifier for the active driver. SQLite and Postgres
// use double quotes; MySQL uses backticks. An embedded quote character is
// doubled, which is the escape all three accept.
func QuoteIdent(driver Driver, ident string) string {
	if driver == DriverMySQL {
		return "`" + strings.ReplaceAll(ident, "`", "``") + "`"
	}
	return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`
}

// Placeholder renders the n-th bind marker (1-based). Postgres numbers its
// placeholders; SQLite and MySQL do not.
func Placeholder(driver Driver, n int) string {
	if driver == DriverPostgres {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

// MaxBindParameters is the per-statement bind-variable ceiling the copier
// must stay under when it builds multi-row INSERTs. The real limits are
// 32766 (SQLite 3.32+), 65535 (Postgres wire protocol), 65535 (MySQL
// prepared statements) and 2100 (SQL Server RPC); each value below leaves headroom.
func MaxBindParameters(driver Driver) int {
	switch driver {
	case DriverPostgres, DriverMySQL:
		return 60000
	case DriverMSSQL:
		return 2000
	default:
		return 30000
	}
}

// ListTables returns every base table in the database the connection is
// pointed at, sorted by name. SQLite internal tables (sqlite_*) are
// excluded — they are not part of any McHarbor schema and cannot be
// written to directly.
func ListTables(ctx context.Context, database *sql.DB, driver Driver) ([]string, error) {
	var query string
	switch driver {
	case DriverPostgres:
		query = `SELECT tablename FROM pg_tables WHERE schemaname = current_schema()`
	case DriverMySQL:
		query = `SELECT table_name FROM information_schema.tables
		          WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE'`
	case DriverMSSQL:
		query = `SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES
		          WHERE TABLE_SCHEMA = SCHEMA_NAME() AND TABLE_TYPE = 'BASE TABLE'`
	default:
		query = `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`
	}

	rows, err := database.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("listing tables: %w", err)
	}
	defer CloseRows(rows)

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scanning table name: %w", err)
		}
		if strings.HasPrefix(strings.ToLower(name), "sqlite_") {
			continue
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating table names: %w", err)
	}
	sort.Strings(tables)
	return tables, nil
}

// TableColumns returns a table's column names in declaration order.
//
// The implementation is deliberately catalog-free: `SELECT * ... WHERE 1=0`
// makes the driver itself report the column list, which is identical on all
// three backends and immune to the schema/catalog naming differences that a
// three-way information_schema query would have to work around.
func TableColumns(ctx context.Context, database *sql.DB, driver Driver, table string) ([]string, error) {
	query := "SELECT * FROM " + QuoteIdent(driver, table) + " WHERE 1 = 0"
	rows, err := database.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("reading columns of %s: %w", table, err)
	}
	defer CloseRows(rows)

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("listing columns of %s: %w", table, err)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating columns of %s: %w", table, err)
	}
	return cols, nil
}

// UniqueKeys returns the column list of every unique index and primary key on a
// table, so a caller can predict a collision before writing instead of catching it.
//
// Partial and expression indexes are left out: a collision against them cannot be
// predicted from column values alone, and a caller that misses one gets a refused
// write, which is recoverable, where a false collision would silently skip a row.
func UniqueKeys(ctx context.Context, database *sql.DB, driver Driver, table string) ([][]string, error) {
	var (
		rows *sql.Rows
		err  error
	)
	switch driver {
	case DriverPostgres:
		rows, err = database.QueryContext(ctx, `
			SELECT ic.relname, a.attname
			  FROM pg_index x
			  JOIN pg_class tc ON tc.oid = x.indrelid
			  JOIN pg_namespace n ON n.oid = tc.relnamespace
			  JOIN pg_class ic ON ic.oid = x.indexrelid
			  JOIN LATERAL unnest(x.indkey) WITH ORDINALITY AS k(attnum, ord) ON TRUE
			  LEFT JOIN pg_attribute a ON a.attrelid = tc.oid AND a.attnum = k.attnum
			 WHERE x.indisunique AND x.indpred IS NULL
			   AND k.ord <= x.indnkeyatts
			   AND n.nspname = current_schema()
			   AND tc.relname = $1
			 ORDER BY ic.relname, k.ord`, table)
	case DriverMySQL:
		rows, err = database.QueryContext(ctx, `
			SELECT index_name, column_name
			  FROM information_schema.statistics
			 WHERE table_schema = DATABASE()
			   AND table_name = ?
			   AND non_unique = 0
			 ORDER BY index_name, seq_in_index`, table)
	case DriverMSSQL:
		rows, err = database.QueryContext(ctx, `
			SELECT i.name, c.name
			  FROM sys.indexes i
			  JOIN sys.index_columns ic ON ic.object_id = i.object_id AND ic.index_id = i.index_id
			  JOIN sys.columns c ON c.object_id = ic.object_id AND c.column_id = ic.column_id
			 WHERE i.object_id = OBJECT_ID(?)
			   AND i.is_unique = 1
			   AND i.has_filter = 0
			   AND ic.is_included_column = 0
			 ORDER BY i.name, ic.key_ordinal`, table)
	default:
		return sqliteUniqueKeys(ctx, database, driver, table)
	}
	if err != nil {
		return nil, fmt.Errorf("reading unique keys of %s: %w", table, err)
	}
	defer CloseRows(rows)

	var order []string
	byIndex := map[string][]string{}
	broken := map[string]bool{}
	for rows.Next() {
		var index, column sql.NullString
		if err := rows.Scan(&index, &column); err != nil {
			return nil, fmt.Errorf("scanning unique key of %s: %w", table, err)
		}
		if _, seen := byIndex[index.String]; !seen {
			order = append(order, index.String)
			byIndex[index.String] = nil
		}
		if !column.Valid || column.String == "" {
			broken[index.String] = true
			continue
		}
		byIndex[index.String] = append(byIndex[index.String], column.String)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating unique keys of %s: %w", table, err)
	}

	var keys [][]string
	for _, index := range order {
		if !broken[index] && len(byIndex[index]) > 0 {
			keys = append(keys, byIndex[index])
		}
	}
	return keys, nil
}

// sqliteUniqueKeys reads PRAGMA index_list and index_info by column name, since
// both pragmas have gained columns across SQLite releases.
func sqliteUniqueKeys(ctx context.Context, database *sql.DB, driver Driver, table string) ([][]string, error) {
	lists, err := pragmaRows(ctx, database, "PRAGMA index_list("+QuoteIdent(driver, table)+")")
	if err != nil {
		return nil, fmt.Errorf("reading indexes of %s: %w", table, err)
	}
	var names []string
	for _, row := range lists {
		if row["unique"] == "1" && row["partial"] != "1" {
			names = append(names, row["name"])
		}
	}
	sort.Strings(names)

	var keys [][]string
	for _, name := range names {
		info, err := pragmaRows(ctx, database, "PRAGMA index_info("+QuoteIdent(driver, name)+")")
		if err != nil {
			return nil, fmt.Errorf("reading index %s of %s: %w", name, table, err)
		}
		var cols []string
		for _, row := range info {
			if row["name"] == "" {
				cols = nil
				break
			}
			cols = append(cols, row["name"])
		}
		if len(cols) > 0 {
			keys = append(keys, cols)
		}
	}
	return keys, nil
}

// pragmaRows runs a PRAGMA and returns each row keyed by lowercase column name,
// fully read and closed so the caller can issue the next pragma on a one-connection pool.
func pragmaRows(ctx context.Context, database *sql.DB, query string) ([]map[string]string, error) {
	rows, err := database.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer CloseRows(rows)

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []map[string]string
	for rows.Next() {
		cells := make([]any, len(cols))
		for i := range cells {
			cells[i] = new(sql.NullString)
		}
		if err := rows.Scan(cells...); err != nil {
			return nil, err
		}
		row := make(map[string]string, len(cols))
		for i, c := range cols {
			if v, ok := cells[i].(*sql.NullString); ok && v.Valid {
				row[strings.ToLower(c)] = v.String
			}
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// TableReferences returns the names of the tables a table's foreign keys
// point at, excluding self-references. The copier uses this to decide the
// insertion order so a child row never lands before its parent.
func TableReferences(ctx context.Context, database *sql.DB, driver Driver, table string) ([]string, error) {
	var (
		rows *sql.Rows
		err  error
	)
	switch driver {
	case DriverPostgres:
		rows, err = database.QueryContext(ctx, `
			SELECT DISTINCT ccu.table_name
			  FROM information_schema.table_constraints tc
			  JOIN information_schema.constraint_column_usage ccu
			    ON ccu.constraint_name = tc.constraint_name
			   AND ccu.constraint_schema = tc.constraint_schema
			 WHERE tc.constraint_type = 'FOREIGN KEY'
			   AND tc.table_schema = current_schema()
			   AND tc.table_name = $1`, table)
	case DriverMySQL:
		rows, err = database.QueryContext(ctx, `
			SELECT DISTINCT referenced_table_name
			  FROM information_schema.key_column_usage
			 WHERE table_schema = DATABASE()
			   AND table_name = ?
			   AND referenced_table_name IS NOT NULL`, table)
	case DriverMSSQL:
		// The second half reads the edges the migration translator moved out of
		// sys.foreign_keys into trg_mch_fk_<child>_<column> triggers on the parent,
		// to avoid error 1785; without them a child could be ordered before its parent.
		rows, err = database.QueryContext(ctx, `
			SELECT OBJECT_NAME(fk.referenced_object_id)
			  FROM sys.foreign_keys fk
			 WHERE fk.parent_object_id = OBJECT_ID(?)
			UNION
			SELECT OBJECT_NAME(tr.parent_id)
			  FROM sys.triggers tr
			  JOIN sys.columns c ON c.object_id = OBJECT_ID(?)
			 WHERE tr.name = 'trg_mch_fk_' + LOWER(?) + '_' + c.name`, table, table, table)
	default:
		// PRAGMA takes no bind parameters in modernc's driver, so the
		// table name is interpolated — it comes from ListTables, never
		// from a request, and is quoted the same way every other
		// identifier in this file is.
		rows, err = database.QueryContext(ctx, "PRAGMA foreign_key_list("+QuoteIdent(driver, table)+")")
	}
	if err != nil {
		return nil, fmt.Errorf("reading foreign keys of %s: %w", table, err)
	}
	defer CloseRows(rows)

	if driver == DriverSQLite {
		return scanSQLiteForeignKeys(rows, table)
	}

	var refs []string
	for rows.Next() {
		var ref sql.NullString
		if err := rows.Scan(&ref); err != nil {
			return nil, fmt.Errorf("scanning foreign key of %s: %w", table, err)
		}
		if ref.Valid && ref.String != "" && !strings.EqualFold(ref.String, table) {
			refs = append(refs, ref.String)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating foreign keys of %s: %w", table, err)
	}
	return refs, nil
}

// scanSQLiteForeignKeys pulls the `table` column out of a
// PRAGMA foreign_key_list result. The pragma's column set has grown across
// SQLite releases, so the row is scanned generically and addressed by the
// column name the pragma reports rather than by a fixed index.
func scanSQLiteForeignKeys(rows *sql.Rows, table string) ([]string, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("listing foreign key columns of %s: %w", table, err)
	}
	target := -1
	for i, c := range cols {
		if strings.EqualFold(c, "table") {
			target = i
			break
		}
	}
	if target < 0 {
		return nil, fmt.Errorf("PRAGMA foreign_key_list(%s) has no `table` column", table)
	}

	var refs []string
	for rows.Next() {
		cells := make([]any, len(cols))
		for i := range cells {
			cells[i] = new(sql.NullString)
		}
		if err := rows.Scan(cells...); err != nil {
			return nil, fmt.Errorf("scanning foreign key of %s: %w", table, err)
		}
		ref, ok := cells[target].(*sql.NullString)
		if !ok || !ref.Valid || ref.String == "" || strings.EqualFold(ref.String, table) {
			continue
		}
		refs = append(refs, ref.String)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating foreign keys of %s: %w", table, err)
	}
	return refs, nil
}

// ForeignKey is one column-level foreign key relation: a child column and
// the parent column it points at.
type ForeignKey struct {
	Column    string
	RefTable  string
	RefColumn string
}

// ForeignKeys returns every foreign key declared on a table, at column
// granularity. TableReferences answers "which tables", which is all the
// copier's ordering needs; an orphan query needs "which column, at which".
func ForeignKeys(ctx context.Context, database *sql.DB, driver Driver, table string) ([]ForeignKey, error) {
	switch driver {
	case DriverPostgres:
		return postgresForeignKeys(ctx, database, table)
	case DriverMySQL:
		return queryForeignKeys(ctx, database, `
			SELECT column_name, referenced_table_name, referenced_column_name
			  FROM information_schema.key_column_usage
			 WHERE table_schema = DATABASE()
			   AND table_name = ?
			   AND referenced_table_name IS NOT NULL`, table)
	default:
		return sqliteForeignKeys(ctx, database, driver, table)
	}
}

// postgresForeignKeys reads column-level foreign keys from information_schema,
// dropping any composite constraint rather than reporting a wrong column pair.
//
// key_column_usage carries an ordinal_position; constraint_column_usage does
// not carry one in any supported Postgres version, so the two sides cannot be
// joined positionally and a multi-column key cross-products into N*N rows of
// mismatched (Column, RefColumn) pairs. A constraint that yields more than one
// row is therefore composite by construction and is skipped — an under-report
// is recoverable, an orphan DELETE against the wrong column is not.
func postgresForeignKeys(ctx context.Context, database *sql.DB, table string) ([]ForeignKey, error) {
	const query = `
		SELECT tc.constraint_name, kcu.column_name, ccu.table_name, ccu.column_name
		  FROM information_schema.table_constraints tc
		  JOIN information_schema.key_column_usage kcu
		    ON kcu.constraint_name = tc.constraint_name
		   AND kcu.constraint_schema = tc.constraint_schema
		  JOIN information_schema.constraint_column_usage ccu
		    ON ccu.constraint_name = tc.constraint_name
		   AND ccu.constraint_schema = tc.constraint_schema
		 WHERE tc.constraint_type = 'FOREIGN KEY'
		   AND tc.table_schema = current_schema()
		   AND tc.table_name = $1`

	rows, err := database.QueryContext(ctx, query, table)
	if err != nil {
		return nil, fmt.Errorf("reading foreign keys of %s: %w", table, err)
	}
	defer CloseRows(rows)

	var order []string
	byConstraint := map[string][]ForeignKey{}
	for rows.Next() {
		var name, col, refTable, refCol sql.NullString
		if err := rows.Scan(&name, &col, &refTable, &refCol); err != nil {
			return nil, fmt.Errorf("scanning foreign key of %s: %w", table, err)
		}
		if !col.Valid || !refTable.Valid || refTable.String == "" {
			continue
		}
		key := ForeignKey{Column: col.String, RefTable: refTable.String, RefColumn: refCol.String}
		if key.RefColumn == "" {
			key.RefColumn = "id"
		}
		if _, seen := byConstraint[name.String]; !seen {
			order = append(order, name.String)
		}
		byConstraint[name.String] = append(byConstraint[name.String], key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating foreign keys of %s: %w", table, err)
	}

	var keys []ForeignKey
	for _, name := range order {
		group := byConstraint[name]
		if len(group) != 1 {
			continue
		}
		keys = append(keys, group[0])
	}
	return keys, nil
}

// queryForeignKeys runs a three-column foreign key query and scans it.
func queryForeignKeys(ctx context.Context, database *sql.DB, query, table string) ([]ForeignKey, error) {
	rows, err := database.QueryContext(ctx, query, table)
	if err != nil {
		return nil, fmt.Errorf("reading foreign keys of %s: %w", table, err)
	}
	defer CloseRows(rows)

	var keys []ForeignKey
	for rows.Next() {
		var col, refTable, refCol sql.NullString
		if err := rows.Scan(&col, &refTable, &refCol); err != nil {
			return nil, fmt.Errorf("scanning foreign key of %s: %w", table, err)
		}
		if !col.Valid || !refTable.Valid || refTable.String == "" {
			continue
		}
		key := ForeignKey{Column: col.String, RefTable: refTable.String, RefColumn: refCol.String}
		if key.RefColumn == "" {
			key.RefColumn = "id"
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating foreign keys of %s: %w", table, err)
	}
	return keys, nil
}

// sqliteForeignKeys reads PRAGMA foreign_key_list by column name rather than
// by index, because the pragma's column set has grown across SQLite releases.
func sqliteForeignKeys(ctx context.Context, database *sql.DB, driver Driver, table string) ([]ForeignKey, error) {
	// PRAGMA takes no bind parameters in modernc's driver, so the table name
	// is interpolated — it comes from ListTables, never from a request, and
	// is quoted the same way every other identifier in this file is.
	rows, err := database.QueryContext(ctx, "PRAGMA foreign_key_list("+QuoteIdent(driver, table)+")")
	if err != nil {
		return nil, fmt.Errorf("reading foreign keys of %s: %w", table, err)
	}
	defer CloseRows(rows)

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("listing foreign key columns of %s: %w", table, err)
	}
	idx := map[string]int{}
	for i, c := range cols {
		idx[strings.ToLower(c)] = i
	}
	tableAt, okTable := idx["table"]
	fromAt, okFrom := idx["from"]
	toAt, okTo := idx["to"]
	if !okTable || !okFrom {
		return nil, fmt.Errorf("PRAGMA foreign_key_list(%s) is missing expected columns", table)
	}
	if !okTo {
		toAt = -1
	}

	var keys []ForeignKey
	for rows.Next() {
		cells := make([]any, len(cols))
		for i := range cells {
			cells[i] = new(sql.NullString)
		}
		if err := rows.Scan(cells...); err != nil {
			return nil, fmt.Errorf("scanning foreign key of %s: %w", table, err)
		}
		read := func(at int) string {
			if at < 0 || at >= len(cells) {
				return ""
			}
			v, ok := cells[at].(*sql.NullString)
			if !ok || !v.Valid {
				return ""
			}
			return v.String
		}
		refTable, from := read(tableAt), read(fromAt)
		if refTable == "" || from == "" {
			continue
		}
		to := read(toAt)
		if to == "" {
			// A pragma row with a NULL `to` means the key points at the
			// parent's primary key, which is `id` on every McHarbor table.
			to = "id"
		}
		keys = append(keys, ForeignKey{Column: from, RefTable: refTable, RefColumn: to})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating foreign keys of %s: %w", table, err)
	}
	return keys, nil
}

// RowCount returns the number of rows in a table.
func RowCount(ctx context.Context, database *sql.DB, driver Driver, table string) (int64, error) {
	var n int64
	query := "SELECT COUNT(*) FROM " + QuoteIdent(driver, table)
	if err := database.QueryRowContext(ctx, query).Scan(&n); err != nil {
		return 0, fmt.Errorf("counting rows in %s: %w", table, err)
	}
	return n, nil
}

// TopoSort orders tables so that every table appears after the tables it
// references. deps maps a table to the tables it points at; references to
// tables outside the input set are ignored.
//
// A cycle cannot be ordered, so the tables still in the graph when Kahn's
// algorithm stalls are appended in their original order. The copier relaxes
// foreign-key enforcement on the destination connection anyway, so a cycle
// degrades to "unordered" rather than "broken".
func TopoSort(tables []string, deps map[string][]string) []string {
	inSet := make(map[string]bool, len(tables))
	for _, t := range tables {
		inSet[t] = true
	}

	remaining := make(map[string]map[string]bool, len(tables))
	for _, t := range tables {
		remaining[t] = map[string]bool{}
		for _, d := range deps[t] {
			if d != t && inSet[d] {
				remaining[t][d] = true
			}
		}
	}

	ordered := make([]string, 0, len(tables))
	placed := make(map[string]bool, len(tables))
	for len(ordered) < len(tables) {
		progressed := false
		for _, t := range tables {
			if placed[t] {
				continue
			}
			ready := true
			for d := range remaining[t] {
				if !placed[d] {
					ready = false
					break
				}
			}
			if ready {
				ordered = append(ordered, t)
				placed[t] = true
				progressed = true
			}
		}
		if !progressed {
			for _, t := range tables {
				if !placed[t] {
					ordered = append(ordered, t)
					placed[t] = true
				}
			}
		}
	}
	return ordered
}

// ServerVersion returns the backend's own version banner. SQLite reports
// its library version; the servers report theirs, SQL Server's cut to its first line.
func ServerVersion(ctx context.Context, database *sql.DB, driver Driver) (string, error) {
	var version string
	if err := database.QueryRowContext(ctx, serverVersionQuery(driver)).Scan(&version); err != nil {
		return "", fmt.Errorf("reading server version: %w", err)
	}
	if driver == DriverMSSQL {
		// @@VERSION is a multi-line banner; the copyright and edition lines after
		// the first are noise in a one-line status field.
		if line, _, found := strings.Cut(version, "\n"); found {
			version = line
		}
		version = strings.TrimSpace(version)
	}
	return version, nil
}

// serverVersionQuery is the per-driver statement ServerVersion runs. SQL Server
// has no version() function, and sqlite_version() is the fallthrough every
// unknown driver used to receive.
func serverVersionQuery(driver Driver) string {
	switch driver {
	case DriverPostgres:
		return "SELECT version()"
	case DriverMySQL:
		return "SELECT VERSION()"
	case DriverMSSQL:
		return "SELECT @@VERSION"
	default:
		return "SELECT sqlite_version()"
	}
}
