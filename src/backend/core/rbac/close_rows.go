// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"database/sql"
	"log/slog"
)

// closeRows closes rows and logs a failure, because a deferred Close has no caller to return the error to.
func closeRows(rows *sql.Rows) {
	if err := rows.Close(); err != nil {
		slog.Warn("closing sql rows failed", "error", err)
	}
}
