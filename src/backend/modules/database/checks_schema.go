// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package database

import (
	"context"
	"regexp"
	"strings"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

// createIndexRE matches the CREATE INDEX forms the migration set uses.
var createIndexRE = regexp.MustCompile(`(?is)^\s*CREATE\s+(?:UNIQUE\s+)?INDEX\s+(?:IF\s+NOT\s+EXISTS\s+)?["` + "`" + `]?([A-Za-z0-9_]+)["` + "`" + `]?\s+ON\s+["` + "`" + `]?([A-Za-z0-9_]+)["` + "`" + `]?`)

// dropIndexRE matches the DROP INDEX forms the migration set uses. An index a
// later migration drops is not "missing" — offering to recreate it would undo
// the migration that removed it.
var dropIndexRE = regexp.MustCompile(`(?is)^\s*DROP\s+INDEX\s+(?:IF\s+EXISTS\s+)?["` + "`" + `]?([A-Za-z0-9_]+)["` + "`" + `]?`)

// dropTableRE matches the DROP TABLE forms the migration set uses. Dropping a
// table drops every index on it with it, so those indexes are not "missing"
// either — the same reasoning as dropIndexRE, reached by a different route.
var dropTableRE = regexp.MustCompile(`(?is)^\s*DROP\s+TABLE\s+(?:IF\s+EXISTS\s+)?["` + "`" + `]?([A-Za-z0-9_]+)["` + "`" + `]?`)

// checkSchema compares the migration ledger and the declared indexes against
// what the database actually holds.
func (c *Checker) checkSchema(ctx context.Context, driver coredb.Driver) {
	c.setPhase(checkPhaseSchema)

	onDisk, err := coredb.MigrationNames()
	if err != nil {
		c.add(Finding{Key: "schema_read_failed", Family: familySchema, Severity: severityFail, Detail: sanitize(err, "")})
		return
	}

	applied, err := c.appliedMigrations(ctx)
	if err != nil {
		c.add(Finding{Key: "schema_ledger_failed", Family: familySchema, Severity: severityFail, Detail: sanitize(err, "")})
		return
	}

	diskSet := make(map[string]bool, len(onDisk))
	for _, n := range onDisk {
		diskSet[n] = true
	}
	// A squashed file's name stays in the ledger of a database that ran it
	// before the squash; it was merged away, not rolled back.
	for _, n := range coredb.RetiredMigrationNames() {
		diskSet[n] = true
	}

	var pending, unknown []string
	for _, n := range onDisk {
		if !applied[n] {
			pending = append(pending, n)
		}
	}
	for n := range applied {
		if !diskSet[n] {
			unknown = append(unknown, n)
		}
	}

	if len(pending) > 0 {
		c.add(Finding{
			Key:      "migrations_pending",
			Family:   familySchema,
			Severity: severityWarn,
			Count:    int64(len(pending)),
			Detail:   strings.Join(pending, ", "),
			Remedy:   remedyApplyMigrations,
		})
	}
	if len(unknown) > 0 {
		// The alarming direction: the binary is older than the database it
		// is pointed at, which is what a rolled-back deployment looks like.
		// No remedy — the fix is deploying the right version.
		c.add(Finding{
			Key:      "migrations_unknown",
			Family:   familySchema,
			Severity: severityWarn,
			Count:    int64(len(unknown)),
			Detail:   strings.Join(unknown, ", "),
		})
	}
	if len(pending) == 0 && len(unknown) == 0 {
		c.add(Finding{Key: "migrations_ok", Family: familySchema, Severity: severityOK, Count: int64(len(onDisk))})
	}

	c.checkIndexDrift(ctx, driver)
}

// appliedMigrations reads the schema_migrations ledger.
func (c *Checker) appliedMigrations(ctx context.Context) (map[string]bool, error) {
	runCtx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	// No LIMIT: the migration ledger is small and the check compares every applied name.
	rows, err := c.source.QueryContext(runCtx, "SELECT name FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer func() { logCloseErr(rows.Close()) }()

	applied := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		applied[name] = true
	}
	return applied, rows.Err()
}

// checkIndexDrift reports indexes the migration set declares and the database
// does not have.
func (c *Checker) checkIndexDrift(ctx context.Context, driver coredb.Driver) {
	declared, unparsed, err := declaredIndexes()
	if err != nil {
		c.add(Finding{Key: "index_read_failed", Family: familySchema, Severity: severityWarn, Detail: sanitize(err, "")})
		return
	}
	if unparsed > 0 {
		// Counted rather than assumed absent: treating an unparseable
		// statement as "no index declared" would under-report silently.
		c.add(Finding{
			Key:      "index_unparsed",
			Family:   familySchema,
			Severity: severitySkip,
			Reason:   "unparseable_statement",
			Count:    int64(unparsed),
		})
	}

	live, err := c.liveIndexes(ctx, driver)
	if err != nil {
		c.add(Finding{Key: "index_list_failed", Family: familySchema, Severity: severityWarn, Detail: sanitize(err, "")})
		return
	}

	missing := 0
	for name, decl := range declared {
		if live[strings.ToLower(name)] {
			continue
		}
		missing++
		c.add(Finding{
			Key:      "index_missing",
			Family:   familySchema,
			Severity: severityWarn,
			Table:    decl.Table,
			Index:    name,
			Remedy:   remedyCreateIndex,
		})
	}
	if unparsed == 0 && missing == 0 {
		// unparsed must be zero too: a clean pass over an incomplete parse is
		// not a clean pass, it is an unknown quantity of unexamined statements.
		c.add(Finding{Key: "indexes_ok", Family: familySchema, Severity: severityOK, Count: int64(len(declared))})
	}
}

// liveIndexes returns the index names the database actually holds, lowercased.
func (c *Checker) liveIndexes(ctx context.Context, driver coredb.Driver) (map[string]bool, error) {
	runCtx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	var query string
	switch driver {
	case coredb.DriverPostgres:
		query = `SELECT indexname FROM pg_indexes WHERE schemaname = current_schema()`
	case coredb.DriverMySQL:
		query = `SELECT DISTINCT index_name FROM information_schema.statistics WHERE table_schema = DATABASE()`
	default:
		query = `SELECT name FROM sqlite_master WHERE type = 'index' AND name NOT LIKE 'sqlite_%'`
	}

	rows, err := c.source.QueryContext(runCtx, query)
	if err != nil {
		return nil, err
	}
	defer func() { logCloseErr(rows.Close()) }()

	live := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		live[strings.ToLower(name)] = true
	}
	return live, rows.Err()
}

// stripLeadingComments removes leading -- and /* */ comment blocks and
// reports whether it reached the end of them; an unterminated comment is
// unresolved, which the caller counts rather than skipping in silence.
func stripLeadingComments(stmt string) (string, bool) {
	s := stmt
	for {
		trimmed := strings.TrimLeft(s, " \t\r\n")
		switch {
		case strings.HasPrefix(trimmed, "--"):
			nl := strings.IndexByte(trimmed, '\n')
			if nl == -1 {
				// A -- comment with no trailing newline still runs to a
				// well-defined end: the end of this chunk. Nothing can be
				// hiding beyond it, so this is the same "ran off the end"
				// case as consuming the last newline-terminated line —
				// benign, not a resolution failure.
				return "", true
			}
			s = trimmed[nl+1:]
		case strings.HasPrefix(trimmed, "/*"):
			end := strings.Index(trimmed, "*/")
			if end == -1 {
				// Same failure, block-comment form: never reached a close.
				return "", false
			}
			s = trimmed[end+2:]
		default:
			// Nothing left, or real content: either way the leading
			// comments (if any) were fully consumed.
			return trimmed, true
		}
	}
}

// declaredIndex is the net declaration of one index: the table it sits on and
// the CREATE statement that declares it. The statement is carried here rather
// than looked up again later, because a second pass would stop at the first
// migration naming the index and recreate a definition a later migration has
// already replaced.
type declaredIndex struct {
	Table string
	Stmt  string
}

// declaredIndexes maps every index name the migration set nets out to at its
// final state to its net declaration, and counts the CREATE/DROP INDEX and
// DROP TABLE statements it could not parse. Migrations run in name order, so a
// later DROP INDEX or DROP TABLE removes an earlier CREATE from the map — an
// index the schema deliberately retired, or that went down with its table, is
// not "missing", and a create_index remedy must never offer to undo a migration.
func declaredIndexes() (map[string]declaredIndex, int, error) {
	names, err := coredb.MigrationNames()
	if err != nil {
		return nil, 0, err
	}

	declared := map[string]declaredIndex{}
	unparsed := 0
	for _, name := range names {
		text, err := coredb.MigrationSQL(name)
		if err != nil {
			return nil, 0, err
		}
		for _, stmt := range coredb.SplitStatements(text) {
			trimmed, resolved := stripLeadingComments(stmt)
			if !resolved {
				// An unterminated comment hid whatever statement (if any)
				// followed it. Assuming "not an index statement" here would
				// silently under-report exactly like an unparseable
				// CREATE/DROP INDEX would.
				unparsed++
				continue
			}
			upper := strings.ToUpper(trimmed)
			// Classify on the leading keywords, not on whether the word
			// "index" appears anywhere in the statement — a CREATE TABLE
			// whose column comment happens to say "a partial unique index"
			// is not index DDL, and must not be counted as unparsed.
			fields := strings.Fields(upper)
			switch {
			case len(fields) >= 2 && fields[0] == "DROP" && fields[1] == "INDEX":
				m := dropIndexRE.FindStringSubmatch(trimmed)
				if m == nil {
					unparsed++
					continue
				}
				delete(declared, strings.ToLower(m[1]))
			case len(fields) >= 2 && fields[0] == "DROP" && fields[1] == "TABLE":
				m := dropTableRE.FindStringSubmatch(trimmed)
				if m == nil {
					unparsed++
					continue
				}
				table := strings.ToLower(m[1])
				for indexName, decl := range declared {
					if strings.ToLower(decl.Table) == table {
						delete(declared, indexName)
					}
				}
			case len(fields) >= 2 && fields[0] == "CREATE" && (fields[1] == "INDEX" || fields[1] == "UNIQUE"):
				m := createIndexRE.FindStringSubmatch(trimmed)
				if m == nil {
					unparsed++
					continue
				}
				// trimmed, not stmt: the comment-stripped form is what the
				// regex matched and what a repair can execute verbatim.
				declared[strings.ToLower(m[1])] = declaredIndex{Table: m[2], Stmt: trimmed}
			}
		}
	}
	return declared, unparsed, nil
}
