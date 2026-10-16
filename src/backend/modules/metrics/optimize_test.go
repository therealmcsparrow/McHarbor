// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

type stubCoordinator struct{ leader bool }

func (s stubCoordinator) LeaderOf(context.Context, string) bool { return s.leader }
func (s stubCoordinator) NodeID() string                        { return "node-test" }

// TestMaybeOptimizeRunsOncePerDay pins the daily schedule: the first tick only
// starts the clock (the open already optimized), then one run per 24 hours, and
// a failing run neither stops the schedule nor retries on the next tick.
func TestMaybeOptimizeRunsOncePerDay(t *testing.T) {
	calls := 0
	c := &Collector{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	c.optimize = func(context.Context) error {
		calls++
		return errors.New("database is locked")
	}
	start := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	steps := []struct {
		after time.Duration
		ran   bool
	}{
		{0, false},
		{30 * time.Second, false},
		{23*time.Hour + 59*time.Minute, false},
		{24 * time.Hour, true},
		{24*time.Hour + 30*time.Second, false},
		{47 * time.Hour, false},
		{48 * time.Hour, true},
	}
	for _, s := range steps {
		if got := c.maybeOptimize(context.Background(), start.Add(s.after)); got != s.ran {
			t.Fatalf("at +%s: ran=%v, want %v", s.after, got, s.ran)
		}
	}
	if calls != 2 {
		t.Fatalf("optimize ran %d times over 48 hours, want 2", calls)
	}
}

// TestOptimizeOnlyOnTheLeader guards the multi-node case: a follower's tick
// returns before any maintenance, so only one node refreshes statistics.
func TestOptimizeOnlyOnTheLeader(t *testing.T) {
	calls := 0
	c := &Collector{
		logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		coordinator: stubCoordinator{leader: false},
		hasCoord:    true,
		optimizeAt:  time.Now().Add(-72 * time.Hour),
	}
	c.optimize = func(context.Context) error { calls++; return nil }
	c.tick(context.Background())
	if calls != 0 {
		t.Fatalf("a follower ran optimize %d times", calls)
	}
}
