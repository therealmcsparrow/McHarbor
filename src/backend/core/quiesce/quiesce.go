// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package quiesce is the process-wide switch that stops McHarbor writing to
// its own database.
//
// It exists for one job: copying the database to another backend. The HTTP
// layer is easy to hold still — one middleware answers 503 and every request
// stops. The background workers are the problem. The metrics collector, the
// activity collector, the alerts and autoheal engines, the audit workers, the
// schedulers and the watchers all hold *sql.DB directly and tick on their own
// timers, so a copy that only paused HTTP would still lose whatever they wrote
// after their table had been read.
//
// The gate has two halves, and both are needed:
//
//   - Begin() stops new ticks. A worker calls Run() and, while the gate is
//     closed, simply does not run this time round. Skipping a tick is always
//     safe here: every one of these workers is idempotent by construction,
//     because it already has to survive a process restart between any two
//     ticks.
//   - Drain() waits for the ticks that were already running when Begin() was
//     called. Without it the copy would start while a collector was halfway
//     through a batch of inserts, which is the exact race the gate is for.
//
// Nothing here touches the database. A worker that ignores the gate is not
// broken, it is just not covered — which is why the migration also runs delta
// passes afterwards.
package quiesce

import (
	"context"
	"sync"
	"time"
)

// state is the single package-level gate. A package-level variable rather than
// an injected dependency because the alternative is threading one flag through
// fourteen constructors that have nothing else in common.
var state struct {
	mu     sync.Mutex
	paused bool
	reason string
	// inFlight counts workers between Run's entry and its exit.
	inFlight int
}

// Begin closes the gate. Ticks that have not started yet will be skipped;
// ticks already running are unaffected until Drain is called. Safe to call
// more than once — the last reason wins.
func Begin(reason string) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.paused = true
	state.reason = reason
}

// End reopens the gate. Workers resume on their next tick; none of them is
// caught up or replayed, because each one re-reads whatever it needs from the
// database when it next runs.
func End() {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.paused = false
	state.reason = ""
}

// Active reports whether the gate is closed. Read by the HTTP maintenance
// middleware so the request path and the background path share one switch
// rather than two that can disagree.
func Active() bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.paused
}

// Reason returns the label passed to Begin, or "" when the gate is open.
func Reason() string {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.reason
}

// InFlight is how many workers are inside Run right now. Exposed for the
// migration's progress readout, which shows the operator what it is waiting
// for rather than an unexplained pause.
func InFlight() int {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.inFlight
}

// Run executes work unless the gate is closed, and reports whether it ran.
//
// This is the whole worker-facing API. A tick body becomes:
//
//	case <-ticker.C:
//		quiesce.Run(func() { c.tick(ctx) })
//
// The closure form matters: it makes the exit path a defer rather than
// something each call site has to remember, so a worker cannot leak an
// in-flight count by returning early or panicking.
func Run(work func()) bool {
	if !enter() {
		return false
	}
	defer leave()
	work()
	return true
}

// enter registers a worker as in-flight, unless the gate is closed.
func enter() bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.paused {
		return false
	}
	state.inFlight++
	return true
}

// leave deregisters a worker.
func leave() {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.inFlight > 0 {
		state.inFlight--
	}
}

// Drain waits until no worker is in flight, or until ctx is done.
//
// It polls rather than waiting on a condition variable because sync.Cond
// cannot be cancelled, and a drain that cannot give up would turn one wedged
// worker into a migration that hangs forever with no way out. 25ms is far
// below the interval of every worker involved, so the poll is invisible next
// to the work it is waiting on.
//
// Returns ctx.Err() if it gave up, which the caller treats as "a worker is
// still running" rather than as a failure — the delta passes cover it.
func Drain(ctx context.Context) error {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if InFlight() == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
