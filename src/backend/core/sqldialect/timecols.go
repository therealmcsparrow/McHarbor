// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package sqldialect

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// The application writes timestamps as RFC3339 text. SQLite and Postgres accept that
// for their timestamp columns; MySQL and MariaDB refuse the `T` and the zone suffix in
// strict mode, and in non-strict mode MariaDB silently drops the offset. So an RFC3339
// argument bound to a DATETIME column is rewritten to the UTC wall-clock form both
// accept. Only arguments whose target column is known to be a DATETIME are touched: an
// RFC3339 string going into a text column must stay byte-for-byte what was written.

// timeColumn is a DATETIME/TIMESTAMP/DATE column and its fractional-second precision.
type timeColumn struct {
	dateOnly  bool
	precision int
}

type timeRegistry map[string]map[string]timeColumn

var (
	mysqlTimeCols atomic.Pointer[timeRegistry]
	mysqlTimeDB   atomic.Pointer[sql.DB]
	refreshMu     sync.Mutex
	lastRefresh   time.Time
)

// placeholderMemo caches where each statement's placeholders point; the registry is
// consulted afresh on every bind, so a refresh never serves a stale answer.
var placeholderMemo sync.Map

const timeColumnsQuery = `SELECT LOWER(TABLE_NAME), LOWER(COLUMN_NAME), DATA_TYPE, COALESCE(DATETIME_PRECISION, 0)
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE() AND DATA_TYPE IN ('datetime', 'timestamp', 'date')`

// LoadMySQLTimeColumns learns which columns are DATETIME, TIMESTAMP or DATE from
// information_schema. Main calls it after migrations; MySQLRetry reloads it when a
// plugin migration adds such a column later.
func LoadMySQLTimeColumns(ctx context.Context, db *sql.DB) error {
	mysqlTimeDB.Store(db)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	rows, err := db.QueryContext(ctx, timeColumnsQuery)
	if err != nil {
		return fmt.Errorf("sqldialect: reading time columns: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing time column rows failed", "error", err)
		}
	}()
	reg := timeRegistry{}
	for rows.Next() {
		var table, column, kind string
		var precision int
		if err := rows.Scan(&table, &column, &kind, &precision); err != nil {
			return fmt.Errorf("sqldialect: scanning time column: %w", err)
		}
		if reg[table] == nil {
			reg[table] = map[string]timeColumn{}
		}
		reg[table][column] = timeColumn{dateOnly: strings.EqualFold(kind, "date"), precision: precision}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("sqldialect: reading time columns: %w", err)
	}
	mysqlTimeCols.Store(&reg)
	return nil
}

// setTimeRegistry installs a registry directly; tests use it in place of a server.
func setTimeRegistry(reg timeRegistry) {
	if reg == nil {
		mysqlTimeCols.Store(nil)
		return
	}
	mysqlTimeCols.Store(&reg)
}

// MySQLArgs rewrites each RFC3339 string argument whose placeholder targets a
// DATETIME, TIMESTAMP or DATE column; query is the statement as the application wrote it.
func MySQLArgs(query string, args []driver.NamedValue) {
	regp := mysqlTimeCols.Load()
	if regp == nil || len(args) == 0 {
		return
	}
	candidate := false
	for _, a := range args {
		if s, ok := a.Value.(string); ok && looksRFC3339(s) {
			candidate = true
			break
		}
	}
	if !candidate {
		return
	}
	refs := placeholderRefs(query)
	for i := range args {
		s, ok := args[i].Value.(string)
		if !ok || !looksRFC3339(s) {
			continue
		}
		idx := args[i].Ordinal - 1
		if idx < 0 || idx >= len(refs.cols) {
			continue
		}
		col, ok := refs.resolve(*regp, refs.cols[idx])
		if !ok {
			continue
		}
		if v, ok := toMySQLTime(s, col); ok {
			args[i].Value = v
		}
	}
}

// MySQLRetry reports whether a statement that failed should run once more: error
// 1292 on a datetime value means a column the registry has not seen, typically one a
// plugin migration just created, so the registry is reloaded first. Strict mode
// rejects the whole statement, so the retry cannot apply a write twice.
func MySQLRetry(err error) bool {
	if err == nil || !strings.Contains(err.Error(), "Error 1292") {
		return false
	}
	db := mysqlTimeDB.Load()
	if db == nil {
		return false
	}
	refreshMu.Lock()
	defer refreshMu.Unlock()
	if time.Since(lastRefresh) < time.Second {
		return false
	}
	lastRefresh = time.Now()
	return LoadMySQLTimeColumns(context.Background(), db) == nil
}

// looksRFC3339 is the cheap shape test run on every string argument.
func looksRFC3339(s string) bool {
	return len(s) >= 20 && len(s) <= 40 && s[4] == '-' && s[7] == '-' && s[10] == 'T' && s[13] == ':'
}

func toMySQLTime(s string, col timeColumn) (string, bool) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return "", false
	}
	t = t.UTC()
	if col.dateOnly {
		return t.Format("2006-01-02"), true
	}
	layout := "2006-01-02 15:04:05"
	if col.precision > 0 {
		layout += "." + strings.Repeat("0", min(col.precision, 6))
	}
	// Formatting with zeros truncates where MySQL would round, so a stored value
	// never lands in the next second relative to what the application wrote.
	return t.Format(layout), true
}

// colRef is where one placeholder points: a column, optionally qualified by a table
// name or alias. An empty column means the placeholder is not a plain column value.
type colRef struct {
	qualifier string
	column    string
}

type stmtRefs struct {
	cols    []colRef
	tables  []string
	aliases map[string]string
}

func (r stmtRefs) resolve(reg timeRegistry, ref colRef) (timeColumn, bool) {
	if ref.column == "" {
		return timeColumn{}, false
	}
	if ref.qualifier != "" {
		table := ref.qualifier
		if t, ok := r.aliases[table]; ok {
			table = t
		}
		col, ok := reg[table][ref.column]
		return col, ok
	}
	for _, t := range r.tables {
		if col, ok := reg[t][ref.column]; ok {
			return col, true
		}
	}
	return timeColumn{}, false
}

func placeholderRefs(q string) stmtRefs {
	if v, ok := placeholderMemo.Load(q); ok {
		return v.(stmtRefs)
	}
	r := analyzePlaceholders(q)
	placeholderMemo.Store(q, r)
	return r
}

// analyzePlaceholders maps every `?` to the column it is compared with, assigned to
// or inserted into. The shapes are INSERT column lists, `col op ?`, `col BETWEEN ? AND ?`
// and `col IN (?, ...)`; anything else maps to no column and is left alone.
func analyzePlaceholders(q string) stmtRefs {
	toks := lex(q)
	r := stmtRefs{aliases: map[string]string{}}
	collectTables(toks, &r)
	insertCols := insertColumnMap(toks)
	for i, t := range toks {
		if t.kind != tokParam {
			continue
		}
		ref, ok := insertCols[i]
		if !ok {
			ref = comparedColumn(toks, i)
		}
		r.cols = append(r.cols, ref)
	}
	return r
}

var notAlias = map[string]bool{
	"where": true, "join": true, "left": true, "right": true, "inner": true, "outer": true,
	"cross": true, "on": true, "set": true, "group": true, "order": true, "limit": true,
	"values": true, "select": true, "natural": true, "using": true, "having": true,
	"union": true, "full": true, "default": true, "returning": true, "for": true,
}

func collectTables(toks []token, r *stmtRefs) {
	for i := 0; i < len(toks); i++ {
		w := toks[i]
		if w.kind != tokWord || !tableIntroducers[w.text] || w.text == "table" || w.text == "references" {
			continue
		}
		for j := i + 1; j < len(toks); {
			name := toks[j]
			if !name.isName() || notAlias[name.text] && name.kind == tokWord {
				break
			}
			table := name.text
			j++
			if at(toks, j).is(".") && at(toks, j+1).isName() {
				table = toks[j+1].text
				j += 2
			}
			r.tables = append(r.tables, table)
			if at(toks, j).is("as") {
				j++
			}
			if a := at(toks, j); a.isName() && (a.kind != tokWord || (!notAlias[a.text] && !tableIntroducers[a.text])) {
				r.aliases[a.text] = table
				j++
			}
			if w.text != "from" || !at(toks, j).is(",") {
				break
			}
			j++
		}
	}
}

// insertColumnMap maps each placeholder that is a whole VALUES entry to its column.
func insertColumnMap(toks []token) map[int]colRef {
	out := map[int]colRef{}
	into := -1
	for i, t := range toks {
		if t.is("into") {
			into = i
			break
		}
		if t.is("select") || t.is("update") || t.is("delete") {
			return out
		}
	}
	if into < 0 || !at(toks, into+1).isName() {
		return out
	}
	j := into + 2
	if at(toks, j).is(".") {
		j += 2
	}
	if !at(toks, j).is("(") {
		return out
	}
	var cols []string
	j++
	for ; j < len(toks) && !toks[j].is(")"); j++ {
		if toks[j].isName() {
			cols = append(cols, toks[j].text)
		}
	}
	j++
	if at(toks, j).is("select") {
		return insertSelectMap(toks, j, cols, out)
	}
	if !at(toks, j).is("values") {
		return out
	}
	depth, pos := 0, 0
	for j++; j < len(toks); j++ {
		t := toks[j]
		switch {
		case t.is("("):
			depth++
			if depth == 1 {
				pos = 0
			}
		case t.is(")"):
			depth--
		case t.is(",") && depth == 1:
			pos++
		case t.kind == tokParam && depth == 1:
			whole := (at(toks, j-1).is("(") || at(toks, j-1).is(",")) && (at(toks, j+1).is(")") || at(toks, j+1).is(","))
			if whole && pos < len(cols) {
				out[j] = colRef{column: cols[pos]}
			}
		case depth == 0 && t.kind == tokWord:
			return out
		}
	}
	return out
}

// insertSelectMap maps `INSERT INTO t (cols) SELECT ?, ?, … [FROM|WHERE …]` by position.
func insertSelectMap(toks []token, sel int, cols []string, out map[int]colRef) map[int]colRef {
	depth, pos := 0, 0
	for j := sel + 1; j < len(toks); j++ {
		t := toks[j]
		switch {
		case t.is("("):
			depth++
		case t.is(")"):
			depth--
		case depth == 0 && t.is(","):
			pos++
		case depth == 0 && (t.is("from") || t.is("where") || t.is("union")):
			return out
		case depth == 0 && t.kind == tokParam:
			prev, next := at(toks, j-1), at(toks, j+1)
			whole := (prev.is("select") || prev.is(",")) && (next.start < 0 || next.is(",") || next.is("from") || next.is("where"))
			if whole && pos < len(cols) {
				out[j] = colRef{column: cols[pos]}
			}
		}
	}
	return out
}

var comparisonOps = map[string]bool{"=": true, "==": true, "<": true, ">": true, "<=": true, ">=": true, "<>": true, "!=": true}

// comparedColumn finds the column a placeholder is compared with or assigned to.
func comparedColumn(toks []token, i int) colRef {
	prev := at(toks, i-1)
	switch {
	case prev.kind == tokPunct && comparisonOps[prev.text], prev.is("is"):
		return columnEndingAt(toks, i-2)
	case prev.is("between"):
		return columnEndingAt(toks, i-2)
	case prev.is("and") && at(toks, i-2).kind == tokParam && at(toks, i-3).is("between"):
		return columnEndingAt(toks, i-4)
	}
	// An IN list: walk back over `?` and `,` to the `(` and check for IN before it.
	j := i - 1
	for j >= 0 && (toks[j].kind == tokParam || toks[j].is(",")) {
		j--
	}
	if at(toks, j).is("(") && at(toks, j-1).is("in") {
		k := j - 2
		if at(toks, k).is("not") {
			k--
		}
		return columnEndingAt(toks, k)
	}
	next := at(toks, i+1)
	if next.kind == tokPunct && comparisonOps[next.text] {
		return columnStartingAt(toks, i+2)
	}
	return colRef{}
}

func columnEndingAt(toks []token, i int) colRef {
	t := at(toks, i)
	if !t.isName() || (t.kind == tokWord && isSQLWord(t.text)) {
		return colRef{}
	}
	if at(toks, i-1).is(".") && at(toks, i-2).isName() {
		return colRef{qualifier: toks[i-2].text, column: t.text}
	}
	return colRef{column: t.text}
}

func columnStartingAt(toks []token, i int) colRef {
	t := at(toks, i)
	if !t.isName() || at(toks, i+1).is("(") {
		return colRef{}
	}
	if at(toks, i+1).is(".") && at(toks, i+2).isName() {
		return colRef{qualifier: t.text, column: toks[i+2].text}
	}
	return colRef{column: t.text}
}

// isSQLWord keeps an expression's closing keyword from being read as a column name.
func isSQLWord(w string) bool {
	switch w {
	case "null", "end", "then", "else", "and", "or", "not", "select", "where", "case":
		return true
	}
	return false
}
