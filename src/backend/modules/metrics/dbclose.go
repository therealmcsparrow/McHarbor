// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"strings"
)

// closeRows closes rows and logs a failure; it exists so deferred cleanups do not drop the error silently.
func closeRows(rows *sql.Rows) {
	if err := rows.Close(); err != nil {
		slog.Warn("closing metrics rows failed", "error", err)
	}
}

// closeStmt closes a prepared statement and logs a failure.
func closeStmt(stmt *sql.Stmt) {
	if err := stmt.Close(); err != nil {
		slog.Warn("closing metrics statement failed", "error", err)
	}
}

// rollbackTx rolls back tx, treating an already committed transaction as success.
func rollbackTx(tx *sql.Tx) {
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		slog.Warn("rolling back metrics transaction failed", "error", err)
	}
}

// closeBody closes a response body and logs a failure.
func closeBody(body io.Closer) {
	if err := body.Close(); err != nil {
		slog.Debug("closing metrics response body failed", "error", err)
	}
}

// expandSQL substitutes {{token}} placeholders in a query with fragments that come from code constants (bucket and identity expressions, scope clauses), never from request input.
func expandSQL(query string, pairs ...string) string {
	return strings.NewReplacer(pairs...).Replace(query)
}
