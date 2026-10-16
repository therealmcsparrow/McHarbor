// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"regexp"
	"strconv"
	"strings"
)

// This file is the SQL Server half of the migration translator. It exists for
// the same reason the Postgres and MySQL halves do (see dialect.go): the
// migration corpus is a historical record and editing a file changes what an
// already-migrated database believes it ran.
//
// SQL Server is the most divergent of the four. Three differences drive almost
// everything below:
//
//   - There is no `IF NOT EXISTS` on any DDL. Existence is a predicate, so a
//     `CREATE TABLE IF NOT EXISTS` becomes `IF OBJECT_ID(…) IS NULL CREATE …`
//     and an index relies on the executor's TolerateDuplicate instead.
//   - There is no upsert syntax. `ON CONFLICT … DO NOTHING` becomes a
//     NOT EXISTS predicate over a table value constructor, and
//     `DO UPDATE` becomes a MERGE.
//   - `NVARCHAR(MAX)` cannot be indexed, and the index key limit is 900 bytes.
//     Keyed text columns therefore become `NVARCHAR(450)` — 450 because
//     NVARCHAR stores two bytes per character.

// mssqlKeyedTextLength is the width a keyed TEXT column becomes on SQL Server.
// The index key limit is 900 bytes and NVARCHAR costs two bytes per character,
// so 450 is the widest single-column key the engine will accept.
const mssqlKeyedTextLength = 450

// rfc3339Now is the expression that reproduces SQLite's
// strftime('%Y-%m-%dT%H:%M:%fZ','now'). Style 126 is ISO8601 without the zone,
// which is why the Z is concatenated rather than formatted.
const rfc3339Now = "(CONVERT(nvarchar(23), SYSUTCDATETIME(), 126) + 'Z')"

var (
	// mssqlCreateTableIfNotExistsRE matches the DDL SQL Server cannot express,
	// capturing the table name so it can become an OBJECT_ID predicate.
	mssqlCreateTableIfNotExistsRE = regexp.MustCompile(`(?is)CREATE\s+TABLE\s+IF\s+NOT\s+EXISTS\s+"?([a-z_][a-z0-9_]*)"?`)
	// mssqlAnyLimitRE matches a LIMIT at any depth, which SQL Server spells as a
	// TOP on the owning SELECT. Only the bare numeric form appears in this corpus.
	mssqlAnyLimitRE = regexp.MustCompile(`(?i)\s+LIMIT\s+(\d+)\b`)
	// mssqlSelectHeadRE finds the SELECT a TOP has to be injected into.
	mssqlSelectHeadRE = regexp.MustCompile(`(?is)\bSELECT\s+(DISTINCT\s+)?`)
	// mssqlConcatRE matches SQLite's `||` string concatenation operator, which
	// SQL Server spells `+`. Matched with surrounding spaces so the `||`
	// inside a policy expression string literal is not touched — that one has
	// no spaces stripped either, so the literal guard in
	// replaceOutsideLiterals is what actually protects it.
	mssqlConcatRE = regexp.MustCompile(`\s\|\|\s`)
	// mssqlOnConflictNothingRE captures the conflict columns of a DO NOTHING.
	mssqlOnConflictNothingRE = regexp.MustCompile(`(?is)\s*ON\s+CONFLICT\s*\(\s*([^)]+?)\s*\)\s*DO\s+NOTHING\s*;?\s*$`)
	// mssqlOnConflictBareRE is the column-less form, which SQLite's
	// INSERT OR IGNORE translates into on Postgres.
	mssqlOnConflictBareRE = regexp.MustCompile(`(?is)\s*ON\s+CONFLICT\s+DO\s+NOTHING\s*;?\s*$`)
	// mssqlOnConflictUpdateRE captures the conflict columns and the SET body
	// of a real upsert.
	mssqlOnConflictUpdateRE = regexp.MustCompile(`(?is)\s*ON\s+CONFLICT\s*\(\s*([^)]+?)\s*\)\s*DO\s+UPDATE\s+SET\s+(.+?)\s*;?\s*$`)
	// mssqlInsertHeadRE splits an INSERT into its target table, its column
	// list and its body.
	mssqlInsertHeadRE = regexp.MustCompile(`(?is)^(.*?)INSERT\s+(?:OR\s+\w+\s+)?INTO\s+"?([a-z_][a-z0-9_]*)"?\s*\(\s*([^)]+?)\s*\)\s*(VALUES\b|SELECT\b)(.*)$`)
	// mssqlExcludedRE is the Postgres/SQLite name for the row being inserted;
	// a MERGE calls it the source alias.
	mssqlExcludedRE = regexp.MustCompile(`(?i)\bexcluded\.`)
	// mssqlDatetimeNowRE matches datetime('now') and datetime('now', mod).
	mssqlDatetimeNowRE = regexp.MustCompile(`(?i)\bdatetime\(\s*'now'\s*(?:,\s*'([^']*)'\s*)?\)`)
	// mssqlModifierRE reads a SQLite time modifier such as `-1 day`.
	mssqlModifierRE = regexp.MustCompile(`^\s*([+-]?\d+)\s+(second|minute|hour|day|month|year)s?\s*$`)
	// mssqlCastIntegerRE / mssqlCastTextRE match the two cast targets SQL
	// Server does not recognise under those names.
	mssqlCastIntegerRE = regexp.MustCompile(`(?is)CAST\s*\(\s*(.+?)\s+AS\s+INTEGER\s*\)`)
	mssqlCastTextRE    = regexp.MustCompile(`(?is)CAST\s*\(\s*(.+?)\s+AS\s+TEXT\s*\)`)
	// The column types. Each is anchored on the whitespace before it so a type
	// name inside an identifier or a literal is left alone.
	mssqlIntegerTypeRE = regexp.MustCompile(`(?i)(\s)INTEGER\b`)
	mssqlBooleanTypeRE = regexp.MustCompile(`(?i)(\s)BOOLEAN\b`)
	mssqlRealTypeRE    = regexp.MustCompile(`(?i)(\s)REAL\b`)
	mssqlBlobTypeRE    = regexp.MustCompile(`(?i)(\s)BLOB\b`)
	mssqlTextTypeRE    = regexp.MustCompile(`(?i)(\s)TEXT\b`)
	mssqlDatetimeTypeR = regexp.MustCompile(`(?i)(\s)DATETIME\b`)
	// mssqlCurrentTimestampDefaultRE matches the timestamp default the corpus
	// writes on a text column, in both spellings it uses.
	mssqlCurrentTimestampDefaultRE = regexp.MustCompile(`(?i)DEFAULT\s+(?:\(\s*CURRENT_TIMESTAMP\s*\)|CURRENT_TIMESTAMP)`)
	// mssqlGlobRE is SQLite's GLOB. SQL Server's LIKE understands character
	// classes, so the one pattern in the corpus translates exactly.
	mssqlGlobRE = regexp.MustCompile(`(?i)\bGLOB\s+'([^']*)'`)
	// mssqlJSONExtractRE / mssqlJSONExtractWholeRE are the two json_extract
	// shapes the schema uses.
	mssqlJSONExtractRE      = regexp.MustCompile(`(?i)json_extract\(\s*([^,]+?)\s*,\s*'\$\.([a-z0-9_]+)'\s*\)`)
	mssqlJSONExtractWholeRE = regexp.MustCompile(`(?i)json_extract\(\s*([^,]+?)\s*,\s*'\$'\s*\)`)
	// mssqlDropTableRE matches the DROP that closes a SQLite table rebuild.
	mssqlDropTableRE = regexp.MustCompile(`(?i)\bDROP\s+TABLE\s+(?:IF\s+EXISTS\s+)?("?[a-z_][a-z0-9_]*"?)\s*;`)
	// mssqlIdentityRE recognises the one DDL shape that needs IDENTITY.
	mssqlIdentityRE = regexp.MustCompile(`(?is)(\s)INTEGER\s+PRIMARY\s+KEY\s+AUTOINCREMENT\b`)
	// mssqlAddColumnRE is SQLite's `ADD COLUMN`, which T-SQL spells `ADD`.
	mssqlAddColumnRE = regexp.MustCompile(`(?i)(\s)ADD\s+COLUMN\s+`)
	// mssqlCreateIndexIfNotExistsRE captures the index and table names so the
	// missing IF NOT EXISTS can become a sys.indexes predicate.
	mssqlCreateIndexIfNotExistsRE = regexp.MustCompile(`(?is)CREATE\s+(UNIQUE\s+)?INDEX\s+IF\s+NOT\s+EXISTS\s+"?([a-z_][a-z0-9_]*)"?\s+ON\s+"?([a-z_][a-z0-9_]*)"?`)
	// mssqlDropColumnRE captures the table and column of a DROP COLUMN.
	mssqlDropColumnRE = regexp.MustCompile(`(?i)\bALTER\s+TABLE\s+"?([a-z_][a-z0-9_]*)"?\s+DROP\s+COLUMN\s+"?([a-z_][a-z0-9_]*)"?\s*;?`)
	// mssqlBareCurrentTimestampRE is CURRENT_TIMESTAMP used as a value.
	mssqlBareCurrentTimestampRE = regexp.MustCompile(`(?i)\bCURRENT_TIMESTAMP\b`)
	// mssqlDerivedTableRE finds a subquery in a FROM or JOIN, which T-SQL
	// requires to carry an alias.
	mssqlDerivedTableRE = regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s*\(\s*SELECT\b`)
	// mssqlLengthRE is SQLite's length(), which T-SQL spells LEN.
	mssqlLengthRE = regexp.MustCompile(`(?i)\blength\s*\(`)
	// mssqlRenameTableRE is the rename that closes every SQLite table rebuild; T-SQL
	// renames through sp_rename, which keeps foreign keys pointing at the object.
	mssqlRenameTableRE = regexp.MustCompile(`(?is)\bALTER\s+TABLE\s+"?([a-z_][a-z0-9_]*)"?\s+RENAME\s+TO\s+"?([a-z_][a-z0-9_]*)"?\s*;?`)
)

// classifyMSSQL returns the action for one statement on SQL Server, or
// ExecuteStatement when nothing special applies. It is called from
// ClassifyStatement after the SQLite-only constructs have already been handled.
func classifyMSSQL(stmt string) StatementAction {
	switch {
	case expressionIndexRE.MatchString(stmt):
		// SQL Server indexes a computed column, not an expression. The one
		// index over COALESCE(...) in this corpus loses its uniqueness here,
		// exactly as it does on MySQL.
		return SkipStatementLoudly
	case createIndexRE.MatchString(stmt), mssqlUniqueIndexOnMaxColumn(stmt):
		// No CREATE INDEX IF NOT EXISTS; the duplicate error stands in for it.
		return TolerateDuplicate
	case dropIndexIfExistsRE.MatchString(stmt):
		// Nor a DROP INDEX IF EXISTS on every supported version.
		return TolerateMissing
	case insertOrIgnoreRE.MatchString(stmt) && !mssqlRewritableConflict(stmt):
		// An INSERT OR IGNORE whose conflict key cannot be recovered — the
		// SELECT-driven form in 080_api_keys.sql — falls back to swallowing
		// the duplicate-key error. On a re-run every row already exists, so
		// the skipped insert reaches the same end state.
		return TolerateDuplicate
	}
	return ExecuteStatement
}

// mssqlUniqueIndexOnMaxColumn is a placeholder for the case an index names a
// column the type rewrite left as NVARCHAR(MAX). It reports false today: the
// keyed-column index catches every indexed column in the corpus, so nothing
// reaches this. It stays so a migration that adds one is tolerated rather than
// failing the whole file.
func mssqlUniqueIndexOnMaxColumn(string) bool { return false }

// translateMSSQL rewrites one migration statement into T-SQL.
func translateMSSQL(stmt string) string {
	out := stmt

	// Conflict handling runs first, on the statement as written: every rewrite
	// below changes the text the INSERT parser would have to match.
	out = mssqlRewriteConflicts(out)

	// The timestamp default has to become an expression before the TEXT type
	// rewrite, because the type it lands on decides which default is legal.
	out = mssqlCurrentTimestampDefaultRE.ReplaceAllString(out, "DEFAULT "+rfc3339Now)

	out = rewriteKeyedTextAs(out, "NVARCHAR("+strconv.Itoa(mssqlKeyedTextLength)+")")

	// The datetime() and strftime() *scalars* must be rewritten before the
	// DATETIME *type*, which shares its spelling: with the type rewrite first,
	// `datetime('now')` became `DATETIME2(3)('now')` — valid-looking text that
	// no server accepts and that the corpus-wide sweep does not recognise.
	out = mssqlDatetimeNowRE.ReplaceAllStringFunc(out, func(match string) string {
		groups := mssqlDatetimeNowRE.FindStringSubmatch(match)
		if len(groups) < 2 || strings.TrimSpace(groups[1]) == "" {
			return rfc3339Now
		}
		mod := mssqlModifierRE.FindStringSubmatch(groups[1])
		if mod == nil {
			// An unrecognised modifier is left as written so the server
			// refuses it loudly rather than silently returning the wrong time.
			return match
		}
		return "(CONVERT(nvarchar(23), DATEADD(" + strings.ToLower(mod[2]) +
			", " + mod[1] + ", SYSUTCDATETIME()), 126) + 'Z')"
	})
	out = strftimeNowRE.ReplaceAllString(out, rfc3339Now)

	// Types. IDENTITY first: it consumes the INTEGER the generic rewrite would
	// otherwise take.
	out = mssqlIdentityRE.ReplaceAllString(out, "${1}BIGINT IDENTITY(1,1) PRIMARY KEY")
	out = mssqlDatetimeTypeR.ReplaceAllString(out, "${1}DATETIME2(3)")
	// BOOLEAN becomes BIGINT rather than BIT deliberately. SQLite has no
	// boolean type and the application stores and scans every flag as an
	// integer; a BIT column would hand database/sql a Go bool, which will not
	// scan into an int.
	out = mssqlBooleanTypeRE.ReplaceAllString(out, "${1}BIGINT")
	out = mssqlIntegerTypeRE.ReplaceAllString(out, "${1}BIGINT")
	out = mssqlRealTypeRE.ReplaceAllString(out, "${1}FLOAT")
	out = mssqlBlobTypeRE.ReplaceAllString(out, "${1}VARBINARY(MAX)")
	out = mssqlTextTypeRE.ReplaceAllString(out, "${1}NVARCHAR(MAX)")

	out = autoincrementRE.ReplaceAllString(out, "")
	// The default collation of a SQL Server database is case-insensitive, so
	// NOCASE is already the behaviour and the clause is simply unknown syntax.
	out = collateNocaseRE.ReplaceAllString(out, "")

	out = addGeneratedIDDefault(out, "(LOWER(REPLACE(CONVERT(nvarchar(36), NEWID()), '-', '')))")

	// DDL that has no IF NOT EXISTS form.
	out = mssqlCreateTableIfNotExistsRE.ReplaceAllStringFunc(out, func(match string) string {
		groups := mssqlCreateTableIfNotExistsRE.FindStringSubmatch(match)
		if len(groups) < 2 {
			return match
		}
		table := strings.Trim(groups[1], `"`)
		return "IF OBJECT_ID(N'" + table + "', N'U') IS NULL CREATE TABLE " + table
	})
	// The existence check has to be a predicate rather than a swallowed error:
	// error 1913 aborts the migration's transaction, so the duplicate never gets
	// the chance to be tolerated.
	out = mssqlCreateIndexIfNotExistsRE.ReplaceAllString(out,
		"IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'$2' AND object_id = OBJECT_ID(N'$3')) CREATE ${1}INDEX $2 ON $3")
	out = createIndexBodyRE.ReplaceAllString(out, "CREATE ${1}INDEX ")
	// CREATE OR ALTER VIEW has existed since 2016 SP1 and is the only
	// idempotent form; CREATE VIEW IF NOT EXISTS is SQLite-only.
	out = createViewRE.ReplaceAllString(out, "CREATE OR ALTER VIEW ")

	// A DROP INDEX must name its table on SQL Server, as on MySQL.
	out = dropIndexBodyRE.ReplaceAllStringFunc(out, func(match string) string {
		groups := dropIndexBodyRE.FindStringSubmatch(match)
		if len(groups) < 2 {
			return match
		}
		table, ok := indexTableIndex()[strings.ToLower(groups[1])]
		if !ok {
			return match
		}
		// IF EXISTS (2016+) for the same reason as the CREATE: a missing index
		// raised inside the transaction would abort it.
		return "DROP INDEX IF EXISTS " + groups[1] + " ON " + table + ";"
	})

	// A column cannot be dropped while a DEFAULT or CHECK constraint names it,
	// and both are system-named, so they are found and dropped first.
	out = mssqlDropColumnRE.ReplaceAllStringFunc(out, func(match string) string {
		groups := mssqlDropColumnRE.FindStringSubmatch(match)
		if len(groups) < 3 {
			return match
		}
		return mssqlDropColumnConstraints(groups[1], groups[2]) + " " + match
	})
	out = replaceOutsideLiterals(out, mssqlLengthRE, "LEN(")

	// The DROP that closes a table rebuild. SQL Server has no DROP TABLE
	// CASCADE and refuses to drop a table another table's foreign key still
	// references, so the referencing constraints come off first. Emitted as a
	// dynamic block because the constraint names are generated and unknowable
	// from the migration text.
	out = mssqlDropTableRE.ReplaceAllStringFunc(out, func(match string) string {
		groups := mssqlDropTableRE.FindStringSubmatch(match)
		if len(groups) < 2 {
			return match
		}
		return mssqlDropReferencingKeys(strings.Trim(groups[1], `"`)) + " " + match
	})

	// Scalar functions.
	out = randomblobRE.ReplaceAllStringFunc(out, func(match string) string {
		groups := randomblobRE.FindStringSubmatch(match)
		if len(groups) < 2 {
			return match
		}
		n, err := strconv.Atoi(groups[1])
		if err != nil || n <= 0 || n > 16 {
			return match
		}
		// Style 2 renders varbinary as hex with no 0x prefix, which is what
		// hex() produces.
		return "CONVERT(nvarchar(" + strconv.Itoa(n*2) + "), CRYPT_GEN_RANDOM(" +
			strconv.Itoa(n) + "), 2)"
	})
	// rowid has no equivalent; every use in the corpus mints a unique suffix.
	out = rowidRE.ReplaceAllString(out, "CONVERT(nvarchar(36), NEWID())")
	out = translateJSONMSSQL(out)
	out = mssqlJSONExtractWholeRE.ReplaceAllString(out, "$1")
	out = mssqlJSONExtractRE.ReplaceAllString(out, "JSON_VALUE($1, '$.$2')")
	out = mssqlCastIntegerRE.ReplaceAllString(out, "CAST(${1} AS BIGINT)")
	out = mssqlCastTextRE.ReplaceAllString(out, "CAST(${1} AS NVARCHAR(MAX))")
	// SQL Server's LIKE understands character classes, so GLOB '[0-9]*'
	// translates exactly rather than approximately.
	out = mssqlGlobRE.ReplaceAllStringFunc(out, func(match string) string {
		groups := mssqlGlobRE.FindStringSubmatch(match)
		if len(groups) < 2 || !strings.HasSuffix(groups[1], "*") {
			return match
		}
		prefix := strings.TrimSuffix(groups[1], "*")
		if strings.ContainsAny(prefix, "*?") {
			return match
		}
		return "LIKE '" + prefix + "%'"
	})

	// T-SQL spells ADD COLUMN as a bare ADD, and has no RENAME TO at all. Both run
	// late: the keyed-text rewrite above reads the ADD COLUMN spelling.
	out = replaceOutsideLiterals(out, mssqlAddColumnRE, "${1}ADD ")
	out = mssqlRenameTableRE.ReplaceAllString(out, "EXEC sp_rename N'$1', N'$2';")

	out = replaceOutsideLiterals(out, mssqlConcatRE, " + ")
	out = mssqlRewriteLimit(out)
	out = mssqlAliasDerivedTables(out)
	// A CURRENT_TIMESTAMP left after the DEFAULT rewrite is a value written into a
	// text column, where SQL Server's datetime would be stored as "Sep 27 2026
	// 1:00PM" rather than an ISO stamp that sorts against its neighbours.
	out = replaceOutsideLiterals(out, mssqlBareCurrentTimestampRE, rfc3339Now)
	out = fixNotNullSetNull(out)
	out = quoteMSSQLReservedWords(out)

	return out
}

// mssqlDropReferencingKeys builds the dynamic block that drops every foreign
// key pointing at table, so the DROP TABLE that follows is accepted.
func mssqlDropReferencingKeys(table string) string {
	return "DECLARE @drop_fk NVARCHAR(MAX) = N''; " +
		"SELECT @drop_fk += N'ALTER TABLE ' + QUOTENAME(OBJECT_SCHEMA_NAME(fk.parent_object_id)) + N'.' + " +
		"QUOTENAME(OBJECT_NAME(fk.parent_object_id)) + N' DROP CONSTRAINT ' + QUOTENAME(fk.name) + N'; ' " +
		"FROM sys.foreign_keys fk WHERE fk.referenced_object_id = OBJECT_ID(N'" + table + "'); " +
		"IF @drop_fk <> N'' EXEC sp_executesql @drop_fk;"
}

// mssqlAliasDerivedTables names every unnamed subquery in a FROM or JOIN.
// SQLite accepts `FROM (SELECT … UNION SELECT …)` bare; SQL Server refuses it
// with a syntax error at the closing parenthesis.
func mssqlAliasDerivedTables(stmt string) string {
	masked, comments := maskComments(stmt)
	locs := mssqlDerivedTableRE.FindAllStringIndex(masked, -1)
	out := masked
	for n := len(locs) - 1; n >= 0; n-- {
		open := strings.IndexByte(masked[locs[n][0]:], '(') + locs[n][0]
		closeAt := matchingParen(masked, open)
		if closeAt < 0 {
			continue
		}
		rest := strings.TrimLeft(masked[closeAt+1:], " \t\r\n")
		word := strings.ToUpper(rest[:len(rest)-len(strings.TrimLeft(rest, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_[\""))])
		if word != "" && !mssqlNotAlias[strings.Trim(word, `["`)] {
			continue
		}
		out = out[:closeAt+1] + " AS mch_d" + strconv.Itoa(n+1) + out[closeAt+1:]
	}
	return restoreComments(out, comments)
}

// mssqlNotAlias lists the words that can follow a derived table's closing
// parenthesis without being its alias.
var mssqlNotAlias = map[string]bool{
	"WHERE": true, "GROUP": true, "ORDER": true, "UNION": true, "ON": true, "JOIN": true,
	"INNER": true, "LEFT": true, "RIGHT": true, "FULL": true, "CROSS": true, "HAVING": true,
	"EXCEPT": true, "INTERSECT": true, "LIMIT": true, "OFFSET": true, "AND": true, "OR": true,
}

// mssqlDropColumnConstraints builds the dynamic block that drops every DEFAULT
// and CHECK constraint on table.column, which SQLite never names and SQL Server
// refuses to drop the column past.
func mssqlDropColumnConstraints(table, column string) string {
	return "DECLARE @drop_cc NVARCHAR(MAX) = N''; " +
		"SELECT @drop_cc += N'ALTER TABLE ' + QUOTENAME(OBJECT_NAME(k.parent_object_id)) + " +
		"N' DROP CONSTRAINT ' + QUOTENAME(k.name) + N'; ' FROM (" +
		"SELECT name, parent_object_id, parent_column_id FROM sys.default_constraints UNION ALL " +
		"SELECT name, parent_object_id, parent_column_id FROM sys.check_constraints) k " +
		"JOIN sys.columns c ON c.object_id = k.parent_object_id AND c.column_id = k.parent_column_id " +
		"WHERE k.parent_object_id = OBJECT_ID(N'" + table + "') AND c.name = N'" + column + "'; " +
		"IF @drop_cc <> N'' EXEC sp_executesql @drop_cc;"
}

// mssqlRewriteLimit turns every `LIMIT n` into a `TOP (n)` on the SELECT that
// owns it, at whatever nesting depth: four of the eight LIMITs in the corpus sit
// in a subquery, and one in an INSERT … SELECT the conflict rewrite has wrapped.
//
// The OFFSET/FETCH form would be the literal equivalent, but it requires an
// ORDER BY and several of the LIMIT 1 subqueries in this corpus have none. TOP
// has no such requirement and is what the corpus's uses actually mean.
func mssqlRewriteLimit(stmt string) string {
	if strings.Contains(strings.ToUpper(stmt), " OFFSET ") {
		// A LIMIT with an OFFSET is a paging query, not a scalar subquery, and
		// needs the FETCH form with its ORDER BY. None appears in this corpus;
		// leaving it alone makes the server refuse it rather than silently
		// dropping the offset.
		return stmt
	}
	masked, comments := maskComments(stmt)
	out := masked
	for {
		loc := mssqlAnyLimitRE.FindStringSubmatchIndex(out)
		if loc == nil {
			break
		}
		head := mssqlOwningSelect(out, loc[0])
		if head < 0 {
			return stmt
		}
		out = out[:head] + "TOP (" + out[loc[2]:loc[3]] + ") " + out[head:loc[0]] + out[loc[1]:]
	}
	return restoreComments(out, comments)
}

// mssqlOwningSelect walks back from pos to the SELECT at the same parenthesis
// depth and returns the offset just past its head (and any DISTINCT), where a
// TOP belongs, or -1 when the LIMIT has no SELECT of its own.
func mssqlOwningSelect(s string, pos int) int {
	depth := 0
	quoted := false
	for i := pos - 1; i >= 0; i-- {
		switch c := s[i]; {
		case c == '\'':
			quoted = !quoted
		case quoted:
		case c == ')':
			depth++
		case c == '(':
			if depth == 0 {
				return -1
			}
			depth--
		case depth == 0 && (c == 's' || c == 'S') && (i == 0 || !isIdentByte(s[i-1])):
			if loc := mssqlSelectHeadRE.FindStringIndex(s[i:]); loc != nil && loc[0] == 0 {
				return i + loc[1]
			}
		}
	}
	return -1
}

// isIdentByte reports a byte that can continue an unquoted identifier.
func isIdentByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// mssqlRewritableConflict reports whether the statement's conflict key can be
// recovered from its text, which is what decides between a NOT EXISTS rewrite
// and falling back to the executor's duplicate tolerance.
func mssqlRewritableConflict(stmt string) bool {
	if mssqlOnConflictNothingRE.MatchString(stmt) || mssqlOnConflictUpdateRE.MatchString(stmt) {
		return true
	}
	m := mssqlInsertHeadRE.FindStringSubmatch(stmt)
	if m == nil || !strings.EqualFold(strings.TrimSpace(m[4]), "VALUES") {
		return false
	}
	_, ok := mssqlConflictKeyFor(m[2])
	return ok
}

// migrationConflictTargets names the key each `INSERT OR IGNORE` in the
// migration corpus actually resolves against, read out of the corresponding
// CREATE TABLE rather than assumed to be `id`.
//
// `settings` is why this map exists and is not simply "the primary key": its
// seed rows mint a random `lower(hex(randomblob(8)))` id, so resolving on `id`
// would insert a duplicate setting on every migration run.
var migrationConflictTargets = map[string][]string{
	"settings":                   {"key"},
	"audit_action_catalog":       {"action"},
	"audit_action_route_map":     {"method", "path", "handler"},
	"alert_destinations":         {"alert_id", "channel_id"},
	"api_key_roles":              {"api_key_id", "role_id"},
	"scanner_configs":            {"id"},
	"plugin_kubernetes_clusters": {"id"},
	"groups":                     {"id"},
	"users":                      {"id"},
	"roles":                      {"id"},
}

// mssqlConflictKeyFor returns the key an INSERT OR IGNORE on table resolves
// against, or false when the corpus does not record one — in which case the
// caller falls back to the executor's duplicate tolerance rather than guessing.
func mssqlConflictKeyFor(table string) ([]string, bool) {
	cols, ok := migrationConflictTargets[strings.ToLower(table)]
	return cols, ok
}

// mssqlRewriteConflicts turns the three upsert shapes the corpus uses into the
// T-SQL that has the same effect.
func mssqlRewriteConflicts(stmt string) string {
	if m := mssqlOnConflictUpdateRE.FindStringSubmatch(stmt); m != nil {
		body := mssqlOnConflictUpdateRE.ReplaceAllString(stmt, "")
		if merged, ok := mssqlMerge(body, splitColumnList(m[1]), m[2]); ok {
			return merged
		}
		// A shape the MERGE builder cannot read keeps its clause, so the
		// server refuses it rather than silently losing the update half.
		return stmt
	}
	if m := mssqlOnConflictNothingRE.FindStringSubmatch(stmt); m != nil {
		body := mssqlOnConflictNothingRE.ReplaceAllString(stmt, ";")
		if guarded, ok := mssqlInsertWhereNotExists(body, splitColumnList(m[1])); ok {
			return guarded
		}
		return body
	}
	if mssqlOnConflictBareRE.MatchString(stmt) {
		body := mssqlOnConflictBareRE.ReplaceAllString(stmt, ";")
		if m := mssqlInsertHeadRE.FindStringSubmatch(body); m != nil {
			if cols, ok := mssqlConflictKeyFor(m[2]); ok {
				if guarded, ok := mssqlInsertWhereNotExists(body, cols); ok {
					return guarded
				}
			}
		}
		return body
	}
	if insertOrIgnoreRE.MatchString(stmt) {
		body := insertOrIgnoreRE.ReplaceAllString(stmt, "INSERT INTO")
		if m := mssqlInsertHeadRE.FindStringSubmatch(body); m != nil &&
			strings.EqualFold(strings.TrimSpace(m[4]), "VALUES") {
			if cols, ok := mssqlConflictKeyFor(m[2]); ok {
				if guarded, ok := mssqlInsertWhereNotExists(body, cols); ok {
					return guarded
				}
			}
		}
		return body
	}
	if insertOrReplaceRE.MatchString(stmt) {
		// INSERT OR REPLACE deletes the conflicting row and inserts. The one
		// use in this corpus is a settings seed keyed on id, so a delete
		// followed by the insert is the same thing and needs no MERGE.
		body := insertOrReplaceRE.ReplaceAllString(stmt, "INSERT INTO")
		if m := mssqlInsertHeadRE.FindStringSubmatch(body); m != nil {
			if cols, ok := mssqlConflictKeyFor(m[2]); ok {
				if deleted, ok := mssqlDeleteThenInsert(body, m[2], m[3], cols); ok {
					return deleted
				}
			}
		}
		return body
	}
	return stmt
}

// mssqlInsertWhereNotExists guards an INSERT with a NOT EXISTS over its own
// source rows, which is the T-SQL equivalent of DO NOTHING and, unlike
// swallowing a duplicate-key error, still inserts the rows that are new.
func mssqlInsertWhereNotExists(stmt string, keyCols []string) (string, bool) {
	m := mssqlInsertHeadRE.FindStringSubmatch(stmt)
	if m == nil || len(keyCols) == 0 {
		return stmt, false
	}
	lead, table, colList, kind, rest := m[1], m[2], m[3], strings.ToUpper(strings.TrimSpace(m[4])), m[5]
	cols := splitColumnList(colList)
	if !containsAll(cols, keyCols) {
		return stmt, false
	}
	predicate := make([]string, 0, len(keyCols))
	for _, key := range keyCols {
		predicate = append(predicate, "tgt."+bracket(key)+" = src."+bracket(key))
	}
	where := " WHERE NOT EXISTS (SELECT 1 FROM " + bracket(table) + " AS tgt WHERE " +
		strings.Join(predicate, " AND ") + ")"

	bracketed := make([]string, 0, len(cols))
	for _, c := range cols {
		bracketed = append(bracketed, bracket(c))
	}
	selectList := make([]string, 0, len(cols))
	for _, c := range cols {
		selectList = append(selectList, "src."+bracket(c))
	}

	body := strings.TrimRight(strings.TrimSpace(rest), "; \n\t")
	head := lead + "INSERT INTO " + bracket(table) + " (" + strings.Join(bracketed, ", ") + ")\n"

	if kind == "VALUES" {
		return head + "SELECT " + strings.Join(selectList, ", ") +
			" FROM (VALUES " + body + ") AS src (" + strings.Join(bracketed, ", ") + ")" +
			where + ";", true
	}
	// A SELECT-driven insert: wrap it so the guard can name its columns.
	return head + "SELECT " + strings.Join(selectList, ", ") +
		" FROM (SELECT " + body + ") AS src (" + strings.Join(bracketed, ", ") + ")" +
		where + ";", true
}

// mssqlDeleteThenInsert reproduces INSERT OR REPLACE: remove the conflicting
// rows, then insert. Only the VALUES form is handled, which is the only one in
// the corpus.
func mssqlDeleteThenInsert(stmt, table, colList string, keyCols []string) (string, bool) {
	m := mssqlInsertHeadRE.FindStringSubmatch(stmt)
	if m == nil || !strings.EqualFold(strings.TrimSpace(m[4]), "VALUES") {
		return stmt, false
	}
	cols := splitColumnList(colList)
	if !containsAll(cols, keyCols) {
		return stmt, false
	}
	bracketed := make([]string, 0, len(cols))
	for _, c := range cols {
		bracketed = append(bracketed, bracket(c))
	}
	predicate := make([]string, 0, len(keyCols))
	for _, key := range keyCols {
		predicate = append(predicate, "tgt."+bracket(key)+" = src."+bracket(key))
	}
	body := strings.TrimRight(strings.TrimSpace(m[5]), "; \n\t")
	del := "DELETE tgt FROM " + bracket(table) + " AS tgt INNER JOIN (VALUES " + body +
		") AS src (" + strings.Join(bracketed, ", ") + ") ON " + strings.Join(predicate, " AND ") + ";"
	ins := "INSERT INTO " + bracket(table) + " (" + strings.Join(bracketed, ", ") + ") VALUES " + body + ";"
	return m[1] + del + " " + ins, true
}

// mssqlMerge builds the MERGE that stands in for ON CONFLICT … DO UPDATE.
func mssqlMerge(stmt string, keyCols []string, setBody string) (string, bool) {
	m := mssqlInsertHeadRE.FindStringSubmatch(stmt)
	if m == nil || len(keyCols) == 0 || !strings.EqualFold(strings.TrimSpace(m[4]), "VALUES") {
		return stmt, false
	}
	lead, table, colList, rest := m[1], m[2], m[3], m[5]
	cols := splitColumnList(colList)
	if !containsAll(cols, keyCols) {
		return stmt, false
	}
	bracketed := make([]string, 0, len(cols))
	insertValues := make([]string, 0, len(cols))
	for _, c := range cols {
		bracketed = append(bracketed, bracket(c))
		insertValues = append(insertValues, "src."+bracket(c))
	}
	on := make([]string, 0, len(keyCols))
	for _, key := range keyCols {
		on = append(on, "tgt."+bracket(key)+" = src."+bracket(key))
	}
	body := strings.TrimRight(strings.TrimSpace(rest), "; \n\t")
	set := mssqlExcludedRE.ReplaceAllString(setBody, "src.")

	return lead + "MERGE " + bracket(table) + " AS tgt\n" +
		"USING (VALUES " + body + ") AS src (" + strings.Join(bracketed, ", ") + ")\n" +
		"ON " + strings.Join(on, " AND ") + "\n" +
		"WHEN MATCHED THEN UPDATE SET " + strings.TrimSpace(set) + "\n" +
		"WHEN NOT MATCHED THEN INSERT (" + strings.Join(bracketed, ", ") + ")" +
		" VALUES (" + strings.Join(insertValues, ", ") + ");", true
}

// splitColumnList splits a comma-separated column list and strips quoting and
// any ASC/DESC an index would carry.
func splitColumnList(list string) []string {
	parts := strings.Split(list, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		col := strings.TrimSpace(part)
		col = strings.TrimSuffix(strings.TrimSuffix(col, " DESC"), " ASC")
		col = strings.Trim(strings.TrimSpace(col), `"[]`)
		if col != "" {
			out = append(out, col)
		}
	}
	return out
}

// containsAll reports whether every want is present in have, case-insensitively.
func containsAll(have, want []string) bool {
	set := make(map[string]bool, len(have))
	for _, h := range have {
		set[strings.ToLower(h)] = true
	}
	for _, w := range want {
		if !set[strings.ToLower(w)] {
			return false
		}
	}
	return true
}

// bracket quotes an identifier the way SQL Server does. Already-bracketed
// names are returned unchanged so a second pass is a no-op.
func bracket(name string) string {
	name = strings.Trim(name, `"[]`)
	return "[" + name + "]"
}

// mssqlReservedWords are the words this schema uses as identifiers that SQL
// Server reserves. It is the MySQL list minus the words SQL Server does not
// reserve, plus the ones only it does — derived from the T-SQL reserved-word
// list rather than assumed to match MySQL's.
var mssqlReservedWords = []string{
	"key", "system", "trigger", "order", "read", "user", "public", "current",
	"file", "plan", "rule", "schema", "session", "size", "statistics", "work",
}

// mssqlReservedContexts mirrors reservedWordContexts: a reserved word is only
// an identifier in a few syntactic positions, and matching those is both
// shorter and more correct than quoting every occurrence.
var mssqlReservedContexts = func() map[string][]*regexp.Regexp {
	types := `(?:NVARCHAR\((?:MAX|[0-9]+)\)|VARBINARY\(MAX\)|BIGINT|FLOAT|DATETIME2\([0-9]+\)|DATETIME2|NUMERIC|BIT)`
	out := make(map[string][]*regexp.Regexp, len(mssqlReservedWords))
	for _, word := range mssqlReservedWords {
		out[word] = []*regexp.Regexp{
			regexp.MustCompile(`(?i)([\n(,]\s*)(` + word + `)(\s+` + types + `[\s,)])`),
			regexp.MustCompile(`(?i)(\bREFERENCES\s+)(` + word + `)(\s*\()`),
			regexp.MustCompile(`(?i)([(,]\s*)(` + word + `)(\s*[,)])`),
			regexp.MustCompile(`(?i)(\b(?:SET|WHERE|AND|OR|ON)\s+)(` + word + `)(\s*(?:[=<>!]|\b(?:IN|LIKE|IS|NOT|BETWEEN)\b))`),
			regexp.MustCompile(`(?i)(\.)(` + word + `)(\s*[,)\s=])`),
			regexp.MustCompile(`(?i)(\b(?:CREATE\s+TABLE\s+|INSERT\s+INTO\s+|DELETE\s+FROM\s+|FROM\s+|JOIN\s+|UPDATE\s+|TABLE\s+|ON\s+)\s*)(` + word + `)(\s*[,;(\s])`),
		}
	}
	return out
}()

// quoteMSSQLReservedWords brackets reserved words used as identifiers, leaving
// genuine keywords and string literals alone.
func quoteMSSQLReservedWords(stmt string) string {
	masked, comments := maskComments(stmt)
	segments := strings.Split(masked, "'")
	for i := range segments {
		if i%2 == 1 {
			continue
		}
		for word, patterns := range mssqlReservedContexts {
			for _, re := range patterns {
				segments[i] = re.ReplaceAllString(segments[i], "${1}["+word+"]${3}")
			}
		}
	}
	return restoreComments(strings.Join(segments, "'"), comments)
}

// replaceOutsideLiterals applies re to the parts of stmt that are not inside a
// single-quoted string. The `||` in 067_rbac.sql's policy expression is inside
// one, and rewriting it to `+` would change a stored policy.
func replaceOutsideLiterals(stmt string, re *regexp.Regexp, replacement string) string {
	masked, comments := maskComments(stmt)
	segments := strings.Split(masked, "'")
	for i := range segments {
		if i%2 == 1 {
			continue
		}
		segments[i] = re.ReplaceAllString(segments[i], replacement)
	}
	return restoreComments(strings.Join(segments, "'"), comments)
}

// rewriteKeyedTextAs is rewriteKeyedText with the target type supplied, so
// MySQL's VARCHAR(191) and SQL Server's NVARCHAR(450) share one implementation.
func rewriteKeyedTextAs(stmt, targetType string) string {
	upper := strings.ToUpper(stmt)
	if !strings.Contains(upper, "CREATE TABLE") && !strings.Contains(upper, "ALTER TABLE") {
		return stmt
	}
	code := stripComments(stmt)
	table := ""
	if m := createTableNameRE.FindStringSubmatch(code); m != nil {
		table = strings.ToLower(m[1])
	} else if m := alterTableNameRE.FindStringSubmatch(code); m != nil {
		table = strings.ToLower(m[1])
	}
	if table == "" {
		return stmt
	}
	table = canonicalTableName(table)
	corpus := keyedColumnIndex()
	local := keyedColumnNames(stmt)

	return keyedTextRE.ReplaceAllStringFunc(stmt, func(match string) string {
		groups := keyedTextRE.FindStringSubmatch(match)
		if len(groups) < 3 {
			return match
		}
		name := strings.ToLower(strings.Trim(groups[2], `"`))
		if !local[name] && !corpus[table+"."+name] {
			return match
		}
		return groups[1] + groups[2] + " " + targetType
	})
}
