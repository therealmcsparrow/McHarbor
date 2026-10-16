// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"regexp"
	"strings"
	"sync"
)

// MySQL and MariaDB reject a TEXT or BLOB column in a key specification
// without a prefix length, so every keyed column has to become a VARCHAR.
// Working out which columns those are is harder than it looks, because a
// column is frequently keyed somewhere other than its own CREATE TABLE:
//
//	CREATE TABLE sessions (… expires_at TEXT NOT NULL …);
//	CREATE INDEX idx_sessions_user_expires ON sessions(user_id, expires_at);
//
// A rewriter that only reads the CREATE TABLE leaves `expires_at` as TEXT and
// the CREATE INDEX two lines later fails. Worse, the index can live in a
// *different migration file* added years afterwards.
//
// So the keyed set is collected once, across the whole embedded migration
// corpus, before any of it is applied.

// keyedColumnIndex maps "table.column" to true for every column that
// participates in a primary key, unique constraint, foreign key or index
// anywhere in the migration set.
//
// Keyed by table *and* column: `value` is indexed on `settings` and is a
// multi-KB blob on `secrets`, and turning the latter into a VARCHAR(255)
// would truncate a TLS certificate chain.
var keyedColumnIndex = sync.OnceValue(buildKeyedColumnIndex)

var (
	createTableNameRE = regexp.MustCompile(`(?is)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?"?([a-z_][a-z0-9_]*)"?\s*\(`)
	createIndexOnRE   = regexp.MustCompile(`(?is)CREATE\s+(?:UNIQUE\s+)?INDEX\s+(?:IF\s+NOT\s+EXISTS\s+)?[^\s(]+\s+ON\s+"?([a-z_][a-z0-9_]*)"?\s*\(([^)]*)\)`)
	alterAddColumnRE  = regexp.MustCompile(`(?is)ALTER\s+TABLE\s+"?([a-z_][a-z0-9_]*)"?\s+ADD\s+COLUMN\s+"?([a-z_][a-z0-9_]*)"?\s+TEXT\b([^;]*)`)
)

func buildKeyedColumnIndex() map[string]bool {
	keyed := make(map[string]bool, 512)

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return keyed
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		body, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			continue
		}
		collectKeyedColumns(string(body), keyed)
	}
	return keyed
}

// collectKeyedColumns adds every keyed column in one migration file to the
// index. Exported shape kept internal; the tests call it directly.
func collectKeyedColumns(sqlText string, keyed map[string]bool) {
	for _, stmt := range SplitStatements(sqlText) {
		// Comments are stripped first: several migrations quote SQL in their
		// explanatory header, and indexing a table named only in prose would
		// record keys for a table that statement never touches.
		stmt = stripComments(stmt)
		upper := strings.ToUpper(stmt)

		switch {
		case strings.Contains(upper, "CREATE TABLE"):
			m := createTableNameRE.FindStringSubmatch(stmt)
			if m == nil {
				continue
			}
			table := strings.ToLower(m[1])
			for column := range keyedColumnNames(stmt) {
				keyed[table+"."+column] = true
			}

		case strings.Contains(upper, "CREATE INDEX"), strings.Contains(upper, "CREATE UNIQUE INDEX"):
			m := createIndexOnRE.FindStringSubmatch(stmt)
			if m == nil {
				continue
			}
			table := strings.ToLower(m[1])
			for _, column := range splitIndexColumns(m[2]) {
				keyed[table+"."+column] = true
			}

		case strings.Contains(upper, "ADD COLUMN"):
			// A column added later can still be keyed, either inline
			// (`REFERENCES …`) or by an index in the same file.
			for _, m := range alterAddColumnRE.FindAllStringSubmatch(stmt, -1) {
				if regexp.MustCompile(`(?i)\b(REFERENCES|UNIQUE|PRIMARY\s+KEY)\b`).MatchString(m[3]) {
					keyed[strings.ToLower(m[1])+"."+strings.ToLower(m[2])] = true
				}
			}
		}
	}
}

// splitIndexColumns parses an index's column list, dropping ASC/DESC and
// skipping anything that is an expression rather than a bare column — an
// expression index cannot be a TEXT column that needs rewriting.
func splitIndexColumns(list string) []string {
	var out []string
	for _, part := range strings.Split(list, ",") {
		part = strings.TrimSpace(part)
		part = regexp.MustCompile(`(?i)\s+(ASC|DESC)$`).ReplaceAllString(part, "")
		part = strings.Trim(part, `"`)
		if part == "" || strings.ContainsAny(part, "()' ") {
			continue
		}
		out = append(out, strings.ToLower(part))
	}
	return out
}

// keyedTextLength is the VARCHAR width a keyed TEXT column becomes.
//
// 191 rather than 255: MariaDB and MySQL measure an index against a byte
// limit, and utf8mb4 costs four bytes per character, so a two-column index of
// VARCHAR(255) is 2040 bytes and a three-column one blows past the 3072-byte
// ceiling. 191 × 4 = 764, which keeps a four-column index inside the limit.
//
// Nothing in this schema needs more: keyed columns are xid identifiers (20
// characters), SHA-256 hex digests (64), permission keys, table names and
// stack names. The columns that genuinely hold long values — a certificate, a
// kubeconfig, a compose file, a secret — are not keyed and stay TEXT.
const keyedTextLength = 191

// indexTableIndex maps an index name to the table it was created on.
//
// SQLite's `DROP INDEX x` needs no table because index names are global to the
// database; MySQL scopes them per table and demands `DROP INDEX x ON t`. The
// corpus already says where each index was created, so the table is looked up
// rather than guessed.
var indexTableIndex = sync.OnceValue(buildIndexTableIndex)

var indexNameRE = regexp.MustCompile(`(?is)CREATE\s+(?:UNIQUE\s+)?INDEX\s+(?:IF\s+NOT\s+EXISTS\s+)?"?([a-z_][a-z0-9_]*)"?\s+ON\s+"?([a-z_][a-z0-9_]*)"?`)

func buildIndexTableIndex() map[string]string {
	out := make(map[string]string, 256)
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return out
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		body, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			continue
		}
		for _, m := range indexNameRE.FindAllStringSubmatch(string(body), -1) {
			out[strings.ToLower(m[1])] = strings.ToLower(m[2])
		}
	}
	return out
}

// tableRenameIndex maps a table-rebuild's scratch name to the name it is
// renamed to.
//
// The rebuild pattern ends with `ALTER TABLE x_scratch RENAME TO x`, and the
// scratch name is whatever the author felt like — `_new`, `_fixed`,
// `_legacy`, `_old`. Guessing from a suffix list missed
// `in_app_notification_reads_fixed` and left its `deleted_at` a TEXT column
// that the index two statements later could not key.
//
// Reading the renames out of the corpus is both exact and self-maintaining:
// the next rebuild picks whatever suffix it likes and this still resolves it.
var tableRenameIndex = sync.OnceValue(buildTableRenameIndex)

var renameToRE = regexp.MustCompile(`(?is)ALTER\s+TABLE\s+"?([a-z_][a-z0-9_]*)"?\s+RENAME\s+TO\s+"?([a-z_][a-z0-9_]*)"?`)

func buildTableRenameIndex() map[string]string {
	out := make(map[string]string, 64)
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return out
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		body, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			continue
		}
		for _, m := range renameToRE.FindAllStringSubmatch(string(body), -1) {
			from := strings.ToLower(m[1])
			to := strings.ToLower(m[2])
			// `ALTER TABLE x RENAME TO x_old` is the *first* half of a rebuild
			// and must not be recorded: it would map the real table onto the
			// scratch one.
			if strings.HasPrefix(to, from) {
				continue
			}
			out[from] = to
		}
	}
	return out
}
