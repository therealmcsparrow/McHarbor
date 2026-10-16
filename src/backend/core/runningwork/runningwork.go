// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package runningwork defines, once, the work a McHarbor restart would kill:
// the rows the pre-upgrade check and the self-update scheduler both inspect, so
// a new kind of long job is added in one place rather than remembered in two.
package runningwork

import (
	"context"
	"database/sql"
	"fmt"
)

// Kind names one sort of in-flight work.
type Kind string

// The kinds of work a restart would interrupt.
const (
	// ContainerBackups are container backup and restore runs.
	ContainerBackups Kind = "containerBackups"
	// InstanceExports are instance exports and their verifications.
	InstanceExports Kind = "instanceExports"
)

// queries maps each kind to the count of its running rows.
var queries = []struct {
	kind  Kind
	query string
}{
	{ContainerBackups, `SELECT COUNT(*) FROM container_backup_runs WHERE status = 'running'`},
	{InstanceExports, `SELECT COUNT(*) FROM instance_backup_exports WHERE status = 'running' OR verify_status = 'running'`},
}

// Result is one kind's count, or the error that kept it from being read. A
// caller decides what an unreadable count means; both current callers treat it
// as busy, because restarting over work that could not be ruled out is the risk.
type Result struct {
	Kind  Kind
	Count int
	Err   error
}

// Count reads every kind in a fixed order.
func Count(ctx context.Context, db *sql.DB) []Result {
	out := make([]Result, 0, len(queries))
	// bounded by queries, a two-entry literal above.
	for _, q := range queries {
		var n int
		if err := db.QueryRowContext(ctx, q.query).Scan(&n); err != nil {
			out = append(out, Result{Kind: q.kind, Err: fmt.Errorf("counting %s: %w", q.kind, err)})
			continue
		}
		out = append(out, Result{Kind: q.kind, Count: n})
	}
	return out
}
