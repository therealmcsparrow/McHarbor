// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"
)

const (
	sqlQueryMaxRows  = 200
	sqlQueryTimeout  = 5 * time.Second
	sqlResetTimeout  = 1 * time.Second
)

var (
	sqlQueryDangerousPattern = regexp.MustCompile(`(?i)\b(insert|update|delete|replace|alter|create|drop|attach|detach|pragma|vacuum|analyze|reindex|begin|commit|rollback|savepoint|release)\b`)
	sqlQueryTablePattern     = regexp.MustCompile(`(?i)\b(?:from|join)\s+([a-z_][a-z0-9_]*)\b`)
	sqlQueryAllowedTables    = map[string]struct{}{
		"workflow_kv":            {},
		"workflow_link_messages": {},
		"workflow_metrics":       {},
		"workflow_runs":          {},
		"workflows":              {},
	}
)

func validateWorkflowSQLQuery(query string) (string, string) {
	normalized := strings.TrimSpace(query)
	if normalized == "" {
		return "", "query is required"
	}

	if strings.HasSuffix(normalized, ";") {
		normalized = strings.TrimSpace(strings.TrimSuffix(normalized, ";"))
	}
	if normalized == "" {
		return "", "query is required"
	}
	if strings.Contains(normalized, ";") {
		return "", "query must contain a single statement"
	}

	upper := strings.ToUpper(normalized)
	if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "WITH") {
		return "", "only SELECT and WITH queries are allowed"
	}
	if sqlQueryDangerousPattern.MatchString(normalized) {
		return "", "query contains a statement that is not allowed"
	}

	matches := sqlQueryTablePattern.FindAllStringSubmatch(normalized, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		tableName := strings.ToLower(match[1])
		if _, ok := sqlQueryAllowedTables[tableName]; !ok {
			return "", fmt.Sprintf("table %q is not available to workflow SQL queries", tableName)
		}
	}

	return normalized, ""
}

func openReadOnlyWorkflowSQLRows(ctx context.Context, db *sql.DB, logger *slog.Logger, query string) (*sql.Rows, func(), error) {
	queryCtx, cancel := context.WithTimeout(ctx, sqlQueryTimeout)
	conn, err := db.Conn(queryCtx)
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("opening workflow sql connection: %w", err)
	}

	cleanup := func() {
		resetCtx, resetCancel := context.WithTimeout(context.Background(), sqlResetTimeout)
		defer resetCancel()

		if _, resetErr := conn.ExecContext(resetCtx, "PRAGMA query_only = OFF"); resetErr != nil && logger != nil {
			logger.Warn("workflows: failed to reset SQL query connection", "error", resetErr)
		}
		if closeErr := conn.Close(); closeErr != nil && logger != nil {
			logger.Warn("workflows: failed to close SQL query connection", "error", closeErr)
		}
		cancel()
	}

	if _, err := conn.ExecContext(queryCtx, "PRAGMA query_only = ON"); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("enabling read-only workflow sql mode: %w", err)
	}

	rows, err := conn.QueryContext(queryCtx, query)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("executing workflow sql query: %w", err)
	}

	return rows, cleanup, nil
}

