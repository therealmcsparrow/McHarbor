// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package database

import "log/slog"

// logCloseErr reports a failed Close of a sql resource through slog, because a deferred close has no caller to return the error to.
func logCloseErr(err error) {
	if err != nil {
		slog.Warn("closing sql resource failed", "error", err)
	}
}
