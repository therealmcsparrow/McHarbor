// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package sqldialect

import (
	"regexp"
	"strconv"
	"strings"
)

// Runtime rules translate only the SQLite idioms the application's own queries use.
// Each output no longer matches its own pattern, so a statement that passes through
// twice (a migration already translated by core/db) comes out unchanged.

var (
	insertOrIgnoreRE  = regexp.MustCompile(`(?i)\bINSERT\s+OR\s+IGNORE\s+INTO\b`)
	insertOrReplaceRE = regexp.MustCompile(`(?is)^\s*INSERT\s+OR\s+REPLACE\s+INTO\s+([a-z_][a-z0-9_]*)\s*\(([^)]*)\)`)
	collateNocaseRE   = regexp.MustCompile(`(?i)([a-z_][a-z0-9_.]*)\s+COLLATE\s+NOCASE`)
	rowidRE           = regexp.MustCompile(`(?i)\b([a-z_][a-z0-9_]*)\.rowid\b`)
	charCodeRE        = regexp.MustCompile(`(?i)\bchar\((\d+)\)`)
)

// conflictTargets names the unique key SQLite's INSERT OR REPLACE resolves on for the
// tables the application upserts that way; Postgres needs it spelled out.
var conflictTargets = map[string][]string{
	"container_uptime_daily":   {"container_id", "day"},
	"container_metrics":        {"container_id", "sampled_at"},
	"container_metrics_rollup": {"container_id", "bucket_start"},
	"host_metrics_rollup":      {"environment_id", "bucket_start"},
	"workflow_kv":              {"key"},
}

// strftimeTokens maps SQLite's strftime directives onto Postgres to_char patterns.
var strftimeTokens = strings.NewReplacer(
	"%Y", "YYYY", "%m", "MM", "%d", "DD", "%H", "HH24", "%M", "MI", "%S", "SS",
	"%f", "SS.MS", "%j", "DDD", "%W", "IW", "%w", "D",
)

func postgresRules(q string) string {
	out := q
	if insertOrIgnoreRE.MatchString(out) {
		out = insertOrIgnoreRE.ReplaceAllString(out, "INSERT INTO")
		if !strings.Contains(strings.ToUpper(out), "ON CONFLICT") {
			out = strings.TrimRight(strings.TrimSpace(out), ";") + " ON CONFLICT DO NOTHING"
		}
	}
	out = upsertPostgres(out)
	out = collateNocaseRE.ReplaceAllString(out, "lower($1)")
	out = rowidRE.ReplaceAllString(out, "$1.id")
	out = charCodeRE.ReplaceAllString(out, "chr($1)")
	out = rewriteCalls(out, "julianday", julianday)
	out = rewriteCalls(out, "datetime", datetime)
	out = rewriteCalls(out, "strftime", strftime)
	out = rewriteCalls(out, "printf", printf)
	out = rewriteCalls(out, "group_concat", groupConcat)
	out = scalarMinMax(out)
	out = widenIntegerCasts(out)
	return out
}

// widenIntegerCasts turns a cast to INTEGER into one to BIGINT. SQLite's INTEGER
// is 64-bit and Postgres's is int4, so CAST(AVG(mem_used) AS INTEGER) overflows
// on any host with more than 2 GiB of memory; SQL Server and MySQL already widen.
func widenIntegerCasts(q string) string {
	lower := strings.ToLower(q)
	if !strings.Contains(lower, "int") {
		return q
	}
	toks := lex(q)
	var b strings.Builder
	last := 0
	for i, t := range toks {
		if t.kind != tokWord || (t.text != "integer" && t.text != "int" && t.text != "int4") {
			continue
		}
		prev, next := at(toks, i-1), at(toks, i+1)
		isCast := prev.is("as") && next.is(")")
		isColonCast := prev.is(":") && at(toks, i-2).is(":") && prev.start == at(toks, i-2).end
		if !isCast && !isColonCast {
			continue
		}
		b.WriteString(q[last:t.start])
		b.WriteString("BIGINT")
		last = t.end
	}
	if last == 0 {
		return q
	}
	b.WriteString(q[last:])
	return b.String()
}

// scalarMinMax turns SQLite's multi-argument max()/min(), which are scalars, into
// GREATEST/LEAST; the one-argument forms are aggregates and are left alone.
func scalarMinMax(q string) string {
	for name, repl := range map[string]string{"max": "GREATEST", "min": "LEAST"} {
		q = rewriteCalls(q, name, func(a []string) string {
			if len(a) < 2 {
				return ""
			}
			return repl + "(" + strings.Join(a, ", ") + ")"
		})
	}
	return q
}

func upsertPostgres(q string) string {
	m := insertOrReplaceRE.FindStringSubmatchIndex(q)
	if m == nil {
		return q
	}
	table := strings.ToLower(q[m[2]:m[3]])
	target, ok := conflictTargets[table]
	if !ok {
		return q
	}
	isTarget := map[string]bool{"id": true}
	for _, c := range target {
		isTarget[c] = true
	}
	var sets []string
	for _, c := range strings.Split(q[m[4]:m[5]], ",") {
		c = strings.TrimSpace(c)
		if c != "" && !isTarget[strings.ToLower(c)] {
			sets = append(sets, c+" = excluded."+c)
		}
	}
	head := "INSERT INTO " + q[m[2]:m[3]] + " (" + q[m[4]:m[5]] + ")"
	rest := strings.TrimRight(strings.TrimSpace(q[m[1]:]), ";")
	action := "DO NOTHING"
	if len(sets) > 0 {
		action = "DO UPDATE SET " + strings.Join(sets, ", ")
	}
	return head + " " + rest + " ON CONFLICT (" + strings.Join(target, ", ") + ") " + action
}

// timeArg turns SQLite's ('now' | expr, modifiers...) into a timestamptz expression.
func timeArg(args []string) string {
	if len(args) == 0 {
		return "now()"
	}
	base := "(" + args[0] + ")::timestamptz"
	if strings.EqualFold(strings.TrimSpace(args[0]), "'now'") {
		base = "now()"
	}
	for _, mod := range args[1:] {
		base = "(" + base + " + (" + strings.TrimSpace(mod) + ")::interval)"
	}
	return base
}

func julianday(args []string) string {
	return "(extract(epoch from " + timeArg(args) + ") / 86400.0 + 2440587.5)"
}

func datetime(args []string) string {
	return "to_char(" + timeArg(args) + " AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI:SS')"
}

func strftime(args []string) string {
	if len(args) < 2 {
		return ""
	}
	format := strings.TrimSpace(args[0])
	t := timeArg(args[1:])
	if format == "'%s'" {
		return "extract(epoch from " + t + ")::bigint"
	}
	if len(format) >= 2 && format[0] == '\'' && format[len(format)-1] == '\'' {
		inner := format[1 : len(format)-1]
		return "to_char(" + t + " AT TIME ZONE 'UTC', '" + quoteLiteralsForToChar(strftimeTokens.Replace(inner)) + "')"
	}
	return ""
}

// quoteLiteralsForToChar protects the characters to_char would read as patterns; the
// SQLite formats in use only carry separators, a literal T and a literal Z.
func quoteLiteralsForToChar(f string) string {
	return strings.NewReplacer("T", `"T"`, "Z", `"Z"`).Replace(f)
}

func printf(args []string) string {
	if len(args) != 2 {
		return ""
	}
	format := strings.TrimSpace(args[0])
	if format == "'%02d'" {
		return "lpad((" + args[1] + ")::text, 2, '0')"
	}
	return ""
}

func groupConcat(args []string) string {
	switch len(args) {
	case 1:
		return "string_agg((" + args[0] + ")::text, ',')"
	case 2:
		return "string_agg((" + args[0] + ")::text, " + args[1] + ")"
	}
	return ""
}

// isTypeSize reports a DATETIME(3)-style type modifier, which shares its spelling
// with SQLite's datetime() function but must never be rewritten.
func isTypeSize(args []string) bool {
	if len(args) != 1 {
		return false
	}
	a := strings.TrimSpace(args[0])
	if a == "" {
		return false
	}
	for _, c := range a {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// rewriteCalls replaces every call to name(...) outside string literals with
// build(args); build returning "" keeps the original call, so an unsupported shape
// fails loudly on the server rather than being silently mistranslated.
func rewriteCalls(q, name string, build func(args []string) string) string {
	lower := strings.ToLower(q)
	needle := strings.ToLower(name) + "("
	if !strings.Contains(lower, needle) {
		return q
	}
	var b strings.Builder
	i := 0
	for i < len(q) {
		c := q[i]
		if c == '\'' {
			j := skipLiteral(q, i)
			b.WriteString(q[i:j])
			i = j
			continue
		}
		if strings.HasPrefix(lower[i:], needle) && (i == 0 || !isIdent(q[i-1])) {
			open := i + len(needle) - 1
			end, args, ok := splitArgs(q, open)
			if ok {
				inner := make([]string, len(args))
				for k, a := range args {
					inner[k] = rewriteCalls(a, name, build)
				}
				if isTypeSize(inner) {
					b.WriteByte(c)
					i++
					continue
				}
				if out := build(inner); out != "" {
					b.WriteString(out)
					i = end + 1
					continue
				}
			}
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}

func skipLiteral(q string, i int) int {
	j := i + 1
	for j < len(q) {
		if q[j] == '\'' {
			if j+1 < len(q) && q[j+1] == '\'' {
				j += 2
				continue
			}
			return j + 1
		}
		j++
	}
	return len(q)
}

// splitArgs reads the argument list whose '(' is at open, honouring nesting and
// literals, and returns the index of the matching ')'.
func splitArgs(q string, open int) (int, []string, bool) {
	depth := 0
	start := open + 1
	var args []string
	for i := open; i < len(q); i++ {
		switch q[i] {
		case '\'':
			i = skipLiteral(q, i) - 1
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				args = append(args, strings.TrimSpace(q[start:i]))
				return i, args, true
			}
		case ',':
			if depth == 1 {
				args = append(args, strings.TrimSpace(q[start:i]))
				start = i + 1
			}
		}
	}
	return 0, nil, false
}

func isIdent(c byte) bool {
	return c == '_' || c == '.' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

var insertOrReplaceMySQLRE = regexp.MustCompile(`(?i)\bINSERT\s+OR\s+REPLACE\s+INTO\b`)

// mysqlTime turns SQLite's ('now' | expr, modifiers...) into a MySQL DATETIME in UTC;
// stored stamps are RFC3339 text, which MySQL only parses once the T and Z are gone.
func mysqlTime(args []string) string {
	if len(args) == 0 {
		return "UTC_TIMESTAMP(6)"
	}
	base := "CAST(REPLACE(REPLACE(" + args[0] + ", 'T', ' '), 'Z', '') AS DATETIME(6))"
	if strings.EqualFold(strings.TrimSpace(args[0]), "'now'") {
		base = "UTC_TIMESTAMP(6)"
	}
	for _, mod := range args[1:] {
		base = "DATE_ADD(" + base + ", INTERVAL CAST(SUBSTRING_INDEX(TRIM(" + mod + "), ' ', 1) AS SIGNED) DAY)"
	}
	return base
}

func mysqlRules(q string) string {
	out := insertOrIgnoreRE.ReplaceAllString(q, "INSERT IGNORE INTO")
	out = insertOrReplaceMySQLRE.ReplaceAllString(out, "REPLACE INTO")
	out = collateNocaseRE.ReplaceAllString(out, "lower($1)")
	out = rewriteCalls(out, "julianday", func(a []string) string {
		return "(TIMESTAMPDIFF(MICROSECOND, '1970-01-01 00:00:00', " + mysqlTime(a) + ") / 86400000000.0 + 2440587.5)"
	})
	out = rewriteCalls(out, "datetime", func(a []string) string {
		return "DATE_FORMAT(" + mysqlTime(a) + ", '%Y-%m-%d %H:%i:%s')"
	})
	out = rewriteCalls(out, "strftime", func(a []string) string {
		if len(a) < 2 {
			return ""
		}
		if strings.TrimSpace(a[0]) == "'%s'" {
			return "TIMESTAMPDIFF(SECOND, '1970-01-01 00:00:00', " + mysqlTime(a[1:]) + ")"
		}
		return "DATE_FORMAT(" + mysqlTime(a[1:]) + ", " + strings.ReplaceAll(a[0], "%M", "%i") + ")"
	})
	out = rewriteCalls(out, "printf", func(a []string) string {
		if len(a) == 2 && strings.TrimSpace(a[0]) == "'%02d'" {
			return "LPAD(" + a[1] + ", 2, '0')"
		}
		return ""
	})
	out = scalarMinMax(out)
	out = rewriteCalls(out, "group_concat", func(a []string) string {
		if len(a) == 2 {
			return "GROUP_CONCAT(" + a[0] + " SEPARATOR " + separatorLiteral(a[1]) + ")"
		}
		return ""
	})
	return out
}

// separatorLiteral turns a char(N) separator into the literal MySQL requires there;
// GROUP_CONCAT's SEPARATOR takes no expression.
func separatorLiteral(sep string) string {
	if m := charCodeRE.FindStringSubmatch(strings.TrimSpace(sep)); m != nil && len(m[0]) == len(strings.TrimSpace(sep)) {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 && n < 128 && n != '\'' {
			return "'" + string(rune(n)) + "'"
		}
	}
	return sep
}
