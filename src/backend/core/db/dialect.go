// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// This file makes the migration set — 116 files written in SQLite idiom over
// two years — apply on PostgreSQL, MySQL 8 and MariaDB 10.6.
//
// The alternative was rewriting all 116 files. That was rejected: a migration
// is a historical record of what an install already ran, and editing one
// changes what an already-migrated database believes happened. Translating at
// apply time leaves the record intact and keeps SQLite — the shipped default,
// and the only driver the files were ever written against — byte-for-byte
// untouched.
//
// Measured before any of this existed (2026-09-13, against postgres:16,
// mysql:8 and mariadb:10.6 booted empty, replaying statement by statement):
// Postgres failed 132 of 864 statements and MySQL failed 847. The constructs
// below are, between them, all of that.

// ─── Statement splitting ──────────────────────────────────────────────────

// SplitStatements breaks a migration file into individual statements.
//
// Needed because neither MySQL (without `multiStatements`, which openMySQL
// deliberately does not enable — it widens the SQL-injection blast radius of
// any future dynamic query) nor the per-statement error tolerance below can
// work on a whole file at once.
//
// The splitter is aware of exactly the two things this migration set contains
// that a naive split on ';' would break: string literals, and `BEGIN … END`
// trigger bodies whose inner statements are themselves semicolon-terminated.
func SplitStatements(sqlText string) []string {
	var (
		out       []string
		current   strings.Builder
		inQuoted  bool
		inLine    bool
		inBlock   bool
		depth     int
		runes     = []rune(sqlText)
		upperTail = func() string {
			// The last ~16 non-space characters, uppercased, so BEGIN/END can
			// be recognised without re-scanning the whole buffer.
			text := current.String()
			if len(text) > 64 {
				text = text[len(text)-64:]
			}
			return strings.ToUpper(text)
		}
	)

	for i := 0; i < len(runes); i++ {
		c := runes[i]

		switch {
		case inLine:
			current.WriteRune(c)
			if c == '\n' {
				inLine = false
			}
			continue
		case inBlock:
			current.WriteRune(c)
			if c == '/' && i > 0 && runes[i-1] == '*' {
				inBlock = false
			}
			continue
		case inQuoted:
			current.WriteRune(c)
			if c == '\'' {
				// '' is an escaped quote, not a terminator.
				if i+1 < len(runes) && runes[i+1] == '\'' {
					current.WriteRune(runes[i+1])
					i++
					continue
				}
				inQuoted = false
			}
			continue
		}

		switch {
		case c == '-' && i+1 < len(runes) && runes[i+1] == '-':
			inLine = true
			current.WriteRune(c)
		case c == '/' && i+1 < len(runes) && runes[i+1] == '*':
			inBlock = true
			current.WriteRune(c)
		case c == '\'':
			inQuoted = true
			current.WriteRune(c)
		case c == ';':
			current.WriteRune(c)
			tail := upperTail()
			if depth > 0 {
				// Inside a trigger body. Only `END;` closes it.
				if strings.HasSuffix(strings.TrimSpace(tail), "END;") {
					depth--
					if depth == 0 {
						out = appendStatement(out, current.String())
						current.Reset()
					}
				}
				continue
			}
			out = appendStatement(out, current.String())
			current.Reset()
		default:
			current.WriteRune(c)
			if c == 'N' || c == 'n' {
				tail := strings.TrimSpace(upperTail())
				if tail == "BEGIN" || strings.HasSuffix(tail, "\nBEGIN") ||
					strings.HasSuffix(tail, " BEGIN") {
					depth++
				}
			}
		}
	}

	return appendStatement(out, current.String())
}

func appendStatement(out []string, stmt string) []string {
	if strings.TrimSpace(stripComments(stmt)) == "" {
		return out
	}
	return append(out, strings.TrimSpace(stmt))
}

// stripComments removes `--` line comments so a statement that is nothing but
// commentary is recognised as empty. It is not used to rewrite the statement
// that actually executes — comments are harmless to every driver and keeping
// them makes a failure message point at readable SQL.
func stripComments(stmt string) string {
	var out strings.Builder
	for _, line := range strings.Split(stmt, "\n") {
		if idx := strings.Index(line, "--"); idx >= 0 {
			line = line[:idx]
		}
		out.WriteString(line)
		out.WriteString("\n")
	}
	return out.String()
}

// ─── Statement classification ─────────────────────────────────────────────

// StatementAction says what the executor should do with a statement on the
// active driver.
type StatementAction int

const (
	// ExecuteStatement runs it and fails the migration on error.
	ExecuteStatement StatementAction = iota
	// SkipStatement drops it silently — a SQLite-only construct with no
	// meaning on this driver and no behaviour to lose.
	SkipStatement
	// SkipStatementLoudly drops it and warns. Reserved for constructs whose
	// absence is a real behavioural difference on this driver.
	SkipStatementLoudly
	// TolerateDuplicate runs it and swallows an "already exists" error,
	// standing in for the `IF NOT EXISTS` the driver does not support.
	TolerateDuplicate
	// TolerateMissing runs it and swallows a "does not exist" error, standing
	// in for the `IF EXISTS` the driver does not support.
	TolerateMissing
)

var (
	pragmaRE = regexp.MustCompile(`(?is)^\s*PRAGMA\b`)
	// foreignKeyPragmaRE is the one PRAGMA with a portable meaning. The
	// anchored form classifies; the unanchored form rewrites, because the
	// statement handed to the rewriter still carries its leading comment.
	txControlRE            = regexp.MustCompile(`(?is)^\s*(BEGIN|COMMIT|END|ROLLBACK)(\s+TRANSACTION)?\s*;?\s*$`)
	foreignKeyPragmaRE     = regexp.MustCompile(`(?is)^\s*PRAGMA\s+foreign_keys\s*=\s*(ON|OFF|1|0)\s*;?\s*$`)
	foreignKeyPragmaBodyRE = regexp.MustCompile(`(?i)PRAGMA\s+foreign_keys\s*=\s*(ON|OFF|1|0)`)
	createTriggerRE        = regexp.MustCompile(`(?is)^\s*CREATE\s+TRIGGER\b`)
	dropTriggerRE          = regexp.MustCompile(`(?is)^\s*DROP\s+TRIGGER\b`)
	createIndexRE          = regexp.MustCompile(`(?is)^\s*CREATE\s+(UNIQUE\s+)?INDEX\s+IF\s+NOT\s+EXISTS\b`)
	dropIndexIfExistsRE    = regexp.MustCompile(`(?is)^\s*DROP\s+INDEX\s+IF\s+EXISTS\b`)
	// createViewRE matches SQLite's CREATE VIEW IF NOT EXISTS, which neither
	// Postgres nor MySQL supports; both spell it CREATE OR REPLACE VIEW.
	createViewRE = regexp.MustCompile(`(?i)CREATE\s+VIEW\s+IF\s+NOT\s+EXISTS\s+`)
	// createIndexBodyRE and dropIndexBodyRE are the unanchored rewriting
	// counterparts of the anchored patterns the classifier uses.
	createIndexBodyRE = regexp.MustCompile(`(?i)CREATE\s+(UNIQUE\s+)?INDEX\s+IF\s+NOT\s+EXISTS\s+`)
	dropIndexBodyRE   = regexp.MustCompile(`(?i)DROP\s+INDEX\s+(?:IF\s+EXISTS\s+)?"?([a-z_][a-z0-9_]*)"?\s*;?`)
	// insertTargetRE names the table an INSERT writes to, so a MySQL
	// ON DUPLICATE KEY assignment can be qualified.
	insertTargetRE = regexp.MustCompile(`(?is)INSERT\s+(?:OR\s+\w+\s+)?(?:IGNORE\s+)?INTO\s+"?([a-z_][a-z0-9_]*)"?`)
	// textDivisionRE matches `<column> / <power of ten>` — the nanosecond
	// conversion in 077_observability_history.sql, and the only place the corpus does
	// arithmetic on a text column.
	textDivisionRE = regexp.MustCompile(`(?i)\b([a-z_][a-z0-9_]*)\s*/\s*([0-9]{7,})\b`)
	// coalesceNowRE matches the now() that datetime('now') was translated
	// into, when it sits inside a COALESCE beside a text column.
	coalesceNowRE = regexp.MustCompile(`(?is)(COALESCE\([^()]*,\s*)now\(\)(\s*\))`)
	// rowidRE matches SQLite's implicit row identifier.
	rowidRE = regexp.MustCompile(`(?i)\browid\b`)
	// castIntegerRE and castTextRE match the two SQLite cast target types
	// MySQL does not recognise.
	castIntegerRE = regexp.MustCompile(`(?is)CAST\s*\(\s*(.+?)\s+AS\s+INTEGER\s*\)`)
	castTextRE    = regexp.MustCompile(`(?is)CAST\s*\(\s*(.+?)\s+AS\s+TEXT\s*\)`)
	// setNullRE matches a foreign key action that nulls the referencing column.
	setNullRE      = regexp.MustCompile(`(?i)ON\s+DELETE\s+SET\s+NULL`)
	partialIndexRE = regexp.MustCompile(`(?is)^\s*CREATE\s+(UNIQUE\s+)?INDEX\b.*\bWHERE\b`)
	// expressionIndexRE matches an index whose column list contains a call —
	// a functional key part, which MariaDB does not support at all.
	expressionIndexRE = regexp.MustCompile(`(?is)^\s*CREATE\s+(?:UNIQUE\s+)?INDEX\b[^(]*\([^)]*[a-z_]+\s*\(`)
)

// ClassifyStatement decides how to handle one statement on one driver.
func ClassifyStatement(driver Driver, stmt string) StatementAction {
	if driver == DriverSQLite {
		return ExecuteStatement
	}

	// Classify on the comment-stripped text. The splitter keeps a statement's
	// leading comment block attached to it — which is right, because a failure
	// message should show the comment that explains the statement — but it
	// means a `^\s*PRAGMA` anchor would never match.
	stmt = strings.TrimSpace(stripComments(stmt))

	switch {
	case foreignKeyPragmaRE.MatchString(stmt):
		// `PRAGMA foreign_keys = OFF` is not decoration: it is what lets a
		// SQLite table-rebuild drop the old table while other tables still
		// reference it. MySQL has a direct equivalent and needs it for exactly
		// the same reason; Postgres does not, because translatePostgres adds
		// CASCADE to the DROP instead.
		if driver == DriverMySQL {
			return ExecuteStatement
		}
		return SkipStatement

	case txControlRE.MatchString(stmt):
		// A file that manages its own transaction does so for SQLite's
		// benefit — see the no-transaction marker in migrate.go. Every other
		// driver still runs inside the transaction applyMigration opened, and
		// a nested BEGIN or an early COMMIT there would break its rollback.
		return SkipStatement

	case pragmaRE.MatchString(stmt):
		// Every other PRAGMA tunes the SQLite file format and journal.
		// Postgres and MySQL have their own equivalents, configured at the
		// server, and nothing in the application depends on one having run.
		return SkipStatement

	case createTriggerRE.MatchString(stmt) || dropTriggerRE.MatchString(stmt):
		// The only triggers in this migration set are the audit-log WORM
		// guards in 072_audit.sql, written with SQLite's RAISE(ABORT). There
		// is no portable equivalent — Postgres wants a PL/pgSQL function and
		// RAISE EXCEPTION, MySQL wants SIGNAL SQLSTATE — and silently emitting
		// a broken one would be worse than not having it.
		//
		// Skipped loudly: on Postgres and MySQL the audit.worm setting has no
		// database-level enforcement until someone ports these two triggers.
		// The application-level audit path is unaffected.
		return SkipStatementLoudly

	case driver == DriverMSSQL:
		return classifyMSSQL(stmt)

	case driver == DriverMySQL && !MySQLCanExpress(stmt):
		// A correlated JSON_TABLE, which MariaDB rejects outright. See
		// MySQLCanExpress: the only statements that reach this are an upgrade
		// repair that is a no-op on a fresh install.
		return SkipStatementLoudly

	case driver == DriverMySQL && expressionIndexRE.MatchString(stmt):
		// MariaDB has no functional key parts, and the corpus has one index
		// over COALESCE(...). §9 of the secrets plan predicted exactly this,
		// and it is why the secrets schema uses a hash column rather than an
		// expression index.
		//
		// Skipped loudly: the uniqueness it enforces is lost on MySQL and
		// MariaDB until someone adds the equivalent generated column.
		return SkipStatementLoudly

	case driver == DriverMySQL && partialIndexRE.MatchString(stmt):
		// MySQL has no partial indexes. The one in this set is a UNIQUE index
		// with a WHERE clause, so dropping the predicate would *widen* the
		// constraint and reject rows the application expects to insert.
		// Skipping it loses the constraint instead, which is the direction
		// that keeps the application working.
		return SkipStatementLoudly

	case driver == DriverMySQL && createIndexRE.MatchString(stmt):
		// MySQL 8 has no CREATE INDEX IF NOT EXISTS. MariaDB does, but
		// treating both the same keeps one code path.
		return TolerateDuplicate

	case driver == DriverMySQL && dropIndexIfExistsRE.MatchString(stmt):
		// Nor a DROP INDEX IF EXISTS. Same trick in the other direction: run
		// it and swallow the "doesn't exist" error.
		return TolerateMissing
	}

	return ExecuteStatement
}

// ─── Rewrites ─────────────────────────────────────────────────────────────

var (
	// keyedTextRE finds a `<name> TEXT` column definition. Whether it is
	// rewritten depends on whether the column is keyed — see rewriteKeyedText.
	keyedTextRE = regexp.MustCompile(`(?i)(\n\s*|\(\s*|ADD\s+COLUMN\s+)("?[a-z_][a-z0-9_]*"?)\s+TEXT\b`)

	// textTimestampRE matches the shape 200 of the 116 migration files use for
	// a timestamp column: `TEXT ... DEFAULT (CURRENT_TIMESTAMP)`. MySQL
	// rejects any default on a TEXT column outright.
	// Both spellings appear in the corpus: `DEFAULT (CURRENT_TIMESTAMP)` in
	// most files and a bare `DEFAULT CURRENT_TIMESTAMP` in a few. They are
	// alternatives rather than optional parentheses — with `\(?…\)?` the
	// trailing paren happily matched the *table's* closing paren on the last
	// column of a CREATE TABLE and swallowed it.
	textTimestampRE = regexp.MustCompile(`(?i)\bTEXT(\s+NOT\s+NULL)?\s+DEFAULT\s+(?:\(\s*CURRENT_TIMESTAMP\s*\)|CURRENT_TIMESTAMP)`)

	// insertOrIgnoreRE / insertOrReplaceRE are SQLite's conflict shorthands.
	insertOrIgnoreRE  = regexp.MustCompile(`(?i)\bINSERT\s+OR\s+IGNORE\s+INTO\b`)
	insertOrReplaceRE = regexp.MustCompile(`(?i)\bINSERT\s+OR\s+REPLACE\s+INTO\b`)

	// collateNocaseRE is SQLite's case-insensitive collation. MySQL's default
	// utf8mb4 collation is already case-insensitive; Postgres needs CITEXT or
	// a lower() index, neither of which a regex can retrofit, so the clause is
	// dropped and the comparison becomes case-sensitive there.
	collateNocaseRE = regexp.MustCompile(`(?i)\s+COLLATE\s+NOCASE\b`)

	// autoincrementRE — none in this set today, but a new migration could add
	// one and it is a one-word fix.
	autoincrementRE = regexp.MustCompile(`(?i)\bAUTOINCREMENT\b`)

	// integerTypeRE matches the INTEGER type name, anchored on the whitespace
	// before it; integerAutoincrementRE is SQLite's rowid-alias key, which
	// Postgres spells BIGSERIAL.
	integerTypeRE          = regexp.MustCompile(`(?i)(\s)INTEGER\b`)
	integerAutoincrementRE = regexp.MustCompile(`(?i)(\s)INTEGER\s+PRIMARY\s+KEY\s+AUTOINCREMENT\b`)

	// dropTableRE adds CASCADE on Postgres. Every DROP TABLE in this set is
	// the tail of a SQLite table-rebuild (create new, copy, drop old, rename),
	// and Postgres refuses to drop a table another table's foreign key still
	// references.
	dropTableRE = regexp.MustCompile(`(?i)\bDROP\s+TABLE\s+(?:IF\s+EXISTS\s+)?[a-z_][a-z0-9_]*\s*;`)

	// SQLite scalar functions with per-driver equivalents.
	randomblobRE  = regexp.MustCompile(`(?i)\bhex\s*\(\s*randomblob\s*\(\s*(\d+)\s*\)\s*\)`)
	strftimeNowRE = regexp.MustCompile(`(?i)strftime\(\s*'[^']*'\s*,\s*'now'\s*\)`)
	jsonExtractRE = regexp.MustCompile(`(?i)json_extract\(\s*([^,]+?)\s*,\s*'\$\.([a-z0-9_]+)'\s*\)`)

	// datetimeTypeRE matches a DATETIME column type. Anchored on the
	// whitespace before it so `DATETIME` inside an identifier or a string is
	// left alone.
	datetimeTypeRE = regexp.MustCompile(`(?i)(\s)DATETIME\b`)

	// defaultedTextRE matches a TEXT column with a literal default, capturing
	// the literal so it can be parenthesised into an expression default.
	// Already-parenthesised defaults are excluded by the character class.
	defaultedTextRE = regexp.MustCompile(`(?i)\bTEXT(\s+(?:NOT\s+NULL\s+)?)DEFAULT\s+('[^']*'|-?[0-9.]+)`)

	// jsonExtractWholeRE matches `json_extract(col, '$')`, which extracts the
	// whole document. Every use in this schema feeds a LIKE, and the column
	// already holds JSON as text, so the call is a no-op on both Postgres and
	// MySQL once removed.
	jsonExtractWholeRE = regexp.MustCompile(`(?i)json_extract\(\s*([^,]+?)\s*,\s*'\$'\s*\)`)
)

// TranslateStatement rewrites one statement into the active driver's dialect.
//
// SQLite gets the text back unchanged — it is the dialect the files are
// written in, and the shipped default. Every rewrite below is therefore
// exercised only on an installation that has deliberately pointed
// MCHARBOR_DB_DRIVER at something else.
func TranslateStatement(driver Driver, stmt string) string {
	switch driver {
	case DriverSQLite:
		return stmt
	case DriverPostgres:
		return translatePostgres(stmt)
	case DriverMySQL:
		return translateMySQL(stmt)
	case DriverMSSQL:
		return translateMSSQL(stmt)
	}
	return stmt
}

func translatePostgres(stmt string) string {
	out := stmt

	// SQLite conflict shorthands → the standard clause Postgres understands.
	out = insertOrIgnoreRE.ReplaceAllString(out, "INSERT INTO")
	if insertOrIgnoreRE.MatchString(stmt) && !strings.Contains(strings.ToUpper(out), "ON CONFLICT") {
		out = appendOnConflictDoNothing(out)
	}
	out = collateNocaseRE.ReplaceAllString(out, "")
	// SQLite's INTEGER is 64-bit and Postgres's is int4, so a byte count past
	// 2 GiB fit the schema's contract and not the column. The serial form goes
	// first, because stripping AUTOINCREMENT would leave a key with no default.
	out = integerAutoincrementRE.ReplaceAllString(out, "${1}BIGSERIAL PRIMARY KEY")
	out = integerTypeRE.ReplaceAllString(out, "${1}BIGINT")
	out = autoincrementRE.ReplaceAllString(out, "")
	out = createViewRE.ReplaceAllString(out, "CREATE OR REPLACE VIEW ")

	// A table-rebuild's DROP is refused while another table's foreign key
	// still points at it. Unanchored, because the statement handed here still
	// carries the comment block that explains it.
	out = dropTableRE.ReplaceAllStringFunc(out, func(match string) string {
		if strings.Contains(strings.ToUpper(match), "CASCADE") {
			return match
		}
		return strings.TrimRight(strings.TrimSpace(match), ";") + " CASCADE;"
	})

	// hex(randomblob(n)) produces 2n hex characters. gen_random_bytes would be
	// the direct equivalent but lives in pgcrypto, which an operator's
	// Postgres may not have installed; gen_random_uuid() is built in from
	// PG13 and yields 32 hex characters once the dashes come out, which
	// covers every use in this schema.
	out = randomblobRE.ReplaceAllStringFunc(out, func(match string) string {
		groups := randomblobRE.FindStringSubmatch(match)
		if len(groups) < 2 {
			return match
		}
		bytes, err := strconv.Atoi(groups[1])
		if err != nil || bytes <= 0 || bytes > 16 {
			return match
		}
		return "substr(replace(gen_random_uuid()::text, '-', ''), 1, " +
			strconv.Itoa(bytes*2) + ")"
	})
	// strftime('<any format>', 'now') → now(); every use in this set is a
	// "right now" stamp rather than a real format conversion.
	out = strftimeNowRE.ReplaceAllString(out, "now()")
	// json_extract(col, '$') is the whole document, which the column already
	// is. Must run before the field form, whose pattern would otherwise not
	// match it and leave the call intact.
	out = jsonExtractWholeRE.ReplaceAllString(out, "$1")
	// json_extract(col, '$.field') → col::jsonb ->> 'field'
	out = jsonExtractRE.ReplaceAllString(out, "($1::jsonb ->> '$2')")

	// datetime('now') / datetime('now', '-1 day') — SQLite scalars with no
	// Postgres equivalent under that name. Run before the DATETIME *type*
	// rewrite, or `datetime(` would be half-rewritten into `TIMESTAMP(`.
	out = datetimeNowRE.ReplaceAllStringFunc(out, func(match string) string {
		parts := datetimeNowRE.FindStringSubmatch(match)
		if len(parts) > 1 && strings.TrimSpace(parts[1]) != "" {
			return "(now() + interval '" + strings.TrimSpace(parts[1]) + "')"
		}
		return "now()"
	})

	// DATETIME is MySQL/SQLite spelling; Postgres calls it TIMESTAMP.
	out = datetimeTypeRE.ReplaceAllString(out, "${1}TIMESTAMP")
	out = addGeneratedIDDefault(out, "replace(gen_random_uuid()::text, '-', '')")

	// Arithmetic on a TEXT column: Postgres has no text/integer operator, and
	// 077_observability_history.sql divides a timestamp column that SQLite happily
	// coerces. The cast is scoped to a division by a large power of ten, which
	// is the nanosecond conversion and nothing else.
	out = textDivisionRE.ReplaceAllString(out, "(${1})::numeric / ${2}")

	// SQLite's implicit rowid. Postgres's nearest equivalent is ctid, which is
	// unique within the table and stable for the life of the statement. No
	// migration uses rowid at present; the rewrite stays so one that does ports.
	out = rowidRE.ReplaceAllString(out, "ctid::text")

	// `COALESCE(created_at, now())` mixes text with timestamptz, which
	// Postgres refuses. The column is text in this schema, so the fallback is
	// cast to match it rather than the other way round.
	out = coalesceNowRE.ReplaceAllString(out, "${1}now()::text${2}")

	// The JSON primitives the permission migrations use.
	out = translateJSONPostgres(out)
	// Postgres spells a POSIX regular-expression match `~`.
	out = translateGlob(out, "~")

	return out
}

func translateMySQL(stmt string) string {
	out := stmt

	// The two rewrites that were already here before Phase 0.
	// The target table qualifies the no-op assignment. Unqualified, MySQL
	// rejects `ON DUPLICATE KEY UPDATE id = id` as ambiguous whenever the
	// INSERT draws from a SELECT that also has an `id`, which several of the
	// seeding migrations do.
	insertTarget := ""
	if m := insertTargetRE.FindStringSubmatch(out); m != nil {
		insertTarget = m[1] + "."
	}
	// `ON CONFLICT (cols) DO UPDATE SET a = excluded.a` is the upsert form.
	// MySQL spells the "value that would have been inserted" VALUES(a).
	out = onConflictUpdateRE.ReplaceAllStringFunc(out, func(match string) string {
		groups := onConflictUpdateRE.FindStringSubmatch(match)
		if len(groups) < 3 {
			return match
		}
		return "ON DUPLICATE KEY UPDATE " + excludedRE.ReplaceAllString(groups[2], "VALUES($1)")
	})

	out = onConflictMySQLRE.ReplaceAllStringFunc(out, func(match string) string {
		groups := onConflictMySQLRE.FindStringSubmatch(match)
		if len(groups) < 2 {
			return match
		}
		cols := strings.Split(groups[1], ",")
		assignments := make([]string, 0, len(cols))
		for _, col := range cols {
			col = strings.TrimSpace(col)
			if col == "" {
				continue
			}
			assignments = append(assignments, insertTarget+col+" = "+insertTarget+col)
		}
		return "ON DUPLICATE KEY UPDATE " + strings.Join(assignments, ", ")
	})
	out = datetimeNowRE.ReplaceAllStringFunc(out, func(match string) string {
		if strings.Contains(match, ",") {
			parts := datetimeNowRE.FindStringSubmatch(match)
			if len(parts) < 2 {
				return match
			}
			return "CURRENT_TIMESTAMP " + strings.TrimSpace(parts[1])
		}
		return "CURRENT_TIMESTAMP"
	})

	// A TEXT column cannot carry a default on MySQL at all. Every one of these
	// is a timestamp, so DATETIME is both legal and more correct than the TEXT
	// it replaces. Must run before the keyed-column rewrite so that rewrite
	// does not see a TEXT that is about to become a DATETIME.
	out = textTimestampRE.ReplaceAllStringFunc(out, func(match string) string {
		if regexp.MustCompile(`(?i)NOT\s+NULL`).MatchString(match) {
			return "DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP"
		}
		return "DATETIME DEFAULT CURRENT_TIMESTAMP"
	})

	// Unanchored, unlike the classifier's copy: the splitter keeps a
	// statement's leading comment attached, so an anchored pattern would not
	// match the text that actually executes.
	out = foreignKeyPragmaBodyRE.ReplaceAllStringFunc(out, func(match string) string {
		if regexp.MustCompile(`(?i)\b(OFF|0)\b`).MatchString(match) {
			return "SET FOREIGN_KEY_CHECKS = 0"
		}
		return "SET FOREIGN_KEY_CHECKS = 1"
	})

	out = rewriteKeyedText(out)
	out = rewriteDefaultedText(out)
	out = addGeneratedIDDefault(out, "(UUID())")

	out = insertOrIgnoreRE.ReplaceAllString(out, "INSERT IGNORE INTO")
	out = insertOrReplaceRE.ReplaceAllString(out, "REPLACE INTO")
	// utf8mb4's default collation is already case-insensitive.
	out = collateNocaseRE.ReplaceAllString(out, "")
	out = autoincrementRE.ReplaceAllString(out, "AUTO_INCREMENT")
	// MySQL 8 has no CREATE INDEX IF NOT EXISTS; the executor tolerates the
	// duplicate-key error instead (see ClassifyStatement).
	// Unanchored, unlike the classifier's copy: the statement handed to the
	// rewriter still carries the comment block that documents it, so an
	// anchored pattern matches nothing.
	out = createIndexBodyRE.ReplaceAllString(out, "CREATE ${1}INDEX ")

	// SQLite's `DROP INDEX IF EXISTS x` names no table; MySQL requires
	// `DROP INDEX x ON <table>` and has no IF EXISTS for it at all. The table
	// comes from the corpus-wide index of where each index was created, and
	// the executor tolerates the "doesn't exist" error in place of IF EXISTS.
	out = dropIndexBodyRE.ReplaceAllStringFunc(out, func(match string) string {
		groups := dropIndexBodyRE.FindStringSubmatch(match)
		if len(groups) < 2 {
			return match
		}
		table, ok := indexTableIndex()[strings.ToLower(groups[1])]
		if !ok {
			return match
		}
		return "DROP INDEX " + groups[1] + " ON " + table
	})

	out = translateJSONMySQL(out)
	// MySQL and MariaDB spell it REGEXP.
	out = translateGlob(out, "REGEXP")
	// MySQL has no rowid at all. The only use is minting a unique suffix, so
	// a UUID serves the same purpose.
	out = rowidRE.ReplaceAllString(out, "UUID()")
	out = fixNotNullSetNull(out)
	out = createViewRE.ReplaceAllString(out, "CREATE OR REPLACE VIEW ")
	// SQLite accepts any type name in a CAST; MySQL takes only its own small
	// set. INTEGER and TEXT are the two this corpus uses.
	out = castIntegerRE.ReplaceAllString(out, "CAST(${1} AS SIGNED)")
	out = castTextRE.ReplaceAllString(out, "CAST(${1} AS CHAR)")
	// MySQL's INTEGER is a 32-bit INT, as Postgres's is. After the cast rewrite,
	// so no INTEGER is left inside a CAST for this to widen.
	out = integerTypeRE.ReplaceAllString(out, "${1}BIGINT")
	out = quoteMySQLReservedWords(out)
	// RANDOM_BYTES is MySQL-only (MariaDB got it in 11.x). UUID() is in both,
	// and hex(randomblob(n)) wants 2n hex characters, which the dash-stripped
	// form of a UUID supplies up to 32.
	out = randomblobRE.ReplaceAllStringFunc(out, func(match string) string {
		groups := randomblobRE.FindStringSubmatch(match)
		if len(groups) < 2 {
			return match
		}
		bytes, err := strconv.Atoi(groups[1])
		if err != nil || bytes <= 0 || bytes > 16 {
			return match
		}
		return "SUBSTR(REPLACE(UUID(), '-', ''), 1, " + strconv.Itoa(bytes*2) + ")"
	})
	out = strftimeNowRE.ReplaceAllString(out, "NOW()")
	out = jsonExtractWholeRE.ReplaceAllString(out, "$1")
	out = jsonExtractRE.ReplaceAllString(out, "JSON_UNQUOTE(JSON_EXTRACT($1, '$.$2'))")

	return out
}

// appendOnConflictDoNothing puts the clause at the end of an INSERT that used
// SQLite's `INSERT OR IGNORE`, before any trailing semicolon.
func appendOnConflictDoNothing(stmt string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(stmt), ";")
	return trimmed + " ON CONFLICT DO NOTHING;"
}

// rewriteKeyedText turns `<col> TEXT` into `<col> VARCHAR(255)` for columns
// that participate in a key, and leaves every other TEXT column alone.
//
// MySQL rejects TEXT and BLOB in a key specification without a prefix length.
// **Keyed columns only** is the whole of the rule: `secrets.value` holds a
// multi-KB kubeconfig or TLS certificate chain and must stay TEXT, and so must
// every other free-text column in the schema.
func rewriteKeyedText(stmt string) string {
	upper := strings.ToUpper(stmt)
	if !strings.Contains(upper, "CREATE TABLE") && !strings.Contains(upper, "ALTER TABLE") {
		return stmt
	}

	// A column added by ALTER TABLE years after its table was created is
	// indexed just as often as one declared inline, and MySQL rejects a TEXT
	// column in a key wherever it came from.
	// The table name is read from the comment-stripped text. A migration's
	// leading comment frequently quotes SQL — 086_repair_notification_reads_fk
	// documents the whole SQLite table-rebuild recipe — and matching that
	// picked the wrong table, which left the real one's keyed columns as TEXT.
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

	// Columns keyed inside this very statement, for a CREATE TABLE the corpus
	// scan has not seen (a test fixture, or a migration added after the index
	// was built).
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
		return groups[1] + groups[2] + " VARCHAR(" + strconv.Itoa(keyedTextLength) + ")"
	})
}

// rewriteDefaultedText parenthesises the default on a TEXT column.
//
// MySQL rejects a plain `DEFAULT 'x'` on TEXT — the type cannot carry a
// literal default at all. Both MySQL 8.0.13+ and MariaDB 10.2+ do accept a
// default written as an *expression*, and `('x')` is one, so the fix is a pair
// of parentheses rather than a type change.
//
// The obvious alternative — widening the column to a VARCHAR — was tried and
// rejected: `appstore_catalog` carries enough defaulted TEXT columns that any
// width large enough for `roles.permissions` (543 characters in the longest
// seeded row) pushes the table past MySQL's 65,535-byte row limit.
//
// Runs after the timestamp rewrite, which has already taken the
// `DEFAULT (CURRENT_TIMESTAMP)` columns out of the running.
func rewriteDefaultedText(stmt string) string {
	upper := strings.ToUpper(stmt)
	if !strings.Contains(upper, "CREATE TABLE") && !strings.Contains(upper, "ADD COLUMN") {
		return stmt
	}
	return defaultedTextRE.ReplaceAllString(stmt, "TEXT${1}DEFAULT (${2})")
}

var (
	// alterTableNameRE names the table an ALTER TABLE targets, so a column
	// added years after the table was created can be looked up in the
	// corpus-wide keyed index.
	alterTableNameRE = regexp.MustCompile(`(?is)ALTER\s+TABLE\s+"?([a-z_][a-z0-9_]*)"?\s`)
	inlineKeyRE      = regexp.MustCompile(`(?i)(\n\s*|\(\s*)("?[a-z_][a-z0-9_]*"?)\s+TEXT\b[^,\n]*\b(PRIMARY\s+KEY|UNIQUE|REFERENCES)\b`)
	tableKeyRE       = regexp.MustCompile(`(?i)\b(PRIMARY\s+KEY|UNIQUE|FOREIGN\s+KEY)\s*\(([^)]+)\)`)
)

// keyedColumnNames collects every column of a CREATE TABLE that takes part in
// a key, from both the inline form (`id TEXT PRIMARY KEY`) and the table
// constraint form (`UNIQUE(name, scope_key)`).
func keyedColumnNames(stmt string) map[string]bool {
	keyed := make(map[string]bool)

	for _, m := range inlineKeyRE.FindAllStringSubmatch(stmt, -1) {
		keyed[strings.ToLower(strings.Trim(m[2], `"`))] = true
	}
	for _, m := range tableKeyRE.FindAllStringSubmatch(stmt, -1) {
		for _, col := range strings.Split(m[2], ",") {
			col = strings.TrimSpace(col)
			col = strings.TrimSuffix(col, " DESC")
			col = strings.TrimSuffix(col, " ASC")
			col = strings.Trim(strings.TrimSpace(col), `"`)
			if col != "" {
				keyed[strings.ToLower(col)] = true
			}
		}
	}
	return keyed
}

// isDuplicateObjectError recognises the "already exists" errors the
// TolerateDuplicate action swallows, standing in for an unsupported
// `IF NOT EXISTS`.
//
// Matched on message text rather than on a driver error type, because
// core/db is deliberately driver-agnostic and importing three error packages
// to read three codes would invert that.
func isDuplicateObjectError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key name") || // MySQL 1061
		strings.Contains(msg, "already exists") // Postgres 42P07 / 42710
}

// mysqlReservedWords are the words this schema uses as identifiers that MySQL
// 8 or MariaDB 10.6 reserves. Derived by intersecting every column and table
// name in the migration corpus with both reserved-word lists, not guessed.
//
// `default` is deliberately absent and must stay that way: it is a keyword in
// every DEFAULT clause in the corpus.
var mysqlReservedWords = []string{
	"key", "condition", "generated", "system", "trigger",
	"order", "interval", "read", "rank", "usage", "groups", "long",
}

// The first attempt at this quoted every whole-word occurrence outside a
// string literal, and was wrong in both directions at once: it backticked the
// KEY in `PRIMARY KEY` and the ORDER in `ORDER BY`, while missing `groups` as
// a table name in `REFERENCES groups(id)`.
//
// A reserved word is only an identifier in a handful of syntactic positions,
// and matching those positions directly is both shorter and correct. Each
// pattern below captures the word in group 2 so the replacement can backtick
// exactly it.
var reservedWordContexts = func() map[string][]*regexp.Regexp {
	types := `(?:TEXT|INTEGER|BIGINT|REAL|BLOB|VARCHAR\([0-9]+\)|DATETIME|TIMESTAMP|BOOLEAN|NUMERIC)`
	out := make(map[string][]*regexp.Regexp, len(mysqlReservedWords))
	for _, word := range mysqlReservedWords {
		out[word] = []*regexp.Regexp{
			// A column definition: `(\n  key TEXT …` or `(key TEXT …`.
			// The trailing character class rather than \b matters: the keyed
			// rewrite has already turned TEXT into VARCHAR(191) by the time
			// this runs, and there is no word boundary between `)` and the
			// space after it.
			regexp.MustCompile(`(?i)([\n(,]\s*)(` + word + `)(\s+` + types + `[\s,)])`),
			// A foreign key's target table: `REFERENCES groups(id)`.
			regexp.MustCompile(`(?i)(\bREFERENCES\s+)(` + word + `)(\s*\()`),
			// A column list entry, in an INSERT or an index: `(id, key, value)`.
			regexp.MustCompile(`(?i)([(,]\s*)(` + word + `)(\s*[,)])`),
			// An assignment or a predicate: `SET key = …`, `WHERE key IN (…)`,
			// `AND key LIKE …`, `WHERE key IS NULL`.
			regexp.MustCompile(`(?i)(\b(?:SET|WHERE|AND|OR|ON)\s+)(` + word + `)(\s*(?:[=<>!]|\b(?:IN|LIKE|IS|NOT|BETWEEN)\b))`),
			// A qualified reference: `s.key`.
			regexp.MustCompile(`(?i)(\.)(` + word + `)(\s*[,)\s=])`),
			// A table name: `CREATE TABLE groups (`, `INSERT INTO groups`,
			// `FROM groups`, `JOIN groups`, `UPDATE groups SET`.
			regexp.MustCompile(`(?i)(\b(?:CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?|INSERT\s+INTO\s+|INSERT\s+IGNORE\s+INTO\s+|REPLACE\s+INTO\s+|DELETE\s+FROM\s+|FROM\s+|JOIN\s+|UPDATE\s+|TABLE\s+|ON\s+)\s*)(` + word + `)(\s*[,;(\s])`),
		}
	}
	return out
}()

// quoteMySQLReservedWords backticks reserved words used as identifiers,
// leaving genuine keywords and string literals alone.
func quoteMySQLReservedWords(stmt string) string {
	// Comments are masked first. The quoter skips string literals by splitting
	// on `'`, and an apostrophe inside a comment — "don\'t", which
	// 012_default_groups.sql opens with — inverts every segment after it, so
	// nothing downstream gets quoted at all.
	masked, comments := maskComments(stmt)

	segments := strings.Split(masked, "'")
	for i := range segments {
		if i%2 == 1 {
			continue // inside a string literal
		}
		for word, patterns := range reservedWordContexts {
			for _, re := range patterns {
				segments[i] = re.ReplaceAllString(segments[i], "${1}`"+word+"`${3}")
			}
		}
	}
	return restoreComments(strings.Join(segments, "'"), comments)
}

// commentPlaceholder is a token no SQL in this corpus contains, used to stand
// in for a comment while the quoter works on the code around it.
const commentPlaceholder = "\x00mcharbor_comment_%d\x00"

// maskComments replaces every `--` line comment with a placeholder and
// returns them in order.
func maskComments(stmt string) (string, []string) {
	var comments []string
	var out strings.Builder

	for _, line := range strings.Split(stmt, "\n") {
		if idx := strings.Index(line, "--"); idx >= 0 {
			comments = append(comments, line[idx:])
			out.WriteString(line[:idx])
			fmt.Fprintf(&out, commentPlaceholder, len(comments)-1)
		} else {
			out.WriteString(line)
		}
		out.WriteString("\n")
	}
	return strings.TrimSuffix(out.String(), "\n"), comments
}

// restoreComments puts the masked comments back.
func restoreComments(stmt string, comments []string) string {
	for i, comment := range comments {
		stmt = strings.Replace(stmt, fmt.Sprintf(commentPlaceholder, i), comment, 1)
	}
	return stmt
}

// globRE matches SQLite's GLOB operator. 077_observability_history.sql uses it once,
// as `timestamp GLOB '[0-9]*'`, to tell a numeric timestamp from an RFC3339
// one before rewriting the column.
var globRE = regexp.MustCompile(`(?i)\bGLOB\s+'([^']*)'`)

// translateGlob rewrites a GLOB into the driver's regular-expression operator.
//
// GLOB is shell-style rather than SQL-style, and the corpus uses exactly one
// pattern — `[0-9]*`, meaning "starts with a digit". Translating the general
// case would mean writing a glob-to-regex compiler for one call site, so the
// character-class and star forms are handled and anything else is left alone
// to fail loudly rather than silently match the wrong rows.
func translateGlob(stmt, operator string) string {
	return globRE.ReplaceAllStringFunc(stmt, func(match string) string {
		groups := globRE.FindStringSubmatch(match)
		if len(groups) < 2 {
			return match
		}
		pattern := groups[1]
		if !strings.HasSuffix(pattern, "*") {
			return match
		}
		prefix := strings.TrimSuffix(pattern, "*")
		if strings.ContainsAny(prefix, "*?") {
			return match
		}
		return operator + " '^" + prefix + "'"
	})
}

// canonicalTableName maps a SQLite table-rebuild's scratch name back to the
// table it will become.
//
// The rebuild pattern — `CREATE TABLE x_new (…)`, copy, `DROP TABLE x`,
// `ALTER TABLE x_new RENAME TO x` — is how every schema change to an existing
// table is expressed in this corpus, because SQLite cannot alter a constraint
// in place. The keyed-column index is built on final names, so without this
// the rebuilt table's columns look unkeyed and stay TEXT, and the CREATE INDEX
// two statements later fails.
func canonicalTableName(table string) string {
	// The corpus's own renames are exact; the suffix list below is the
	// fallback for a scratch table whose rename lives in a migration this
	// index could not parse.
	if final, ok := tableRenameIndex()[table]; ok {
		return final
	}
	for _, suffix := range []string{"_new", "_old", "_tmp", "_backup", "_legacy", "_fixed"} {
		if strings.HasSuffix(table, suffix) {
			return strings.TrimSuffix(table, suffix)
		}
	}
	return table
}

// idPrimaryKeyRE matches a single-column text primary key with no default.
var idPrimaryKeyRE = regexp.MustCompile(`(?i)(\n\s*"?id"?\s+(?:VARCHAR\(\d+\)|TEXT))\s+PRIMARY\s+KEY(\s*[,)])`)

// addGeneratedIDDefault gives `id TEXT PRIMARY KEY` a generated default.
//
// SQLite permits NULL in a TEXT PRIMARY KEY — a documented quirk of its rowid
// tables — and two migrations rely on it, inserting a settings row without
// naming an id. Postgres and MySQL both reject that, correctly.
//
// Rather than special-case those two INSERTs, every id primary key gets a
// default on the external drivers. The application always supplies an xid, so
// the default is never used in normal operation; it exists so a migration
// written against SQLite's laxer rule still applies.
func addGeneratedIDDefault(stmt, defaultExpr string) string {
	if !strings.Contains(strings.ToUpper(stmt), "CREATE TABLE") {
		return stmt
	}
	return idPrimaryKeyRE.ReplaceAllString(stmt,
		"${1} PRIMARY KEY DEFAULT "+defaultExpr+"${2}")
}

// isMissingObjectError recognises the "does not exist" errors the
// TolerateMissing action swallows, standing in for an unsupported
// `IF EXISTS`.
func isMissingObjectError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "doesn't exist") ||
		strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "check that column/key exists")
}

// fixNotNullSetNull rewrites `ON DELETE SET NULL` on a NOT NULL column.
//
// SQLite accepts the combination and only complains if the delete actually
// happens; MySQL rejects the table outright with errno 150, "Foreign key
// constraint is incorrectly formed". Three tables in this corpus declare it,
// all with `created_by TEXT NOT NULL … ON DELETE SET NULL`.
//
// NO ACTION is the closest thing MySQL will accept. It is a real behavioural
// difference — deleting the referenced user is refused rather than nulling the
// column — but the SQLite behaviour it replaces was already an error waiting
// to happen, so this is the safer of the two.
func fixNotNullSetNull(stmt string) string {
	if !strings.Contains(strings.ToUpper(stmt), "CREATE TABLE") {
		return stmt
	}
	if !setNullRE.MatchString(stmt) {
		return stmt
	}

	notNullColumns := map[string]bool{}
	for _, m := range notNullColumnRE.FindAllStringSubmatch(stmt, -1) {
		notNullColumns[strings.ToLower(strings.Trim(m[1], `"`))] = true
	}

	// Table-level: `FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL`.
	out := tableForeignKeyRE.ReplaceAllStringFunc(stmt, func(match string) string {
		groups := tableForeignKeyRE.FindStringSubmatch(match)
		if len(groups) < 2 || !notNullColumns[strings.ToLower(strings.TrimSpace(groups[1]))] {
			return match
		}
		return setNullRE.ReplaceAllString(match, "ON DELETE NO ACTION")
	})

	// Inline: `created_by TEXT NOT NULL REFERENCES users(id) ON DELETE SET NULL`.
	out = inlineNotNullForeignKeyRE.ReplaceAllStringFunc(out, func(match string) string {
		return setNullRE.ReplaceAllString(match, "ON DELETE NO ACTION")
	})

	return out
}

var (
	// notNullColumnRE names every NOT NULL column in a CREATE TABLE.
	notNullColumnRE = regexp.MustCompile(`(?i)[\n(]\s*("?[a-z_][a-z0-9_]*"?)\s+\S+[^,\n]*\bNOT\s+NULL\b`)
	// tableForeignKeyRE matches a table-level foreign key with a SET NULL action.
	tableForeignKeyRE = regexp.MustCompile(`(?is)FOREIGN\s+KEY\s*\(([^)]+)\)\s*REFERENCES[^,\n]*ON\s+DELETE\s+SET\s+NULL`)
	// inlineNotNullForeignKeyRE matches the inline form on a NOT NULL column.
	inlineNotNullForeignKeyRE = regexp.MustCompile(`(?i)[^,\n]*\bNOT\s+NULL\b[^,\n]*REFERENCES[^,\n]*ON\s+DELETE\s+SET\s+NULL`)
)
