// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lxc

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// countingMeasure reports size and counts its calls; gate, when set, holds each
// call until it is closed.
type countingMeasure struct {
	calls atomic.Int32
	size  atomic.Uint64
	fail  atomic.Bool
	gate  chan struct{}
}

func (c *countingMeasure) measure(ctx context.Context, _ string) (uint64, error) {
	c.calls.Add(1)
	if c.gate != nil {
		select {
		case <-c.gate:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	if c.fail.Load() {
		return 0, errors.New("walk failed")
	}
	return c.size.Load(), nil
}

func waitForCalls(t *testing.T, m *countingMeasure, want int32) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for m.calls.Load() < want {
		if time.Now().After(deadline) {
			t.Fatalf("measure called %d times, want %d", m.calls.Load(), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestRootfsUsageIsMeasuredOnceAndServedFromTheCache(t *testing.T) {
	measure := &countingMeasure{}
	measure.size.Store(4096)
	cache := newRootfsUsageCache(measure.measure)

	if got := cache.get(context.Background(), "ct1", "/pool/ct1/rootfs", time.Second); got != 4096 {
		t.Fatalf("first sample = %d, want the measured 4096", got)
	}
	measure.size.Store(8192)
	for i := 0; i < 10; i++ {
		if got := cache.get(context.Background(), "ct1", "/pool/ct1/rootfs", time.Second); got != 4096 {
			t.Fatalf("a fresh entry was re-measured: %d", got)
		}
	}
	time.Sleep(100 * time.Millisecond)
	if got := cache.get(context.Background(), "ct1", "/pool/ct1/rootfs", time.Second); got != 4096 {
		t.Fatalf("a fresh entry was re-measured in the background: %d", got)
	}
	if calls := measure.calls.Load(); calls != 1 {
		t.Fatalf("measured %d times within the TTL, want 1", calls)
	}
}

func TestAStaleRootfsUsageIsRefreshedInTheBackground(t *testing.T) {
	measure := &countingMeasure{}
	measure.size.Store(100)
	cache := newRootfsUsageCache(measure.measure)
	now := time.Date(2026, 9, 27, 19, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	cache.now = func() time.Time { mu.Lock(); defer mu.Unlock(); return now }

	cache.get(context.Background(), "ct1", "/r", time.Second)
	mu.Lock()
	now = now.Add(rootfsUsageTTL + time.Second)
	mu.Unlock()
	measure.size.Store(200)
	measure.gate = make(chan struct{})

	if got := cache.get(context.Background(), "ct1", "/r", time.Second); got != 100 {
		t.Fatalf("a stale sample waited for the walk: %d, want the old 100 at once", got)
	}
	cache.get(context.Background(), "ct1", "/r", time.Second)
	waitForCalls(t, measure, 2)
	close(measure.gate)
	deadline := time.Now().Add(5 * time.Second)
	for cache.get(context.Background(), "ct1", "/r", 0) != 200 {
		if time.Now().After(deadline) {
			t.Fatal("the background refresh never landed")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if calls := measure.calls.Load(); calls != 2 {
		t.Fatalf("two stale samples started %d walks, want one refresh", calls-1)
	}
}

func TestTheFirstRootfsUsageWaitIsBounded(t *testing.T) {
	measure := &countingMeasure{gate: make(chan struct{})}
	defer close(measure.gate)
	cache := newRootfsUsageCache(measure.measure)
	start := time.Now()
	if got := cache.get(context.Background(), "ct1", "/r", 50*time.Millisecond); got != 0 {
		t.Fatalf("an unfinished first walk reported %d", got)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("the sample waited %v for a slow walk", elapsed)
	}
}

func TestForgetDropsARootfsSizeAndAFailedFirstWalkReportsNone(t *testing.T) {
	measure := &countingMeasure{}
	measure.size.Store(700)
	cache := newRootfsUsageCache(measure.measure)
	cache.get(context.Background(), "ct1", "/r", time.Second)

	cache.forget("ct1")
	measure.fail.Store(true)
	if got := cache.get(context.Background(), "ct1", "/r", time.Second); got != 0 {
		t.Fatalf("a forgotten container still reported %d", got)
	}
	if got := cache.get(context.Background(), "", "", time.Second); got != 0 {
		t.Fatalf("no rootfs reported %d", got)
	}
}
