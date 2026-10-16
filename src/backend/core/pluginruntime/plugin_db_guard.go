// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pluginruntime

import (
	"regexp"
	"strings"

	plugin "github.com/therealmcsparrow/mcharbor/core/plugin"
)

// pluginDBExec is the table-prefix guard: a plugin statement that names a table
// outside the plugin's own prefix is rejected.
//
// It is defence in depth, not a security boundary — a plugin is a Go `.so`
// opened into this process and already owns its memory — so the job is catching
// a statement that touches the wrong table by accident, or through a migration
// copied from another plugin.
//
// The first version was `strings.Contains(query, prefix)`, which
// `DELETE FROM users -- plugin_foo_` satisfies: a prefix mentioned anywhere,
// comments included, passed. Comments are stripped now and every table the
// statement names must carry the prefix, so mentioning it is no longer enough.
func pluginDBExec(prefix, query string) error {
	stripped := strings.ToLower(stripSQLComments(query))
	tables := sqlTableReferences(stripped)
	if len(tables) == 0 {
		// A statement that names no table — `SELECT 1`, a PRAGMA — has nothing
		// to scope and nothing to reach, but it must still mention the prefix so
		// a plugin cannot use the no-table branch as a way past the guard.
		if !strings.Contains(stripped, prefix) {
			return plugin.ErrScopeViolation
		}
		return nil
	}
	for _, table := range tables {
		if !strings.HasPrefix(table, prefix) {
			return plugin.ErrScopeViolation
		}
	}
	return nil
}

// sqlTableReferenceRE finds each table-introducing keyword and the identifier
// after it, tolerating an `IF NOT EXISTS` and the four identifier quotings the
// supported backends use.
var sqlTableReferenceRE = regexp.MustCompile(
	"(?is)\\b(?:from|join|into|update|table)\\s+(?:if\\s+(?:not\\s+)?exists\\s+)?[\"'`\\[]?([a-z_][a-z0-9_]*)")

// sqlSubqueryKeywords are the words that follow a table-introducing keyword when
// what comes next is a subquery or a clause rather than a table name.
var sqlSubqueryKeywords = map[string]bool{
	"select": true, "values": true, "set": true, "where": true, "only": true,
}

// sqlTableReferences returns every table name the statement names.
//
// It is a keyword scan rather than a parser. A false positive costs a plugin a
// rejected statement it can rewrite; a false negative is a table it should not
// have reached — so the scan errs towards rejecting.
func sqlTableReferences(query string) []string {
	matches := sqlTableReferenceRE.FindAllStringSubmatch(query, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if sqlSubqueryKeywords[m[1]] {
			continue
		}
		out = append(out, m[1])
	}
	return out
}

// stripSQLComments removes -- line comments and /* */ block comments, so neither
// a table name nor a prefix can be hidden inside one.
func stripSQLComments(query string) string {
	const quote = '\''
	var b strings.Builder
	b.Grow(len(query))
	inQuoted := false
	for i := 0; i < len(query); i++ {
		c := query[i]
		switch {
		case c == quote:
			inQuoted = !inQuoted
			b.WriteByte(c)
		case inQuoted:
			b.WriteByte(c)
		case c == '-' && i+1 < len(query) && query[i+1] == '-':
			j := strings.IndexByte(query[i:], '\n')
			if j < 0 {
				return b.String()
			}
			i += j
			b.WriteByte('\n')
		case c == '/' && i+1 < len(query) && query[i+1] == '*':
			j := strings.Index(query[i+2:], "*/")
			if j < 0 {
				return b.String()
			}
			i += 2 + j + 1
			b.WriteByte(' ')
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
