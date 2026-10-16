// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package closeutil logs failures from deferred cleanup calls that have no caller left to return an error to.
package closeutil

import (
	"database/sql"
	"errors"
	"log/slog"
	"os"
)

// Report logs err from a Close call at warn level; callers pass X.Close() inline so sqlclosecheck and bodyclose still see the Close.
func Report(err error, what string) {
	if err != nil && !errors.Is(err, os.ErrClosed) {
		slog.Warn("close failed", "resource", what, "error", err)
	}
}

// Rollback rolls tx back and logs any failure other than the transaction already being committed or rolled back.
func Rollback(tx *sql.Tx) {
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		slog.Warn("rollback failed", "error", err)
	}
}

// Remove deletes path and logs a failure other than the path already being gone.
func Remove(path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Warn("remove failed", "path", path, "error", err)
	}
}

// RemoveAll deletes path and its children and logs any failure.
func RemoveAll(path string) {
	if err := os.RemoveAll(path); err != nil {
		slog.Warn("remove failed", "path", path, "error", err)
	}
}
