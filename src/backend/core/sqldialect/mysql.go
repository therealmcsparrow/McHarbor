// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package sqldialect

import (
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// mysqlReserved lists the names the schema uses as identifiers that MySQL 8.4 or
// MariaDB 11 reserve, found by creating and querying a column of every table and
// column name on both servers. A name missing here fails loudly with error 1064.
var mysqlReserved = map[string]bool{
	"condition": true, "generated": true, "groups": true, "key": true, "release": true,
	"system": true, "trigger": true, "order": true, "interval": true, "read": true,
	"rank": true, "usage": true, "long": true,
}

// identifierFollowers are the tokens that may follow a column or table reference, and
// never follow INTERVAL the keyword, which is always followed by its operand.
var identifierFollowers = map[string]bool{
	",": true, ")": true, "=": true, "<": true, ">": true, "<=": true, ">=": true, "<>": true,
	"!=": true, ";": true, "||": true, ".": true,
	"as": true, "from": true, "is": true, "in": true, "like": true, "not": true, "between": true,
	"asc": true, "desc": true, "and": true, "or": true, "where": true, "set": true, "on": true,
	"join": true, "left": true, "inner": true, "order": true, "group": true, "limit": true,
	"having": true, "then": true, "else": true, "end": true, "when": true, "values": true,
	"union": true, "collate": true, "glob": true, "regexp": true,
}

// tableIntroducers are the words after which a name followed by `(` is a table with
// a column list rather than a function call.
var tableIntroducers = map[string]bool{"into": true, "table": true, "join": true, "from": true, "update": true, "references": true}

// isDML reports a statement the runtime quoter may touch. DDL is left to the
// migration translator, whose positional rules know CREATE TABLE's grammar.
func isDML(toks []token) bool {
	if len(toks) == 0 || toks[0].kind != tokWord {
		return false
	}
	switch toks[0].text {
	case "select", "insert", "update", "delete", "replace", "with":
		return true
	}
	return false
}

// reservedAsIdentifier decides whether the reserved word at i names something. The
// keyword readings the application uses are ORDER BY, PRIMARY/UNIQUE/FOREIGN KEY,
// ON DUPLICATE KEY, RANK() and INTERVAL <n> <unit>; every other use is a name.
func reservedAsIdentifier(toks []token, i int) bool {
	prev, next := at(toks, i-1), at(toks, i+1)
	if prev.is(".") {
		return true
	}
	switch toks[i].text {
	case "order":
		return !next.is("by")
	case "key":
		return !prev.is("primary") && !prev.is("unique") && !prev.is("foreign") && !prev.is("duplicate")
	case "interval":
		return next.start < 0 || identifierFollowers[next.text]
	}
	if next.is("(") {
		return tableIntroducers[prev.text] && prev.kind == tokWord
	}
	return true
}

// quoteReservedMySQL backticks the reserved words a DML statement uses as names.
// Literals, comments and already-quoted names are never touched, so running it
// twice changes nothing.
func quoteReservedMySQL(q string) string {
	if !containsReserved(q) {
		return q
	}
	toks := lex(q)
	if !isDML(toks) {
		return q
	}
	var b strings.Builder
	last := 0
	for i, t := range toks {
		if t.kind != tokWord || !mysqlReserved[t.text] || !reservedAsIdentifier(toks, i) {
			continue
		}
		b.WriteString(q[last:t.start])
		b.WriteByte('`')
		b.WriteString(q[t.start:t.end])
		b.WriteByte('`')
		last = t.end
	}
	if last == 0 {
		return q
	}
	b.WriteString(q[last:])
	return b.String()
}

// containsReserved is a cheap pre-check so the common statement skips the lexer.
func containsReserved(q string) bool {
	lower := strings.ToLower(q)
	for w := range mysqlReserved {
		if strings.Contains(lower, w) {
			return true
		}
	}
	return false
}

// MySQLDSN pins the per-connection session settings the application depends on.
// They go in the DSN because go-sql-driver applies DSN system variables on every
// pooled connection, where a one-off SET reaches only the connection that ran it:
// parseTime and loc=UTC make DATETIME columns scan into RFC3339 strings, time_zone
// keeps UTC_TIMESTAMP and CURRENT_TIMESTAMP in UTC, NO_BACKSLASH_ESCAPES gives
// string literals SQLite's meaning, PIPES_AS_CONCAT makes || concatenate as it does
// in SQLite, and utf8mb4 carries every character.
func MySQLDSN(dsn string) string {
	base, query, _ := strings.Cut(dsn, "?")
	params, err := url.ParseQuery(query)
	if err != nil {
		return dsn
	}
	set := func(k, v string, force bool) {
		if force || params.Get(k) == "" {
			params.Set(k, v)
		}
	}
	set("parseTime", "true", true)
	set("loc", "UTC", false)
	set("charset", "utf8mb4", false)
	set("time_zone", "'+00:00'", false)
	set("sql_mode", "CONCAT(@@sql_mode,',NO_BACKSLASH_ESCAPES,PIPES_AS_CONCAT')", false)
	return base + "?" + params.Encode()
}

// mysqlUpsert turns SQLite/Postgres `ON CONFLICT … DO UPDATE SET a = excluded.a` into
// `ON DUPLICATE KEY UPDATE a = VALUES(a)`, and `ON CONFLICT … DO NOTHING` into a no-op
// assignment of the first inserted column. MySQL resolves on whichever unique key
// collides rather than the named one; every conflict target here is such a key.
func mysqlUpsert(q string) string {
	lower := strings.ToLower(q)
	if !strings.Contains(lower, "conflict") {
		return q
	}
	toks := lex(q)
	on := -1
	for i := 0; i+1 < len(toks); i++ {
		if toks[i].is("on") && toks[i+1].is("conflict") {
			on = i
			break
		}
	}
	if on < 0 {
		return q
	}
	j := on + 2
	if at(toks, j).is("(") {
		for j < len(toks) && !toks[j].is(")") {
			j++
		}
		j++
	}
	if !at(toks, j).is("do") {
		return q
	}
	head := strings.TrimRight(q[:toks[on].start], " \t\r\n")
	switch {
	case at(toks, j+1).is("nothing"):
		col := firstInsertColumn(toks)
		if col == "" {
			return q
		}
		tail := ""
		if k := j + 2; k < len(toks) {
			tail = " " + q[toks[k].start:]
		}
		return head + " ON DUPLICATE KEY UPDATE " + col + " = " + col + tail
	case at(toks, j+1).is("update") && at(toks, j+2).is("set"):
		k := j + 3
		if k >= len(toks) {
			return q
		}
		var b strings.Builder
		b.WriteString(head)
		b.WriteString(" ON DUPLICATE KEY UPDATE ")
		last := toks[k].start
		for i := k; i < len(toks); i++ {
			if toks[i].is("excluded") && at(toks, i+1).is(".") && at(toks, i+2).isName() {
				b.WriteString(q[last:toks[i].start])
				b.WriteString("VALUES(")
				b.WriteString(q[toks[i+2].start:toks[i+2].end])
				b.WriteString(")")
				last = toks[i+2].end
				i += 2
			}
		}
		b.WriteString(q[last:])
		return b.String()
	}
	return q
}

// firstInsertColumn is the first name in an INSERT's column list, as written.
func firstInsertColumn(toks []token) string {
	for i := 0; i+2 < len(toks); i++ {
		if toks[i].is("into") {
			j := i + 2
			if at(toks, j).is(".") {
				j += 2
			}
			if at(toks, j).is("(") && at(toks, j+1).isName() {
				return toks[j+1].text
			}
			return ""
		}
	}
	return ""
}

// edit replaces q[start:end]; edits are applied back to front so spans stay valid.
type edit struct {
	start, end int
	text       string
}

func applyEdits(q string, edits []edit) string {
	if len(edits) == 0 {
		return q
	}
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	for _, e := range edits {
		q = q[:e.start] + e.text + q[e.end:]
	}
	return q
}

var mysqlCastTypes = map[string]string{"integer": "SIGNED", "int": "SIGNED", "bigint": "SIGNED", "text": "CHAR", "real": "DOUBLE", "float": "DOUBLE"}

// matchingOpen returns the index of the `(` that the `)` at i closes, or -1.
func matchingOpen(toks []token, i int) int {
	depth := 0
	for j := i; j >= 0; j-- {
		switch {
		case toks[j].is(")"):
			depth++
		case toks[j].is("("):
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return -1
}

// matchingClose returns the index of the `)` that closes the `(` at i, or -1.
func matchingClose(toks []token, i int) int {
	depth := 0
	for j := i; j < len(toks); j++ {
		switch {
		case toks[j].is("("):
			depth++
		case toks[j].is(")"):
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return -1
}

// mysqlStatementRules rewrites the SQLite shapes MySQL and MariaDB spell differently:
// `x IS ?` (null-safe equality) becomes `x <=> ?`; CAST to SQLite type names uses
// MySQL's; a FROM (subquery) gets the alias MySQL requires; and an IN (subquery) that
// has a LIMIT, or sits in a DELETE/UPDATE that may read its own target table, is
// wrapped in a derived table, the form MySQL accepts for both.
func mysqlStatementRules(q string) string {
	lower := strings.ToLower(q)
	if !strings.Contains(lower, " is ") && !strings.Contains(lower, "cast") && !strings.Contains(lower, "(") {
		return q
	}
	toks := lex(q)
	if !isDML(toks) {
		return q
	}
	var edits []edit
	alias := 0
	mutates := toks[0].is("delete") || toks[0].is("update")
	for i, t := range toks {
		switch {
		case t.is("is") && at(toks, i+1).kind == tokParam:
			edits = append(edits, edit{t.start, t.end, "<=>"})
		case t.is("as") && at(toks, i+2).is(")") && mysqlCastTypes[at(toks, i+1).text] != "" && at(toks, i+1).kind == tokWord:
			if open := matchingOpen(toks, i+2); open > 0 && toks[open-1].is("cast") {
				edits = append(edits, edit{toks[i+1].start, toks[i+1].end, mysqlCastTypes[toks[i+1].text]})
			}
		case t.is("(") && at(toks, i+1).is("select") && (at(toks, i-1).is("from") || at(toks, i-1).is("join")):
			end := matchingClose(toks, i)
			if end < 0 {
				continue
			}
			next := at(toks, end+1)
			if next.is("as") || next.isName() && (next.kind != tokWord || !notAlias[next.text]) {
				continue
			}
			alias++
			edits = append(edits, edit{toks[end].end, toks[end].end, " AS mch_d" + strconv.Itoa(alias)})
		case t.is("(") && at(toks, i+1).is("select") && at(toks, i-1).is("in"):
			end := matchingClose(toks, i)
			if end < 0 || (!mutates && !hasTopLevelLimit(toks, i+1, end)) {
				continue
			}
			alias++
			edits = append(edits,
				edit{toks[i+1].start, toks[i+1].start, "SELECT * FROM ("},
				edit{toks[end].start, toks[end].start, ") AS mch_d" + strconv.Itoa(alias)})
		}
	}
	return applyEdits(q, edits)
}

// hasTopLevelLimit reports a LIMIT between from and to that is not inside a nested
// parenthesis, i.e. one that belongs to the subquery itself.
func hasTopLevelLimit(toks []token, from, to int) bool {
	depth := 0
	for j := from; j < to; j++ {
		switch {
		case toks[j].is("("):
			depth++
		case toks[j].is(")"):
			depth--
		case depth == 0 && toks[j].is("limit"):
			return true
		}
	}
	return false
}

var distinctConcatRE = regexp.MustCompile(`(?is)GROUP_CONCAT\(\s*([a-z_][a-z0-9_]*)\s+SEPARATOR\s+('[^']*')\s*\)\s+FROM\s+\(\s*SELECT\s+DISTINCT\s+([^()]+?)\s+AS\s+([a-z_][a-z0-9_]*)\s+FROM\s+([^()]+?)\s*\)(\s+AS\s+[a-z_][a-z0-9_]*)?`)

// flattenDistinctConcat turns `GROUP_CONCAT(n SEPARATOR s) FROM (SELECT DISTINCT e AS n
// FROM …)` into `GROUP_CONCAT(DISTINCT e SEPARATOR s) FROM …`. SQLite has no DISTINCT
// with a separator, so the application dedups in a derived table, but MariaDB cannot
// see the outer query's columns from inside one, which the correlated form needs.
func flattenDistinctConcat(q string) string {
	if !strings.Contains(strings.ToLower(q), "distinct") {
		return q
	}
	return distinctConcatRE.ReplaceAllStringFunc(q, func(m string) string {
		g := distinctConcatRE.FindStringSubmatch(m)
		if !strings.EqualFold(g[1], g[4]) {
			return m
		}
		return "GROUP_CONCAT(DISTINCT " + g[3] + " SEPARATOR " + g[2] + ") FROM " + g[5]
	})
}
