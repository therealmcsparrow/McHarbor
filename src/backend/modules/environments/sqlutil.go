// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package environments

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// logCloseErr reports a failed Close of a sql resource through slog, because a deferred close has no caller to return the error to.
func logCloseErr(err error) {
	if err != nil {
		slog.Warn("closing sql resource failed", "error", err)
	}
}

// inTx runs fn in a transaction bound to ctx and rolls it back on error, so a cancelled request cannot leave a half-applied write.
func inTx(ctx context.Context, conn *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer db.RollbackTx(tx)
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
