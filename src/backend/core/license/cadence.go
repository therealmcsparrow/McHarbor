// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package license

import "time"

// Notice phases. The phase is stored against each showing so the ledger
// shows not just that the customer was told, but how loudly.
const (
	PhaseDaily  = "daily"
	PhaseHourly = "hourly"
)

// EscalateAfter is how long a condition may stand before the reminder goes
// from daily to hourly. Two weeks is long enough to cover a holiday and
// short enough that nobody can claim they were not told.
const EscalateAfter = 14 * 24 * time.Hour

// Notice intervals, one per phase.
const (
	DailyInterval  = 24 * time.Hour
	HourlyInterval = time.Hour
)

// NextShowing decides whether a banner is due, and in which phase.
//
// The rule, verbatim from the brief: daily for the first fourteen days,
// hourly after that. `since` is when the condition began — the over-since
// clock for an overage, the expiry moment for an expired licence. A zero
// `lastShown` means it has never been shown, which is always due.
//
// Deliberately computed here rather than in the browser: a reload must not
// be able to re-trigger a showing, and two open tabs must not be able to
// double-count one.
func NextShowing(since, lastShown, now time.Time) (due bool, phase string) {
	phase = PhaseDaily
	interval := DailyInterval
	if !since.IsZero() && now.Sub(since) > EscalateAfter {
		phase = PhaseHourly
		interval = HourlyInterval
	}

	if lastShown.IsZero() {
		return true, phase
	}
	return now.Sub(lastShown) >= interval, phase
}
