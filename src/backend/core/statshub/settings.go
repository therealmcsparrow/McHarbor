// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package statshub

import (
	"context"
	"database/sql"
	"strconv"
	"time"
)

// PushIntervalKey is the settings row holding the operator's push interval in
// seconds, in the "agent" category beside agent_metrics_enabled.
const PushIntervalKey = "agent_stats_push_interval"

// DefaultPushInterval is used until an operator sets one: frequent enough for
// the containers page's live columns, three pushes per raw history sample.
const DefaultPushInterval = 10 * time.Second

// ReadPushInterval reads the configured push interval, falling back to the
// default for a missing or out-of-range value. It reads the row directly
// because core/settings is linked by plugins and must not grow for this.
func ReadPushInterval(db *sql.DB) time.Duration {
	return ReadPushIntervalContext(context.Background(), db)
}

// ReadPushIntervalContext is ReadPushInterval bound to the caller's context, so a cancelled request stops the settings read.
func ReadPushIntervalContext(ctx context.Context, db *sql.DB) time.Duration {
	var raw string
	if err := db.QueryRowContext(ctx,
		"SELECT value FROM settings WHERE category = 'agent' AND key = ? LIMIT 1", PushIntervalKey,
	).Scan(&raw); err != nil {
		return DefaultPushInterval
	}
	secs, err := strconv.Atoi(raw)
	if err != nil {
		return DefaultPushInterval
	}
	d := time.Duration(secs) * time.Second
	if d < MinPushInterval || d > MaxPushInterval {
		return DefaultPushInterval
	}
	return d
}
