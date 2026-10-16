// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"context"
	"time"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

// optimizeInterval is SQLite's own suggestion for a long-lived connection:
// PRAGMA optimize once a day, after the open-time run core/db already did.
const optimizeInterval = 24 * time.Hour

// maybeOptimize refreshes the planner statistics once a day, on the leader's
// tick after the retention prunes have moved the row counts. The first call only
// starts the clock, because opening the database ran one moments earlier; a
// failure is logged and the tick goes on. It reports whether a refresh ran.
func (c *Collector) maybeOptimize(ctx context.Context, now time.Time) bool {
	if c.optimizeAt.IsZero() {
		c.optimizeAt = now
		return false
	}
	if now.Sub(c.optimizeAt) < optimizeInterval {
		return false
	}
	c.optimizeAt = now
	run := c.optimize
	if run == nil {
		run = func(ctx context.Context) error { return coredb.Optimize(ctx, c.db, coredb.DriverOf()) }
	}
	if err := run(ctx); err != nil {
		c.logger.Warn("metrics collector: daily planner-statistics refresh failed", "error", err)
	}
	return true
}
