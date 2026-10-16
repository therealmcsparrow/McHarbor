// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"strconv"
	"strings"
)

// The SQL Server half of the JSON primitive translation. It follows
// dialect_json.go's approach — translate the four primitives rather than the
// templates they appear in — and differs from the other two drivers in one
// structural way: SQL Server has no JSON type. Every JSON value here is
// NVARCHAR holding JSON text, which is exactly how the SQLite schema already
// stores it, so nothing needs casting in either direction.
//
// OPENJSON is the equivalent of json_each and, unlike MariaDB's JSON_TABLE, it
// accepts a correlated outer column reference — so the whole-template
// workaround translateJSONMySQL needs has no counterpart here.

// mssqlTimeReplacer maps SQLite's strftime directives onto the CONVERT style
// SQL Server uses. The corpus's only three-argument strftime renders RFC3339,
// which style 126 produces directly, so the map exists for the general case and
// the caller special-cases the one format it actually sees.
var mssqlTimeReplacer = strings.NewReplacer(
	"%Y", "yyyy", "%m", "MM", "%d", "dd",
	"%H", "HH", "%M", "mm", "%S", "ss", "%f", "fff",
)

// mssqlTimeFormat renders a SQLite strftime format as a T-SQL FORMAT pattern.
func mssqlTimeFormat(sqliteFormat string) string {
	return mssqlTimeReplacer.Replace(sqliteFormat)
}

// translateJSONMSSQL rewrites the JSON primitives and the epoch strftime into
// T-SQL.
func translateJSONMSSQL(stmt string) string {
	out := stmt

	// There is no json_object. Building the object by hand would mean parsing
	// the key/value list; JSON_MODIFY over an empty object does it in SQL and
	// keeps the result NVARCHAR, which is what the column is.
	out = rewriteCall(out, jsonObjectRE, func(arg string) string {
		pairs := splitTopLevelArgs(arg)
		if len(pairs) == 0 || len(pairs)%2 != 0 {
			// An odd list is not a json_object at all; leave it so the server
			// refuses it rather than emitting something that parses.
			return "json_object(" + arg + ")"
		}
		expr := "'{}'"
		for i := 0; i < len(pairs); i += 2 {
			key := strings.Trim(strings.TrimSpace(pairs[i]), "'")
			expr = "JSON_MODIFY(" + expr + ", '$." + key + "', " + strings.TrimSpace(pairs[i+1]) + ")"
		}
		return expr
	})

	// json_insert's append form. JSON_MODIFY with the 'append' keyword is the
	// direct equivalent, one call per value.
	out = rewriteCall(out, jsonInsertRE, func(arg string) string {
		column, values := splitJSONInsert(arg)
		if column == "" || len(values) == 0 {
			return "json_insert(" + arg + ")"
		}
		expr := "COALESCE(NULLIF(" + column + ", ''), '[]')"
		for _, v := range values {
			// splitJSONInsert renders each value as a JSON string literal;
			// JSON_MODIFY wants a SQL string, so the quoting is swapped back.
			expr = "JSON_MODIFY(" + expr + ", 'append $', '" +
				strings.Trim(strings.ReplaceAll(v, `\"`, `"`), `"`) + "')"
		}
		return expr
	})

	// json_array('a','b') becomes the JSON text literal it denotes, so the
	// OPENJSON rewrite below can treat it exactly like a column.
	out = rewriteCall(out, jsonArrayRE, func(arg string) string {
		return "'[" + strings.Join(sqlQuotedValuesAsJSON(arg), ",") + "]'"
	})

	// json_each(x) yields rows with a `value` column. OPENJSON's default schema
	// yields key/value/type, so `value` resolves under the same name the SQLite
	// original expects. An empty string is not valid JSON and several of these
	// columns default to '', so it is folded to an empty array first.
	out = rewriteCall(out, jsonEachRE, func(arg string) string {
		return "OPENJSON(COALESCE(NULLIF(" + arg + ", ''), '[]')) AS " + nextJSONTableAlias()
	})

	// STRING_AGG over quoted elements is the portable way to build a JSON
	// array as text; SQL Server has no JSON_ARRAYAGG before 2025.
	out = rewriteCall(out, jsonGroupArrayRE, func(arg string) string {
		return "('[' + STRING_AGG('\"' + STRING_ESCAPE(CONVERT(nvarchar(" +
			strconv.Itoa(mssqlKeyedTextLength) + "), " + arg + "), 'json') + '\"', ',') + ']')"
	})

	out = strftimeEpochRE.ReplaceAllStringFunc(out, func(match string) string {
		groups := strftimeEpochRE.FindStringSubmatch(match)
		if len(groups) < 3 {
			return match
		}
		// DATEADD from the epoch rather than a FROM_UNIXTIME equivalent, which
		// SQL Server does not have. The seconds are cast because the argument
		// is arithmetic on a text column.
		instant := "DATEADD(second, CAST(" + groups[2] + " AS BIGINT), CAST('1970-01-01' AS datetime2))"
		if groups[1] == "%Y-%m-%dT%H:%M:%SZ" || groups[1] == "%Y-%m-%dT%H:%M:%fZ" {
			// The RFC3339 case, which is the only one in the corpus. Style 126
			// is exactly this format minus the zone, so the Z is concatenated.
			return "(CONVERT(nvarchar(23), " + instant + ", 126) + 'Z')"
		}
		return "FORMAT(" + instant + ", '" + mssqlTimeFormat(groups[1]) + "')"
	})

	return out
}

// splitTopLevelArgs splits an argument list on commas that are not inside a
// nested call or a string literal.
func splitTopLevelArgs(arg string) []string {
	var (
		out      []string
		current  strings.Builder
		depth    int
		inQuoted bool
	)
	for i := 0; i < len(arg); i++ {
		c := arg[i]
		switch {
		case c == '\'':
			if inQuoted && i+1 < len(arg) && arg[i+1] == '\'' {
				current.WriteByte(c)
				current.WriteByte(arg[i+1])
				i++
				continue
			}
			inQuoted = !inQuoted
		case inQuoted:
		case c == '(':
			depth++
		case c == ')':
			depth--
		case c == ',' && depth == 0:
			out = append(out, current.String())
			current.Reset()
			continue
		}
		current.WriteByte(c)
	}
	if strings.TrimSpace(current.String()) != "" {
		out = append(out, current.String())
	}
	return out
}
