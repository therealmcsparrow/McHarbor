// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"regexp"
	"strconv"
	"strings"
)

// Role permissions are stored as a JSON array in a text column, and the
// migrations that grant new keys to the seeded roles and presets manipulate
// that array in SQL, with SQLite's JSON functions.
//
// An earlier version of this file matched the two *templates* those
// migrations use. That was wrong: 067_rbac.sql also uses `json_each` inside
// correlated `EXISTS` filters and conditional `UNION` branches, and a
// template matcher either misses those or mangles them.
//
// What is translated here instead are the three **primitives**, which covers
// every shape uniformly — including shapes nobody has written yet:
//
//	json_each(x)           a table function yielding a `value` column
//	json_group_array(x)    an aggregate producing a JSON array
//	json_array(a, b, …)    a JSON array literal
//
// Everything else — the UNIONs, the EXISTS, the LIKE filters — is ordinary
// SQL that all three engines already understand.

var (
	// jsonEachRE matches `json_each(<expr>)`. The argument may itself be a
	// call, so the matcher balances parentheses rather than stopping at the
	// first `)`.
	jsonEachRE = regexp.MustCompile(`(?i)\bjson_each\s*\(`)
	// jsonGroupArrayRE and jsonArrayRE are matched the same way.
	jsonGroupArrayRE = regexp.MustCompile(`(?i)\bjson_group_array\s*\(`)
	jsonArrayRE      = regexp.MustCompile(`(?i)\bjson_array\s*\(`)
	// jsonObjectRE matches SQLite's json_object(k, v, …). Both drivers have
	// the same function under a different name.
	jsonObjectRE = regexp.MustCompile(`(?i)\bjson_object\s*\(`)
	// jsonInsertRE matches `json_insert(col, '$[#]', 'a', ...)` - SQLite's
	// append-to-array form, used by the early single-category permission
	// migrations. Those are guarded by a `NOT LIKE`, which is what keeps them
	// idempotent, so an append is the faithful translation on every driver.
	jsonInsertRE = regexp.MustCompile(`(?i)\bjson_insert\s*\(`)
	// jsonAppendPairRE pulls each `'$[#]', 'value'` pair out of that list.
	jsonAppendPairRE = regexp.MustCompile(`'\$\[#\]'\s*,\s*'([^']*)'`)

	// strftimeEpochRE is the one three-argument strftime in the corpus:
	// `strftime('<fmt>', <expr>, 'unixepoch')`, used by 077_observability_history.sql
	// to render a nanosecond integer as RFC3339.
	strftimeEpochRE = regexp.MustCompile(`(?is)strftime\(\s*'([^']*)'\s*,\s*(.+?)\s*,\s*'unixepoch'\s*\)`)
)

// rewriteCall finds every `name(` produced by re and replaces the whole call —
// argument list included — with what build returns for that argument.
//
// Parenthesis-balanced rather than regexp-bounded, because the arguments are
// frequently themselves calls: `json_each(json_array('a','b'))`.
func rewriteCall(stmt string, re *regexp.Regexp, build func(arg string) string) string {
	loc := re.FindStringIndex(stmt)
	if loc == nil {
		return stmt
	}
	argStart := loc[1] // just past the opening paren
	argEnd, ok := matchParen(stmt, argStart)
	if !ok {
		return stmt
	}
	replacement := build(strings.TrimSpace(stmt[argStart:argEnd]))
	head := stmt[:loc[0]]
	tail := stmt[argEnd+1:]
	// Recurse on the tail only: the replacement is already translated, and
	// re-scanning it could rewrite a function name the replacement itself
	// introduced.
	return head + replacement + rewriteCall(tail, re, build)
}

// matchParen returns the index of the `)` closing the paren that opened just
// before start, honouring nesting and string literals.
func matchParen(s string, start int) (int, bool) {
	depth := 1
	inQuoted := false
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '\'':
			if inQuoted && i+1 < len(s) && s[i+1] == '\'' {
				i++
				continue
			}
			inQuoted = !inQuoted
		case '(':
			if !inQuoted {
				depth++
			}
		case ')':
			if inQuoted {
				continue
			}
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

// translateJSONPostgres rewrites the three primitives into Postgres.
func translateJSONPostgres(stmt string) string {
	out := stmt

	// json_object(k, v, …) builds an object; Postgres spells it
	// json_build_object, and the column it lands in is text.
	out = rewriteCall(out, jsonObjectRE, func(arg string) string {
		return "json_build_object(" + arg + ")::text"
	})

	// json_insert first: its argument list contains the values that the
	// json_array rewrite would otherwise not see.
	out = rewriteCall(out, jsonInsertRE, func(arg string) string {
		column, values := splitJSONInsert(arg)
		if column == "" || len(values) == 0 {
			return "json_insert(" + arg + ")"
		}
		return "((" + column + ")::jsonb || '[" +
			strings.Join(values, ",") + "]'::jsonb)::text"
	})

	// json_array('a','b') is a JSON array literal. Rendering it as a quoted
	// text literal lets the json_each rewrite treat it exactly like a column.
	out = rewriteCall(out, jsonArrayRE, func(arg string) string {
		return "'[" + strings.Join(sqlQuotedValuesAsJSON(arg), ",") + "]'"
	})

	// `FROM json_each(x)` yields rows with a `value` column; Postgres's
	// equivalent is a set-returning function aliased to the same name.
	out = rewriteCall(out, jsonEachRE, func(arg string) string {
		return "jsonb_array_elements_text((" + arg + ")::jsonb) AS value"
	})

	out = rewriteCall(out, jsonGroupArrayRE, func(arg string) string {
		return "json_agg(" + arg + ")::text"
	})

	out = strftimeEpochRE.ReplaceAllStringFunc(out, func(match string) string {
		groups := strftimeEpochRE.FindStringSubmatch(match)
		if len(groups) < 3 {
			return match
		}
		// The argument is arithmetic on a TEXT column, but the cast is already
		// applied by translatePostgres's textDivisionRE before this runs —
		// casting again here produced `(ts)::(numeric)::numeric`, because
		// `numeric` is itself a valid left operand for the same pattern.
		return "to_char(to_timestamp(" + groups[2] + "), '" +
			postgresTimeFormat(groups[1]) + "')"
	})

	return out
}

// jsonUnionTemplateRE matches the set-union shape the permission migrations
// use most: take a role's JSON array, union it with a literal list, and write
// the result back.
//
// It is matched as a whole — rather than through the primitives — because the
// primitive translation needs JSON_TABLE, and MariaDB rejects a correlated
// outer column reference inside JSON_TABLE with "Unknown column
// 'roles.permissions' in 'JSON_TABLE'". JSON_MERGE_PRESERVE expresses the
// same intent with no derived table and no correlation problem.
var jsonUnionTemplateRE = regexp.MustCompile(`(?is)SELECT\s+json_group_array\(\s*value\s*\)\s+FROM\s*\(\s*SELECT\s+value\s+FROM\s+json_each\(\s*([a-z_][a-z0-9_.]*)\s*\)\s+UNION\s+SELECT\s+value\s+FROM\s+json_each\(\s*json_array\((.*?)\)\s*\)\s*\)`)

// translateJSONMySQL rewrites the primitives into MySQL / MariaDB.
func translateJSONMySQL(stmt string) string {
	out := stmt

	// The whole-template rewrite runs first; whatever it matches never reaches
	// the primitive path and so never needs JSON_TABLE.
	out = jsonUnionTemplateRE.ReplaceAllStringFunc(out, func(match string) string {
		groups := jsonUnionTemplateRE.FindStringSubmatch(match)
		if len(groups) < 3 {
			return match
		}
		values := sqlQuotedValues(groups[2])
		if len(values) == 0 {
			return match
		}
		// JSON_MERGE_PRESERVE concatenates without de-duplicating, so a key
		// the role already held appears twice. That is a deliberate, bounded
		// difference: every reader of this column treats it as a set —
		// hasPermission is a membership test — so a duplicate changes no
		// decision, and a migration runs at most once because
		// schema_migrations records it.
		return "SELECT JSON_MERGE_PRESERVE(COALESCE(NULLIF(" + groups[1] +
			", ''), '[]'), JSON_ARRAY(" + strings.Join(values, ",") + "))"
	})

	// JSON_OBJECT is native and spelled the same, so only the case differs;
	// rewriting it explicitly keeps the audit test's "no SQLite-only
	// construct survives" sweep honest.
	out = rewriteCall(out, jsonObjectRE, func(arg string) string {
		return "JSON_OBJECT(" + arg + ")"
	})

	// JSON_ARRAY_APPEND is the direct equivalent of json_insert's append form:
	// same (doc, path, value) shape, and '$' on an array appends exactly as
	// '$[#]' does.
	out = rewriteCall(out, jsonInsertRE, func(arg string) string {
		column, values := splitJSONInsert(arg)
		if column == "" || len(values) == 0 {
			return "json_insert(" + arg + ")"
		}
		parts := make([]string, 0, len(values))
		for _, v := range values {
			parts = append(parts, "'$', "+strings.ReplaceAll(v, `"`, "'"))
		}
		return "JSON_ARRAY_APPEND(" + column + ", " + strings.Join(parts, ", ") + ")"
	})

	// JSON_ARRAY is native and needs no rewrite; the others do.
	out = rewriteCall(out, jsonEachRE, func(arg string) string {
		// JSON_TABLE is the only lateral JSON expansion both MySQL 8 and
		// MariaDB 10.6 have. The alias makes `value` resolvable exactly as the
		// SQLite original expects, and the generated table alias is unique per
		// call so two json_each() in one statement do not collide.
		// An empty string is not valid JSON, and several of these columns
		// default to ''. COALESCE/NULLIF turns that into an empty array.
		return "JSON_TABLE(COALESCE(NULLIF(" + arg + ", ''), '[]'), '$[*]' COLUMNS (value VARCHAR(" +
			strconv.Itoa(keyedTextLength) + ") PATH '$')) AS " + nextJSONTableAlias()
	})

	out = rewriteCall(out, jsonGroupArrayRE, func(arg string) string {
		return "JSON_ARRAYAGG(" + arg + ")"
	})

	out = strftimeEpochRE.ReplaceAllStringFunc(out, func(match string) string {
		groups := strftimeEpochRE.FindStringSubmatch(match)
		if len(groups) < 3 {
			return match
		}
		return "DATE_FORMAT(FROM_UNIXTIME(" + groups[2] + "), '" +
			mysqlTimeFormat(groups[1]) + "')"
	})

	// MySQL refuses to read the table an UPDATE targets from that UPDATE's own
	// subquery, so the subquery is pushed one level down first.
	out = wrapSelfReferencingSubqueries(out)

	// Last, because the JSON rewrites above and the wrap just before introduce
	// derived tables of their own that need naming too.
	out = aliasDerivedTables(out)

	return out
}

// jsonTableAliasCounter numbers the JSON_TABLE aliases within one process.
// MySQL requires every derived table to be aliased and rejects two with the
// same name in one query.
var jsonTableAliasCounter int

func nextJSONTableAlias() string {
	jsonTableAliasCounter++
	return "jt" + strconv.Itoa(jsonTableAliasCounter)
}

// sqlQuotedValuesAsJSON turns a SQL literal list (`'a', 'b'`) into JSON
// string literals, dropping comments and whitespace between entries.
func sqlQuotedValuesAsJSON(list string) []string {
	var out []string
	for _, m := range regexp.MustCompile(`'([^']*)'`).FindAllStringSubmatch(list, -1) {
		out = append(out, `"`+strings.ReplaceAll(m[1], `"`, `\"`)+`"`)
	}
	return out
}

// The two format translators cover the strftime specifiers this corpus uses.
// Anything else is left as-is and will fail loudly rather than silently
// produce a wrong timestamp.
var (
	postgresTimeReplacer = strings.NewReplacer(
		"%Y", "YYYY", "%m", "MM", "%d", "DD",
		"%H", "HH24", "%M", "MI", "%S", "SS",
		"T", `"T"`, "Z", `"Z"`,
	)
	mysqlTimeReplacer = strings.NewReplacer(
		"%Y", "%Y", "%m", "%m", "%d", "%d",
		"%H", "%H", "%M", "%i", "%S", "%s",
	)
)

func postgresTimeFormat(sqliteFormat string) string {
	return postgresTimeReplacer.Replace(sqliteFormat)
}

func mysqlTimeFormat(sqliteFormat string) string {
	return mysqlTimeReplacer.Replace(sqliteFormat)
}

// splitJSONInsert takes a json_insert argument list and returns the column
// being appended to plus the appended values, already rendered as JSON string
// literals.
func splitJSONInsert(arg string) (string, []string) {
	comma := strings.Index(arg, ",")
	if comma < 0 {
		return "", nil
	}
	column := strings.TrimSpace(arg[:comma])
	var values []string
	for _, m := range jsonAppendPairRE.FindAllStringSubmatch(arg[comma:], -1) {
		values = append(values, `"`+strings.ReplaceAll(m[1], `"`, `\"`)+`"`)
	}
	return column, values
}

// derivedTableRE finds a `FROM (` that opens a derived table.
var derivedTableRE = regexp.MustCompile(`(?i)\bFROM\s*\(`)

// aliasDerivedTables gives every unaliased derived table a name.
//
// SQLite allows `SELECT … FROM (SELECT …)`; MySQL and MariaDB both reject it
// with "Every derived table must have its own alias". The permission
// migrations nest two levels of subquery to de-duplicate a JSON array, so this
// is not a rare shape — and the alias has to be unique within the statement,
// not just present.
func aliasDerivedTables(stmt string) string {
	var out strings.Builder
	rest := stmt
	counter := 0

	for {
		loc := derivedTableRE.FindStringIndex(rest)
		if loc == nil {
			out.WriteString(rest)
			return out.String()
		}

		closing, ok := matchParen(rest, loc[1])
		if !ok {
			out.WriteString(rest)
			return out.String()
		}

		inner := rest[loc[1]:closing]
		// A parenthesised expression is not a derived table — only a SELECT is.
		if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(inner)), "SELECT") {
			out.WriteString(rest[:closing+1])
			rest = rest[closing+1:]
			continue
		}

		out.WriteString(rest[:loc[1]])
		out.WriteString(aliasDerivedTables(inner))
		out.WriteString(")")

		tail := rest[closing+1:]
		if !hasAlias(tail) {
			counter++
			out.WriteString(" AS d" + strconv.Itoa(counter))
		}
		rest = tail
	}
}

// hasAlias reports whether what follows a derived table's closing paren
// already names it.
func hasAlias(tail string) bool {
	trimmed := strings.TrimLeft(tail, " \t\r\n")
	upper := strings.ToUpper(trimmed)
	if strings.HasPrefix(upper, "AS ") || strings.HasPrefix(upper, "AS\n") {
		return true
	}
	// A bare identifier immediately after the paren is also an alias, but a
	// keyword is not.
	word := trimmed
	if i := strings.IndexAny(word, " \t\r\n,;)"); i >= 0 {
		word = word[:i]
	}
	if word == "" {
		return false
	}
	switch strings.ToUpper(word) {
	case "UNION", "WHERE", "ORDER", "GROUP", "LIMIT", "ON", "JOIN", "SET",
		"HAVING", "AND", "OR", "THEN", "ELSE", "END", "WHEN":
		return false
	}
	return regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`).MatchString(word)
}

// sqlQuotedValues splits a SQL literal list (`'a', 'b'`), preserving the
// quotes and dropping comments and whitespace between entries.
func sqlQuotedValues(list string) []string {
	var out []string
	for _, m := range regexp.MustCompile(`'([^']*)'`).FindAllStringSubmatch(list, -1) {
		out = append(out, "'"+m[1]+"'")
	}
	return out
}

// MySQLCanExpress reports whether a statement survives translation into
// something MySQL and MariaDB will both accept.
//
// The one shape that does not is a correlated JSON_TABLE: MariaDB rejects an
// outer column reference inside it, and there is no rewrite that preserves the
// semantics. Only 067_rbac.sql's conditional key-rename statements reach it —
// they map `container_backups.*` onto `backups.*` / `restore.*` on an
// install upgrading from an older release, and are a no-op on a fresh one,
// which is the only kind of install these drivers can currently have.
func MySQLCanExpress(stmt string) bool {
	return !strings.Contains(translateJSONMySQL(stmt), "JSON_TABLE(")
}

// updateTargetRE names the table an UPDATE writes to.
var updateTargetRE = regexp.MustCompile(`(?is)^\s*UPDATE\s+"?([a-z_][a-z0-9_]*)"?\s`)

// selfReferencingSubqueryRE finds a `FROM <table>` inside a statement.
func selfReferencingSubqueryRE(table string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)\bFROM\s+` + table + `\b`)
}

// wrapSelfReferencingSubqueries works around MySQL error 1093, "You can't
// specify target table 'x' for update in FROM clause".
//
// MySQL refuses to read the table an UPDATE is writing to from that UPDATE's
// own subquery; SQLite and Postgres both allow it. The documented workaround
// is to put one more derived table in the way, which forces MySQL to
// materialise the inner result before the update begins — which is the
// semantics SQLite had all along.
func wrapSelfReferencingSubqueries(stmt string) string {
	m := updateTargetRE.FindStringSubmatch(stripComments(stmt))
	if m == nil {
		return stmt
	}
	table := m[1]
	if !selfReferencingSubqueryRE(table).MatchString(stmt) {
		return stmt
	}

	// Find each `( SELECT … )` that mentions the target table and wrap it.
	var out strings.Builder
	rest := stmt
	counter := 0
	open := regexp.MustCompile(`\(\s*(?is:SELECT)`)

	for {
		loc := open.FindStringIndex(rest)
		if loc == nil {
			out.WriteString(rest)
			return out.String()
		}
		closing, ok := matchParen(rest, loc[0]+1)
		if !ok {
			out.WriteString(rest)
			return out.String()
		}
		inner := rest[loc[0]+1 : closing]
		out.WriteString(rest[:loc[0]])
		if selfReferencingSubqueryRE(table).MatchString(inner) {
			counter++
			out.WriteString("(SELECT * FROM (" + inner + ") AS mch_deref" +
				strconv.Itoa(counter) + ")")
		} else {
			out.WriteString("(" + inner + ")")
		}
		rest = rest[closing+1:]
	}
}
