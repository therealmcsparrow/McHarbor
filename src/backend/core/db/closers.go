// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"database/sql"
	"errors"
	"log/slog"
)

// CloseRows closes rows and logs a failure, because a deferred Close has no caller to return the error to.
func CloseRows(rows *sql.Rows) {
	if err := rows.Close(); err != nil {
		slog.Warn("closing sql rows failed", "error", err)
	}
}

// RollbackTx rolls tx back and logs a failure, treating sql.ErrTxDone as success since a committed transaction has nothing left to undo.
func RollbackTx(tx *sql.Tx) {
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		slog.Warn("rolling back transaction failed", "error", err)
	}
}
