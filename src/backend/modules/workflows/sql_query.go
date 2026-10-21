// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

const (
	sqlQueryMaxRows = 200
	sqlQueryTimeout = 5 * time.Second
	sqlResetTimeout = 1 * time.Second

	// sqliteOpflagP2IsReg is SQLite's OPFLAG_P2ISREG: the root page sits in a register,
	// so the EXPLAIN listing cannot show which b-tree the cursor opens.
	sqliteOpflagP2IsReg = 0x10
)

// ErrSQLQueryRequired, and the sentinels below it, are the refusals the sql-query node
// reports to the workflow; their text is the node's error payload, so it names no schema.
var (
	ErrSQLQueryRequired            = errors.New("query is required")
	ErrSQLQueryMultipleStatements  = errors.New("query must contain a single statement")
	ErrSQLQueryNotSelect           = errors.New("only SELECT and WITH queries are allowed")
	ErrSQLQueryForbiddenStatement  = errors.New("query contains a statement that is not allowed")
	ErrSQLQueryTableNotAllowed     = errors.New("query reads a table that is not available to workflow SQL queries")
	ErrSQLQueryVirtualTableRefused = errors.New("query uses a table-valued function or virtual table, which workflow SQL queries cannot read")
)

var (
	// sqlQueryDangerousPattern is a cheap first filter with a readable refusal; the
	// boundary is guardWorkflowSQLQuery, which reads what SQLite actually compiled.
	sqlQueryDangerousPattern = regexp.MustCompile(`(?i)\b(insert|update|delete|replace|alter|create|drop|attach|detach|pragma|vacuum|analyze|reindex|begin|commit|rollback|savepoint|release)\b`)
	sqlQueryAllowedTables    = map[string]struct{}{
		"workflow_kv":            {},
		"workflow_link_messages": {},
		"workflow_metrics":       {},
		"workflow_runs":          {},
		"workflows":              {},
	}

	// sqlQueryScratchOpcodes open cursors on transient structures that never reach a
	// stored table, so they need no root-page check.
	sqlQueryScratchOpcodes = map[string]struct{}{
		"OpenEphemeral": {},
		"OpenAutoindex": {},
		"OpenPseudo":    {},
		"OpenDup":       {},
	}

	// sqlQueryRefusedOpcodes write, run sub-programs or change the schema; query_only
	// should already stop them, and the guard refuses them before execution regardless.
	sqlQueryRefusedOpcodes = map[string]struct{}{
		"OpenWrite":    {},
		"Program":      {},
		"ParseSchema":  {},
		"CreateBtree":  {},
		"Destroy":      {},
		"Clear":        {},
		"DropTable":    {},
		"DropIndex":    {},
		"DropTrigger":  {},
		"SetCookie":    {},
		"Vacuum":       {},
		"IncrVacuum":   {},
		"JournalMode":  {},
		"Checkpoint":   {},
		"SqlExec":      {},
		"IntegrityCk":  {},
		"LoadAnalysis": {},
		"Expire":       {},
	}

	sqlQueryResetStatements = []string{"ROLLBACK", "PRAGMA query_only = OFF"}
)

// sqlQueryRootPagesQuery is the guard's own lookup, run before the user query and never
// through the guard; the allow-list arrives as one JSON array bound to its placeholder.
const sqlQueryRootPagesQuery = `SELECT rootpage FROM main.sqlite_schema
	WHERE type IN ('table', 'index') AND rootpage > 0
	AND tbl_name IN (SELECT value FROM json_each(?))` // No LIMIT: bounded by the allow-listed tables and their indexes

var sqlQueryRefusals = []error{
	ErrSQLQueryRequired,
	ErrSQLQueryMultipleStatements,
	ErrSQLQueryNotSelect,
	ErrSQLQueryForbiddenStatement,
	ErrSQLQueryTableNotAllowed,
	ErrSQLQueryVirtualTableRefused,
}

// isWorkflowSQLRefusal reports whether err is one of the node's refusal sentinels.
func isWorkflowSQLRefusal(err error) bool {
	return workflowSQLRefusalMessage(err) != ""
}

// workflowSQLRefusalMessage returns the sentinel's own text, never the wrapped detail.
func workflowSQLRefusalMessage(err error) string {
	for _, sentinel := range sqlQueryRefusals {
		if errors.Is(err, sentinel) {
			return sentinel.Error()
		}
	}
	return ""
}

func validateWorkflowSQLQuery(query string) (string, error) {
	normalized := strings.TrimSpace(query)
	if normalized == "" {
		return "", ErrSQLQueryRequired
	}

	if strings.HasSuffix(normalized, ";") {
		normalized = strings.TrimSpace(strings.TrimSuffix(normalized, ";"))
	}
	if normalized == "" {
		return "", ErrSQLQueryRequired
	}
	// Every statement separator is a ';' character, so refusing any remaining one keeps
	// EXPLAIN and execution on exactly one statement, whatever quoting surrounds it.
	if strings.Contains(normalized, ";") {
		return "", ErrSQLQueryMultipleStatements
	}

	upper := strings.ToUpper(normalized)
	if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "WITH") {
		return "", ErrSQLQueryNotSelect
	}
	if sqlQueryDangerousPattern.MatchString(normalized) {
		return "", ErrSQLQueryForbiddenStatement
	}

	return normalized, nil
}

// allowedWorkflowSQLRootPages returns the b-tree root pages of the allow-listed tables and
// every index on them, read inside the caller's transaction so EXPLAIN sees the same schema.
func allowedWorkflowSQLRootPages(ctx context.Context, conn *sql.Conn) (map[int64]struct{}, error) {
	names := make([]string, 0, len(sqlQueryAllowedTables))
	for name := range sqlQueryAllowedTables {
		names = append(names, name)
	}
	sort.Strings(names)
	encoded, err := json.Marshal(names)
	if err != nil {
		return nil, fmt.Errorf("encoding workflow sql allow-list: %w", err)
	}

	rows, err := conn.QueryContext(ctx, sqlQueryRootPagesQuery, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("reading workflow sql root pages: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()

	pages := make(map[int64]struct{}, len(names)*2)
	for rows.Next() {
		var page int64
		if err := rows.Scan(&page); err != nil {
			return nil, fmt.Errorf("scanning workflow sql root page: %w", err)
		}
		pages[page] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating workflow sql root pages: %w", err)
	}
	return pages, nil
}

// guardWorkflowSQLQuery compiles query with EXPLAIN and refuses any cursor on a b-tree that
// is not an allow-listed table or its index, any other schema, and any virtual table; the
// check reads SQLite's own name resolution, so quoting, comments and aliases cannot hide a table.
func guardWorkflowSQLQuery(ctx context.Context, conn *sql.Conn, query string) error {
	allowed, err := allowedWorkflowSQLRootPages(ctx, conn)
	if err != nil {
		return err
	}

	rows, err := conn.QueryContext(ctx, "EXPLAIN "+query)
	if err != nil {
		return fmt.Errorf("explaining workflow sql query: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()

	for rows.Next() {
		var (
			addr, p1, p2, p3, p5 int64
			opcode               string
			p4, comment          any
		)
		if err := rows.Scan(&addr, &opcode, &p1, &p2, &p3, &p4, &p5, &comment); err != nil {
			return fmt.Errorf("scanning workflow sql program: %w", err)
		}
		if err := checkWorkflowSQLOpcode(opcode, p2, p3, p5, allowed); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating workflow sql program: %w", err)
	}
	return nil
}

func checkWorkflowSQLOpcode(opcode string, p2, p3, p5 int64, allowed map[int64]struct{}) error {
	if _, refused := sqlQueryRefusedOpcodes[opcode]; refused {
		return fmt.Errorf("%w: opcode %s", ErrSQLQueryForbiddenStatement, opcode)
	}
	// Virtual-table opcodes are VOpen, VFilter, VColumn, ...; Variable is the one opcode
	// that starts with V and is not one of them.
	if len(opcode) > 1 && opcode[0] == 'V' && opcode[1] >= 'A' && opcode[1] <= 'Z' {
		return fmt.Errorf("%w: opcode %s", ErrSQLQueryVirtualTableRefused, opcode)
	}
	if _, scratch := sqlQueryScratchOpcodes[opcode]; scratch {
		return nil
	}
	if opcode != "OpenRead" && opcode != "ReopenIdx" {
		if strings.HasPrefix(opcode, "Open") {
			return fmt.Errorf("%w: opcode %s", ErrSQLQueryForbiddenStatement, opcode)
		}
		return nil
	}
	if p3 != 0 || p5&sqliteOpflagP2IsReg != 0 {
		return fmt.Errorf("%w: schema %d", ErrSQLQueryTableNotAllowed, p3)
	}
	if _, ok := allowed[p2]; !ok {
		return fmt.Errorf("%w: root page %d", ErrSQLQueryTableNotAllowed, p2)
	}
	return nil
}

// releaseWorkflowSQLConn ends the read transaction and clears query_only before the
// connection returns to the pool; a connection that cannot be reset is discarded instead,
// because the pool would otherwise hand a read-only, mid-transaction connection to a writer.
func releaseWorkflowSQLConn(ctx context.Context, conn *sql.Conn, logger *slog.Logger, statements []string) {
	resetCtx, resetCancel := context.WithTimeout(context.WithoutCancel(ctx), sqlResetTimeout)
	defer resetCancel()

	var resetErr error
	for _, statement := range statements {
		if _, err := conn.ExecContext(resetCtx, statement); err != nil {
			// ROLLBACK fails harmlessly when the transaction never started.
			if statement == "ROLLBACK" && strings.Contains(err.Error(), "no transaction is active") {
				continue
			}
			resetErr = fmt.Errorf("resetting workflow sql connection (%s): %w", statement, err)
			break
		}
	}

	if resetErr != nil {
		if logger != nil {
			logger.Warn("workflows: discarding SQL query connection that could not be reset", "error", resetErr)
		}
		rawErr := conn.Raw(func(any) error { return driver.ErrBadConn })
		if rawErr != nil && !errors.Is(rawErr, driver.ErrBadConn) && logger != nil {
			logger.Warn("workflows: failed to discard SQL query connection", "error", rawErr)
		}
	}

	if closeErr := conn.Close(); closeErr != nil && !errors.Is(closeErr, sql.ErrConnDone) && logger != nil {
		logger.Warn("workflows: failed to close SQL query connection", "error", closeErr)
	}
}

func openReadOnlyWorkflowSQLRows(ctx context.Context, db *sql.DB, logger *slog.Logger, query string) (*sql.Rows, func(), error) {
	queryCtx, cancel := context.WithTimeout(ctx, sqlQueryTimeout)
	conn, err := db.Conn(queryCtx)
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("opening workflow sql connection: %w", err)
	}

	cleanup := func() {
		releaseWorkflowSQLConn(ctx, conn, logger, sqlQueryResetStatements)
		cancel()
	}

	if _, err := conn.ExecContext(queryCtx, "PRAGMA query_only = ON"); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("enabling read-only workflow sql mode: %w", err)
	}
	// One read transaction pins one snapshot, so the root pages the guard checked are the
	// ones the query runs against even if another connection changes the schema meanwhile.
	if _, err := conn.ExecContext(queryCtx, "BEGIN"); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("starting workflow sql read transaction: %w", err)
	}
	if err := guardWorkflowSQLQuery(queryCtx, conn, query); err != nil {
		cleanup()
		return nil, nil, err
	}

	rows, err := conn.QueryContext(queryCtx, query)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("executing workflow sql query: %w", err)
	}

	return rows, cleanup, nil
}
