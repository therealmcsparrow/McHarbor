// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package sqldialect

import (
	"database/sql/driver"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// This file is the runtime half of the SQL Server port; core/db/dialect_mssql.go is the
// migration half. The two are deliberately separate implementations rather than one
// shared builder: core/db imports the drivers and owns a historical corpus it may not
// edit, while this package sees a prepared statement whose placeholders must keep their
// ordinals. Sharing a MERGE builder between them would mean either an import cycle or
// pulling the driver imports into the translator, and neither input has the same shape.
//
// Every rule below answers to a statement that exists in src/backend/modules/** or
// src/backend/core/**; the corpus was swept before the rules were written, and the ones
// with no call site are marked so.

// mssqlRFC3339Now reproduces SQLite's strftime('%Y-%m-%dT%H:%M:%fZ','now'). It must stay
// byte-identical to rfc3339Now in core/db/dialect_mssql.go: the same column is written by
// a migration and by a runtime statement, and a stamp that differs in shape would sort
// wrongly against its neighbours.
const mssqlRFC3339Now = "(CONVERT(nvarchar(23), SYSUTCDATETIME(), 126) + 'Z')"

// MSSQL translates one runtime statement into T-SQL: SQLite idioms first, then the `?`
// placeholders to the `@pN` form go-mssqldb derives from each argument's ordinal, then the
// rules that can only be applied once the placeholders have names.
func MSSQL(q string) string {
	return mssqlMemo.get(q, func(s string) string {
		s = mssqlAliasDerivedTables(mssqlUpsert(mssqlRules(s)))
		s = rebindWith(s, "@p")
		return quoteReservedMSSQL(mssqlNullSafeEquality(mssqlLimitRules(s)))
	})
}

// MSSQLArgs turns Go booleans into 1/0. The schema stores every flag as an integer, as
// SQLite does, so a Go bool would arrive as a TDS BIT and lean on an implicit conversion
// that a CHECK constraint or a comparison against a BIGINT column can still refuse.
func MSSQLArgs(_ string, args []driver.NamedValue) {
	for i := range args {
		if b, ok := args[i].Value.(bool); ok {
			args[i].Value = mssqlBoolAsInt(b)
		}
	}
}

// MSSQLValue turns a BIT result column into 1/0, because the application scans flags into
// integers. The host schema has no BIT column — BOOLEAN becomes BIGINT — but a plugin
// migration can still declare one, and database/sql will not convert a bool into an int.
func MSSQLValue(v driver.Value) driver.Value {
	if b, ok := v.(bool); ok {
		return mssqlBoolAsInt(b)
	}
	return v
}

func mssqlBoolAsInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// MSSQLDSN names the application in sys.dm_exec_sessions so a DBA can tell McHarbor's
// connections from anything else sharing the instance. Unlike MySQL, nothing behavioural
// has to be pinned here: the TDS login already sets ANSI_NULLS and QUOTED_IDENTIFIER on,
// and every timestamp column in the schema is text, so no time-zone handling applies.
func MSSQLDSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return dsn
	}
	params := u.Query()
	for _, k := range []string{"app", "app name", "application name"} {
		if params.Get(k) != "" {
			return dsn
		}
	}
	params.Set("app", "McHarbor")
	u.RawQuery = params.Encode()
	return u.String()
}

var (
	// mssqlGlobRE is SQLite's GLOB. T-SQL's LIKE understands character classes, so a
	// prefix pattern translates exactly. No runtime statement uses GLOB today; one
	// migration does, and a plugin may.
	mssqlGlobRE = regexp.MustCompile(`(?i)\bGLOB\s+'([^']*)'`)
	// mssqlJSONExtractRE and mssqlJSONExtractWholeRE are the two json_extract shapes the
	// schema uses. Neither appears in a runtime statement; both do in migrations.
	mssqlJSONExtractRE      = regexp.MustCompile(`(?i)\bjson_extract\(\s*([^,()]+?)\s*,\s*'\$\.([a-z0-9_]+)'\s*\)`)
	mssqlJSONExtractWholeRE = regexp.MustCompile(`(?i)\bjson_extract\(\s*([^,()]+?)\s*,\s*'\$'\s*\)`)
	// mssqlNegativeLiteralRE matches a substr start written as a literal negative number.
	mssqlNegativeLiteralRE = regexp.MustCompile(`^\s*\(?\s*-\s*[0-9]+\s*\)?\s*$`)
	// mssqlRandomblobRE recognises the one argument hex() is ever given here.
	mssqlRandomblobRE = regexp.MustCompile(`(?i)^randomblob\(\s*(\d+)\s*\)$`)
	// mssqlTimeModRE reads a literal SQLite time modifier such as '-1 day'.
	mssqlTimeModRE = regexp.MustCompile(`(?i)^'\s*([+-]?\d+)\s+(second|minute|hour|day|month|year)s?\s*'$`)
	// mssqlTimeModConcatRE reads the modifier the api_key usage pruner builds at runtime,
	// `'-' || ? || ' days'`, whose count is a bound argument rather than a literal.
	mssqlTimeModConcatRE = regexp.MustCompile(`(?i)^'([+-])'\s*\|\|\s*(.+?)\s*\|\|\s*'\s*(second|minute|hour|day|month|year)s?'$`)
	// mssqlParamName matches the `pN` half of a placeholder the rebind wrote, so a
	// hand-written `@LockTimeout` is never mistaken for one.
	mssqlParamName = regexp.MustCompile(`^p[0-9]+$`)
	// mssqlExcludedRE is the SQLite name for the row being inserted; a MERGE calls it the
	// source alias. Both spellings appear in the corpus.
	mssqlExcludedRE = regexp.MustCompile(`(?i)\bexcluded\.`)
	// mssqlUpsertSelectTrueRE is the `WHERE true` SQLite's grammar requires between an
	// upsert's SELECT and its ON CONFLICT; T-SQL has no boolean literal to give it.
	mssqlUpsertSelectTrueRE = regexp.MustCompile(`(?i)\s+WHERE\s+true\s*;?\s*$`)
)

// mssqlCastTypes maps the SQLite type names a CAST targets onto T-SQL's. BIGINT maps to
// itself so a second pass over an already-translated statement changes nothing.
var mssqlCastTypes = map[string]string{
	"integer": "BIGINT", "int": "BIGINT", "bigint": "BIGINT",
	"text": "NVARCHAR(MAX)", "real": "FLOAT", "blob": "VARBINARY(MAX)",
}

func mssqlRules(q string) string {
	out := q
	// The default collation of a SQL Server database is case-insensitive, so NOCASE is
	// already the behaviour, but the clause itself is unknown syntax.
	out = collateNocaseRE.ReplaceAllString(out, "lower($1)")
	out = rowidRE.ReplaceAllString(out, "$1.id")
	// The time functions run before the `||` rewrite: the only dynamic modifier in the
	// corpus is built with `||`, and DATEADD needs it in SQLite spelling to read the unit.
	out = rewriteCalls(out, "julianday", mssqlJulianday)
	out = rewriteCalls(out, "datetime", mssqlDatetime)
	out = rewriteCalls(out, "strftime", mssqlStrftime)
	out = rewriteCalls(out, "printf", mssqlPrintf)
	// group_concat before char(): STRING_AGG's separator takes a literal, not NCHAR(31).
	out = rewriteCalls(out, "group_concat", mssqlGroupConcat)
	out = rewriteCalls(out, "instr", mssqlInstr)
	out = rewriteCalls(out, "substr", mssqlSubstr)
	out = rewriteCalls(out, "ifnull", mssqlIfnull)
	out = rewriteCalls(out, "hex", mssqlHex)
	out = rewriteCalls(out, "random", mssqlRandom)
	out = mssqlScalarMinMax(out)
	out = charCodeRE.ReplaceAllString(out, "NCHAR($1)")
	out = mssqlJSONExtractWholeRE.ReplaceAllString(out, "$1")
	out = mssqlJSONExtractRE.ReplaceAllString(out, "JSON_VALUE($1, '$.$2')")
	out = mssqlGlobRE.ReplaceAllStringFunc(out, mssqlGlob)
	out = mssqlCasts(out)
	out = mssqlCurrentTimestamp(out)
	out = mssqlConcat(out)
	return out
}

// mssqlTime turns SQLite's ('now' | expr, modifiers…) into a UTC datetime2. Stored stamps
// are RFC3339 text, which SQL Server only converts reliably once the T and Z are gone.
// It reports false on a modifier it cannot read, so the caller keeps the original call and
// the server refuses it rather than silently returning the wrong instant.
func mssqlTime(args []string) (string, bool) {
	if len(args) == 0 {
		return "SYSUTCDATETIME()", true
	}
	base := "CAST(REPLACE(REPLACE(" + args[0] + ", 'T', ' '), 'Z', '') AS DATETIME2(3))"
	if strings.EqualFold(strings.TrimSpace(args[0]), "'now'") {
		base = "SYSUTCDATETIME()"
	}
	for _, mod := range args[1:] {
		unit, amount, ok := mssqlTimeModifier(mod)
		if !ok {
			return "", false
		}
		base = "DATEADD(" + unit + ", " + amount + ", " + base + ")"
	}
	return base, true
}

func mssqlTimeModifier(mod string) (unit, amount string, ok bool) {
	mod = strings.TrimSpace(mod)
	if m := mssqlTimeModRE.FindStringSubmatch(mod); m != nil {
		return strings.ToLower(m[2]), m[1], true
	}
	if m := mssqlTimeModConcatRE.FindStringSubmatch(mod); m != nil {
		sign := ""
		if m[1] == "-" {
			sign = "-"
		}
		return strings.ToLower(m[3]), sign + "(" + strings.TrimSpace(m[2]) + ")", true
	}
	return "", "", false
}

// mssqlJulianday returns a real julian day number rather than a datetime, so a caller that
// subtracts two of them gets the same answer SQLite would; DATEDIFF_BIG is needed because
// the microsecond count overflows a 32-bit DATEDIFF within days.
func mssqlJulianday(args []string) string {
	t, ok := mssqlTime(args)
	if !ok {
		return ""
	}
	return "(DATEDIFF_BIG(microsecond, '1970-01-01', " + t + ") / 86400000000.0 + 2440587.5)"
}

// mssqlDatetime uses style 120, which is SQLite's `YYYY-MM-DD HH:MM:SS` exactly — a space
// rather than the T of the RFC3339 stamps most of these columns hold, which is the
// inconsistency the SQLite original already has.
func mssqlDatetime(args []string) string {
	t, ok := mssqlTime(args)
	if !ok {
		return ""
	}
	return "CONVERT(nvarchar(19), " + t + ", 120)"
}

// mssqlStrftime covers the formats the corpus asks for and nothing else; T-SQL has no
// general strftime, and FORMAT() is a CLR call that would be run per row.
func mssqlStrftime(args []string) string {
	if len(args) < 2 {
		return ""
	}
	t, ok := mssqlTime(args[1:])
	if !ok {
		return ""
	}
	switch strings.TrimSpace(args[0]) {
	case "'%s'":
		return "DATEDIFF_BIG(second, '1970-01-01', " + t + ")"
	case "'%Y-%m-%dT%H:%M:%SZ'", "'%Y-%m-%dT%H:%M:%fZ'":
		return "(CONVERT(nvarchar(19), " + t + ", 126) + 'Z')"
	case "'%Y-%m-%d'":
		return "CONVERT(nvarchar(10), " + t + ", 23)"
	case "'%Y-%m-%d %H:%M:%S'":
		return "CONVERT(nvarchar(19), " + t + ", 120)"
	}
	return ""
}

// mssqlGroupConcat maps onto STRING_AGG. The separator is reduced to a literal because
// T-SQL accepts only a literal or a variable there, never NCHAR(31); the input is cast to
// nvarchar(max) because otherwise the result is nvarchar(4000) and a long list raises 9829.
func mssqlGroupConcat(args []string) string {
	switch len(args) {
	case 1:
		return "STRING_AGG(CAST(" + args[0] + " AS nvarchar(max)), ',')"
	case 2:
		return "STRING_AGG(CAST(" + args[0] + " AS nvarchar(max)), " + separatorLiteral(args[1]) + ")"
	}
	return ""
}

// mssqlInstr reverses the argument order, which is the whole reason this rule exists:
// CHARINDEX takes the needle first and a silent swap returns plausible wrong rows.
func mssqlInstr(args []string) string {
	if len(args) != 2 {
		return ""
	}
	return "CHARINDEX(" + args[1] + ", " + args[0] + ")"
}

// mssqlSubstr supplies the length T-SQL insists on; SQLite's two-argument form runs to the
// end of the string, which int max reproduces. A literal negative start counts from the end
// in SQLite and from before the start in T-SQL, so it is left untranslated to fail loudly.
func mssqlSubstr(args []string) string {
	if len(args) >= 2 && mssqlNegativeLiteralRE.MatchString(args[1]) {
		return ""
	}
	switch len(args) {
	case 2:
		return "SUBSTRING(" + args[0] + ", " + args[1] + ", 2147483647)"
	case 3:
		return "SUBSTRING(" + args[0] + ", " + args[1] + ", " + args[2] + ")"
	}
	return ""
}

func mssqlIfnull(args []string) string {
	if len(args) != 2 {
		return ""
	}
	return "ISNULL(" + args[0] + ", " + args[1] + ")"
}

// mssqlHex handles hex(randomblob(n)) and nothing else, because style 2 renders varbinary
// as the bare hex hex() produces and there is no general hex() in T-SQL below 2019.
func mssqlHex(args []string) string {
	if len(args) != 1 {
		return ""
	}
	m := mssqlRandomblobRE.FindStringSubmatch(strings.TrimSpace(args[0]))
	if m == nil {
		return ""
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 || n > 64 {
		return ""
	}
	return "CONVERT(nvarchar(" + strconv.Itoa(n*2) + "), CRYPT_GEN_RANDOM(" + strconv.Itoa(n) + "), 2)"
}

// mssqlRandom stands in for SQLite's random(); NEWID is the only per-row entropy T-SQL
// has, since RAND() is evaluated once per statement.
func mssqlRandom(args []string) string {
	if len(args) != 1 || args[0] != "" {
		return ""
	}
	return "CHECKSUM(NEWID())"
}

// mssqlPrintf covers the one format SQLite is asked for anywhere in this repo. No runtime
// statement uses printf today.
func mssqlPrintf(args []string) string {
	if len(args) != 2 || strings.TrimSpace(args[0]) != "'%02d'" {
		return ""
	}
	return "RIGHT('0' + CONVERT(nvarchar(12), " + args[1] + "), 2)"
}

// mssqlGlob translates a prefix GLOB into the LIKE that means the same. Anything with a
// wildcard elsewhere is left alone, because LIKE's and GLOB's escaping differ.
func mssqlGlob(match string) string {
	m := mssqlGlobRE.FindStringSubmatch(match)
	if m == nil || !strings.HasSuffix(m[1], "*") {
		return match
	}
	prefix := strings.TrimSuffix(m[1], "*")
	if strings.ContainsAny(prefix, "*?") {
		return match
	}
	return "LIKE '" + prefix + "%'"
}

// mssqlScalarMinMax turns SQLite's two-argument max()/min(), which are scalars, into a
// CASE. GREATEST/LEAST would read better but arrived in SQL Server 2022, and this backend
// supports 2019. The one-argument forms are aggregates and are left alone.
func mssqlScalarMinMax(q string) string {
	for name, op := range map[string]string{"max": ">=", "min": "<="} {

		q = rewriteCalls(q, name, func(a []string) string {
			if len(a) != 2 {
				return ""
			}
			return "(CASE WHEN (" + a[0] + ") " + op + " (" + a[1] + ") THEN (" + a[0] + ") ELSE (" + a[1] + ") END)"
		})
	}
	return q
}

// mssqlCasts renames a CAST's target type. Token-driven so `SELECT x AS text` — an alias
// that happens to spell a type — is not rewritten.
func mssqlCasts(q string) string {
	if !strings.Contains(strings.ToLower(q), "cast") {
		return q
	}
	toks := lex(q)
	var edits []edit
	for i, t := range toks {
		if !t.is("as") || at(toks, i+1).kind != tokWord || !at(toks, i+2).is(")") {
			continue
		}
		repl := mssqlCastTypes[toks[i+1].text]
		if repl == "" {
			continue
		}
		if open := matchingOpen(toks, i+2); open > 0 && toks[open-1].is("cast") {
			edits = append(edits, edit{toks[i+1].start, toks[i+1].end, repl})
		}
	}
	return applyEdits(q, edits)
}

// mssqlCurrentTimestamp replaces the keyword with an RFC3339 string. T-SQL's
// CURRENT_TIMESTAMP is a datetime, and inserting one into the nvarchar column these
// statements target would store `Sep 27 2026  3:01AM` without complaining.
func mssqlCurrentTimestamp(q string) string {
	if !strings.Contains(strings.ToUpper(q), "CURRENT_TIMESTAMP") {
		return q
	}
	var edits []edit
	for _, t := range lex(q) {
		if t.kind == tokWord && t.text == "current_timestamp" {
			edits = append(edits, edit{t.start, t.end, mssqlRFC3339Now})
		}
	}
	return applyEdits(q, edits)
}

// mssqlConcat rewrites `||` to `+`. Token-driven because the RBAC policy expressions and
// the alert state keys both carry a `||` inside a string literal, where it is data.
func mssqlConcat(q string) string {
	if !strings.Contains(q, "||") {
		return q
	}
	var edits []edit
	for _, t := range lex(q) {
		if t.is("||") {
			edits = append(edits, edit{t.start, t.end, "+"})
		}
	}
	return applyEdits(q, edits)
}

// mssqlUpsert turns the four SQLite upsert spellings the application uses into T-SQL.
//
// `DO UPDATE` becomes a MERGE. Every "ignore the duplicate" form becomes a TRY/CATCH that
// swallows the unique-violation errors, rather than the NOT EXISTS predicate the migration
// translator builds: the index decides there, including SQLite's rule that two NULLs do
// not collide, and `user_roles` inserts a NULL environment_id into its unique key — a
// predicate would suppress a row SQLite inserts.
func mssqlUpsert(q string) string {
	ignore := insertOrIgnoreRE.MatchString(q)
	out := q
	if ignore {
		out = insertOrIgnoreRE.ReplaceAllString(out, "INSERT INTO")
	}
	if insertOrReplaceRE.MatchString(out) {
		// Canonicalised through the Postgres builder so one MERGE builder covers both
		// spellings; it resolves the conflict key out of conflictTargets.
		out = upsertPostgres(out)
	}
	if !strings.Contains(strings.ToLower(out), "conflict") {
		if ignore {
			return mssqlTolerateDuplicate(out)
		}
		return out
	}

	toks := lex(out)
	on := -1
	for i := 0; i+1 < len(toks); i++ {
		if toks[i].is("on") && toks[i+1].is("conflict") {
			on = i
			break
		}
	}
	if on < 0 {
		if ignore {
			return mssqlTolerateDuplicate(out)
		}
		return out
	}
	j := on + 2
	var keys []string
	if at(toks, j).is("(") {
		end := matchingClose(toks, j)
		if end < 0 {
			return out
		}
		for k := j + 1; k < end; k++ {
			if toks[k].isName() {
				keys = append(keys, out[toks[k].start:toks[k].end])
			}
		}
		j = end + 1
	}
	if !at(toks, j).is("do") {
		return out
	}
	head := strings.TrimRight(out[:toks[on].start], " \t\r\n")
	switch {
	case at(toks, j+1).is("nothing"):
		return mssqlTolerateDuplicate(head)
	case at(toks, j+1).is("update") && at(toks, j+2).is("set"):
		if k := j + 3; k < len(toks) {
			if merged, ok := mssqlMergeUpsert(head, keys, out[toks[k].start:]); ok {
				return merged
			}
		}
	}
	// A shape the MERGE builder cannot read keeps its ON CONFLICT clause, so the server
	// refuses it rather than silently losing the update half.
	return out
}

// mssqlTolerateDuplicate wraps a statement so a unique-key collision is ignored, which is
// what INSERT OR IGNORE and ON CONFLICT DO NOTHING mean. Safe inside a transaction because
// go-mssqldb leaves XACT_ABORT off, so a constraint violation aborts the statement and
// not the transaction; any other error is rethrown unchanged.
func mssqlTolerateDuplicate(stmt string) string {
	body := strings.TrimRight(strings.TrimSpace(stmt), ";")
	return "BEGIN TRY " + body +
		"; END TRY BEGIN CATCH IF ERROR_NUMBER() NOT IN (2601, 2627) THROW; END CATCH;"
}

// mssqlMergeUpsert builds the MERGE that stands in for ON CONFLICT … DO UPDATE, from an
// INSERT whose source is a single VALUES row or a SELECT. A SELECT that yields one key
// twice is refused by the server (error 8672) where SQLite would upsert twice in turn.
// HOLDLOCK is not optional: without it MERGE takes update locks late and two nodes racing
// on the same key deadlock instead of one waiting.
func mssqlMergeUpsert(head string, keys []string, setBody string) (string, bool) {
	toks := lex(head)
	if len(toks) == 0 || !toks[0].is("insert") || len(keys) == 0 {
		return "", false
	}
	into := -1
	for i, t := range toks {
		if t.is("into") {
			into = i
			break
		}
	}
	if into < 0 || !at(toks, into+1).isName() {
		return "", false
	}
	table := head[toks[into+1].start:toks[into+1].end]

	open := into + 2
	if !at(toks, open).is("(") {
		return "", false
	}
	closeCols := matchingClose(toks, open)
	if closeCols < 0 {
		return "", false
	}
	var cols []string
	for k := open + 1; k < closeCols; k++ {
		if toks[k].isName() {
			cols = append(cols, head[toks[k].start:toks[k].end])
		}
	}
	if len(cols) == 0 {
		return "", false
	}

	v := closeCols + 1
	var source string
	switch {
	case at(toks, v).is("values") && at(toks, v+1).is("("):
		closeTuple := matchingClose(toks, v+1)
		// The tuple has to be the last thing in the statement; a multi-row VALUES or a
		// trailing clause is a shape this builder does not claim to handle.
		if closeTuple != len(toks)-1 {
			return "", false
		}
		source = "(VALUES (" + head[toks[v+1].end:toks[closeTuple].start] + "))"
	case at(toks, v).is("select"):
		// A derived table's column list renames whatever the SELECT produces, so the
		// source columns line up with the INSERT's by position, exactly as SQLite does.
		body := mssqlUpsertSelectTrueRE.ReplaceAllString(head[toks[v].start:], "")
		source = "(" + strings.TrimRight(strings.TrimSpace(body), "; \t\r\n") + ")"
	default:
		return "", false
	}

	set := mssqlExcludedRE.ReplaceAllString(setBody, "src.")
	// The settings_catalogs upsert reads its own current row by table name, which the
	// MERGE has aliased away.
	set = regexp.MustCompile(`(?i)\b`+regexp.QuoteMeta(table)+`\.`).ReplaceAllString(set, "tgt.")
	set = strings.TrimRight(strings.TrimSpace(set), "; \t\r\n")

	predicate := make([]string, 0, len(keys))
	for _, k := range keys {
		predicate = append(predicate, "tgt."+k+" = src."+k)
	}
	sourced := make([]string, 0, len(cols))
	for _, c := range cols {
		sourced = append(sourced, "src."+c)
	}
	colList := strings.Join(cols, ", ")

	return "MERGE " + table + " WITH (HOLDLOCK) AS tgt USING " + source + " AS src (" +
		colList + ") ON " + strings.Join(predicate, " AND ") +
		" WHEN MATCHED THEN UPDATE SET " + set +
		" WHEN NOT MATCHED THEN INSERT (" + colList + ") VALUES (" + strings.Join(sourced, ", ") + ");", true
}

// mssqlAliasDerivedTables names every derived table in a FROM or JOIN, which T-SQL
// requires as MySQL does. It is not the MySQL pass: that one shares its counter with the
// IN-subquery wrap, so the alias numbers would shift if the two were merged.
func mssqlAliasDerivedTables(q string) string {
	if !strings.Contains(q, "(") {
		return q
	}
	toks := lex(q)
	var edits []edit
	n := 0
	for i, t := range toks {
		if !t.is("(") || !at(toks, i+1).is("select") {
			continue
		}
		if !at(toks, i-1).is("from") && !at(toks, i-1).is("join") {
			continue
		}
		end := matchingClose(toks, i)
		if end < 0 {
			continue
		}
		next := at(toks, end+1)
		if next.is("as") || next.isName() && (next.kind != tokWord || !notAlias[next.text]) {
			continue
		}
		n++
		edits = append(edits, edit{toks[end].end, toks[end].end, " AS mch_d" + strconv.Itoa(n)})
	}
	return applyEdits(q, edits)
}

// mssqlLimitRules replaces every LIMIT, at whatever nesting depth, with the T-SQL that
// means the same for that query block.
//
// A LIMIT with an OFFSET becomes OFFSET/FETCH, which needs an ORDER BY at the same level;
// every paginated statement in this application has one, and a statement that does not is
// left untranslated so the server refuses it rather than silently paging wrongly. A bare
// LIMIT becomes TOP on the SELECT that owns it, because several of those are scalar
// subqueries with no ORDER BY at all.
//
// It runs after the rebind, so moving a count to the front of the SELECT moves `@p4`
// rather than a `?` whose number would then be assigned from its new position.
func mssqlLimitRules(q string) string {
	if !strings.Contains(strings.ToLower(q), "limit") {
		return q
	}
	toks := lex(q)
	var edits []edit
	for i, t := range toks {
		if !t.is("limit") || i == 0 {
			continue
		}
		count, j, ok := mssqlOperand(q, toks, i+1)
		if !ok {
			continue
		}
		if at(toks, j).is("offset") {
			offset, k, ok := mssqlOperand(q, toks, j+1)
			if !ok || !mssqlHasOrderBy(toks, i) {
				continue
			}
			edits = append(edits, edit{t.start, toks[k-1].end,
				"OFFSET " + offset + " ROWS FETCH NEXT " + count + " ROWS ONLY"})
			continue
		}
		sel, ok := mssqlOwningSelect(toks, i)
		if !ok {
			continue
		}
		edits = append(edits,
			edit{toks[i-1].end, toks[j-1].end, ""},
			edit{toks[sel].end, toks[sel].end, " TOP (" + count + ")"})
	}
	return applyEdits(q, edits)
}

// mssqlOperand reads the count after LIMIT or OFFSET: a literal, or the `@pN` the rebind
// left there, which lexes as a punctuation token and a word.
func mssqlOperand(q string, toks []token, i int) (text string, next int, ok bool) {
	t := at(toks, i)
	if t.kind == tokNumber {
		return q[t.start:t.end], i + 1, true
	}
	if n := at(toks, i+1); t.is("@") && n.kind == tokWord && mssqlParamName.MatchString(n.text) {
		return q[t.start:n.end], i + 2, true
	}
	return "", i, false
}

// mssqlOwningSelect returns the index of the token a TOP must follow for the LIMIT at
// from: the SELECT that begins the same query block, or its DISTINCT. It reports false
// across a set operator, where TOP on one branch would not bound the compound.
func mssqlOwningSelect(toks []token, from int) (int, bool) {
	depth := 0
	for j := from - 1; j >= 0; j-- {
		switch {
		case toks[j].is(")"):
			depth++
		case toks[j].is("("):
			depth--
			if depth < 0 {
				return 0, false
			}
		case depth != 0:
		case toks[j].is("select"):
			if mssqlInCompound(toks, j) {
				return 0, false
			}
			if n := at(toks, j+1); n.is("distinct") || n.is("all") {
				return j + 1, true
			}
			return j, true
		}
	}
	return 0, false
}

// mssqlInCompound reports a SELECT that is one branch of a set operator, where a LIMIT
// belongs to the compound and a TOP on the branch would not bound it.
func mssqlInCompound(toks []token, sel int) bool {
	depth := 0
	for j := sel - 1; j >= 0; j-- {
		switch {
		case toks[j].is(")"):
			depth++
		case toks[j].is("("):
			depth--
			if depth < 0 {
				return false
			}
		case depth != 0:
		case toks[j].is("union"), toks[j].is("except"), toks[j].is("intersect"):
			return true
		}
	}
	return false
}

// mssqlHasOrderBy reports an ORDER BY belonging to the same query block as the LIMIT at
// limit, which is what OFFSET/FETCH requires and TOP does not.
func mssqlHasOrderBy(toks []token, limit int) bool {
	want := 0
	for j := 0; j < limit; j++ {
		switch {
		case toks[j].is("("):
			want++
		case toks[j].is(")"):
			want--
		}
	}
	depth := 0
	for j := 0; j < limit; j++ {
		switch {
		case toks[j].is("("):
			depth++
		case toks[j].is(")"):
			depth--
		case depth == want && toks[j].is("order") && at(toks, j+1).is("by"):
			return true
		}
	}
	return false
}

// mssqlNullSafeEquality expands `col IS @pN`, SQLite's null-safe comparison, into the
// predicate T-SQL needs. The parameter is named twice, which is legal because a name is
// declared once however often the batch mentions it.
func mssqlNullSafeEquality(q string) string {
	if !strings.Contains(strings.ToLower(q), " is ") {
		return q
	}
	toks := lex(q)
	var edits []edit
	for i, t := range toks {
		if !t.is("is") {
			continue
		}
		param, j, ok := mssqlOperand(q, toks, i+1)
		if !ok {
			continue
		}
		start, ok := mssqlColumnStart(toks, i-1)
		if !ok {
			continue
		}
		col := q[toks[start].start:toks[i-1].end]
		edits = append(edits, edit{toks[start].start, toks[j-1].end,
			"(" + col + " = " + param + " OR (" + col + " IS NULL AND " + param + " IS NULL))"})
	}
	return applyEdits(q, edits)
}

// mssqlColumnStart returns the first token of the possibly qualified column name that ends
// at i.
func mssqlColumnStart(toks []token, i int) (int, bool) {
	if !at(toks, i).isName() {
		return 0, false
	}
	if at(toks, i-1).is(".") && at(toks, i-2).isName() {
		return i - 2, true
	}
	return i, true
}

// mssqlRuntimeReserved lists the names this schema uses as identifiers that T-SQL
// reserves. Derived by intersecting all 1055 table and column names the migration corpus
// declares with the T-SQL reserved-keyword list, not with MySQL's: only these two are in
// both sets, and `size`, `system`, `condition`, `groups` and `generated` — which MySQL
// does reserve — are not T-SQL keywords at all.
var mssqlRuntimeReserved = map[string]bool{"key": true, "trigger": true}

// quoteReservedMSSQL brackets the reserved words a statement uses as names. Literals,
// comments and already-bracketed names are never touched, so running it twice is a no-op.
func quoteReservedMSSQL(q string) string {
	if !containsMSSQLReserved(q) {
		return q
	}
	toks := lex(q)
	if !mssqlQuotable(toks) {
		return q
	}
	var b strings.Builder
	last := 0
	for i, t := range toks {
		if t.kind != tokWord || !mssqlRuntimeReserved[t.text] {
			continue
		}
		if at(toks, i-1).is("[") || !reservedAsIdentifier(toks, i) {
			continue
		}
		b.WriteString(q[last:t.start])
		b.WriteByte('[')
		b.WriteString(q[t.start:t.end])
		b.WriteByte(']')
		last = t.end
	}
	if last == 0 {
		return q
	}
	b.WriteString(q[last:])
	return b.String()
}

func containsMSSQLReserved(q string) bool {
	lower := strings.ToLower(q)
	for w := range mssqlRuntimeReserved {
		if strings.Contains(lower, w) {
			return true
		}
	}
	return false
}

// mssqlQuotable reports a statement whose grammar the quoter understands. DDL is left to
// the migration translator, whose positional rules know CREATE TABLE's shape.
func mssqlQuotable(toks []token) bool {
	if len(toks) == 0 || toks[0].kind != tokWord {
		return false
	}
	switch toks[0].text {
	case "select", "insert", "update", "delete", "with", "merge", "begin", "declare":
		return true
	}
	return false
}
