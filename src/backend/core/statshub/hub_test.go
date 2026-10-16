// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package statshub

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func newClock() *clock { return &clock{now: time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)} }

func samples(n int, tag string) []Sample {
	out := make([]Sample, n)
	for i := range out {
		out[i] = Sample{ID: fmt.Sprintf("%s-%d", tag, i)}
	}
	return out
}

func TestConcurrentFirstReadsShareOnePull(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	h := New(Options{Pull: func(ctx context.Context, envID string) ([]Sample, error) {
		calls.Add(1)
		<-release
		return samples(3, "pull"), nil
	}})
	defer h.Close()

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			snap, err := h.Snapshot(context.Background(), "env")
			if err != nil || len(snap.Samples) != 3 {
				t.Errorf("snapshot = %v, %v", snap, err)
			}
		}()
	}
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()
	if got := calls.Load(); got != 1 {
		t.Fatalf("20 concurrent readers caused %d pulls, want 1", got)
	}
}

func TestStalePullIsServedWhileOneRefreshRuns(t *testing.T) {
	clk := newClock()
	var calls atomic.Int32
	block := make(chan struct{})
	h := New(Options{Now: clk.Now, RefreshAfter: 4 * time.Second, Pull: func(ctx context.Context, envID string) ([]Sample, error) {
		n := calls.Add(1)
		if n > 1 {
			<-block
		}
		return samples(int(n), "pull"), nil
	}})
	defer h.Close()

	if _, err := h.Snapshot(context.Background(), "env"); err != nil {
		t.Fatal(err)
	}
	clk.Advance(10 * time.Second)
	for range 5 {
		snap, err := h.Snapshot(context.Background(), "env")
		if err != nil {
			t.Fatal(err)
		}
		if len(snap.Samples) != 1 {
			t.Fatalf("stale read returned %d samples, want the cached 1 without waiting", len(snap.Samples))
		}
	}
	close(block)
	h.Close()
	if got := calls.Load(); got != 2 {
		t.Fatalf("five stale reads caused %d pulls, want 1 initial + 1 refresh", got)
	}
}

func TestPushOrderingIsIdempotent(t *testing.T) {
	clk := newClock()
	h := New(Options{Now: clk.Now, Pull: func(context.Context, string) ([]Sample, error) {
		return nil, errors.New("pull must not run while pushes are fresh")
	}})
	defer h.Close()

	if r := h.Push("env", Batch{Session: "s1", Seq: 1, Samples: samples(1, "a")}); !r.Applied {
		t.Fatalf("first batch not applied: %+v", r)
	}
	if r := h.Push("env", Batch{Session: "s1", Seq: 2, Samples: samples(2, "b")}); !r.Applied {
		t.Fatalf("second batch not applied: %+v", r)
	}
	if r := h.Push("env", Batch{Session: "s1", Seq: 2, Samples: samples(9, "dup")}); r.Applied || !r.Duplicate {
		t.Fatalf("resent seq 2 = %+v, want duplicate and not applied", r)
	}
	if r := h.Push("env", Batch{Session: "s1", Seq: 1, Samples: samples(9, "old")}); r.Applied || !r.Duplicate {
		t.Fatalf("late seq 1 = %+v, want duplicate and not applied", r)
	}
	snap, err := h.Snapshot(context.Background(), "env")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Mode != ModePush || len(snap.Samples) != 2 {
		t.Fatalf("snapshot = %d samples via %s, want batch 2 via push", len(snap.Samples), snap.Mode)
	}
	if r := h.Push("env", Batch{Session: "s2", Seq: 1, Samples: samples(4, "c")}); !r.Applied {
		t.Fatalf("a reconnect's first batch must start a new sequence: %+v", r)
	}
}

func TestSilentPushFallsBackToPull(t *testing.T) {
	clk := newClock()
	h := New(Options{Now: clk.Now, Pull: func(context.Context, string) ([]Sample, error) {
		return samples(7, "pull"), nil
	}})
	defer h.Close()

	r := h.Push("env", Batch{Session: "s", Seq: 1, Samples: samples(1, "push")})
	if !h.PushActive("env") {
		t.Fatal("push not active right after a batch")
	}
	clk.Advance(3*r.Interval + time.Second)
	if h.PushActive("env") {
		t.Fatal("push still active after three missed intervals")
	}
	snap, err := h.Snapshot(context.Background(), "env")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Mode != ModePull || len(snap.Samples) != 7 {
		t.Fatalf("stale push served %s/%d, want a fresh pull", snap.Mode, len(snap.Samples))
	}
}

func TestBackpressureStretchesTheInterval(t *testing.T) {
	clk := newClock()
	h := New(Options{Now: clk.Now, BaseInterval: func() time.Duration { return 5 * time.Second }, Pull: func(context.Context, string) ([]Sample, error) { return nil, nil }})
	defer h.Close()

	var last PushResult
	for i := range 100 {
		last = h.Push(fmt.Sprintf("env-%d", i), Batch{Session: "s", Seq: 1})
	}
	if last.Interval < 10*time.Second {
		t.Fatalf("100 agents at 5s is 20 batches/s; interval %s must stretch to at least 10s", last.Interval)
	}
	small := New(Options{Now: clk.Now, BaseInterval: func() time.Duration { return 5 * time.Second }, Pull: func(context.Context, string) ([]Sample, error) { return nil, nil }})
	defer small.Close()
	if r := small.Push("one", Batch{Session: "s", Seq: 1}); r.Interval != 5*time.Second {
		t.Fatalf("a lone agent was slowed to %s", r.Interval)
	}
}

func TestForgetEndsPushMode(t *testing.T) {
	h := New(Options{Pull: func(context.Context, string) ([]Sample, error) { return samples(2, "pull"), nil }})
	defer h.Close()
	h.Push("env", Batch{Session: "s", Seq: 5, Samples: samples(1, "push")})
	h.Forget("env")
	if h.PushActive("env") {
		t.Fatal("forgotten environment still reports push")
	}
	if r := h.Push("env", Batch{Session: "s", Seq: 1}); !r.Applied {
		t.Fatal("after Forget, a reconnect reusing the session must not be treated as a replay")
	}
}

func TestFailingRefreshStopsServingOldNumbers(t *testing.T) {
	clk := newClock()
	var fail atomic.Bool
	h := New(Options{Now: clk.Now, MaxStale: time.Minute, Pull: func(context.Context, string) ([]Sample, error) {
		if fail.Load() {
			return nil, errors.New("daemon unreachable")
		}
		return samples(1, "pull"), nil
	}})
	defer h.Close()
	if _, err := h.Snapshot(context.Background(), "env"); err != nil {
		t.Fatal(err)
	}
	fail.Store(true)
	clk.Advance(10 * time.Second)
	if _, err := h.Snapshot(context.Background(), "env"); err != nil {
		t.Fatalf("a snapshot inside MaxStale must still be served: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	clk.Advance(2 * time.Minute)
	if _, err := h.Snapshot(context.Background(), "env"); err == nil {
		t.Fatal("a snapshot past MaxStale with a failing refresh was served as current")
	}
}

func TestColdReadDoesNotWaitOutAFullSweep(t *testing.T) {
	release := make(chan struct{})
	h := New(Options{FirstReadWait: 30 * time.Millisecond, Pull: func(context.Context, string) ([]Sample, error) {
		<-release
		return samples(5, "pull"), nil
	}})
	defer h.Close()
	start := time.Now()
	snap, err := h.Snapshot(context.Background(), "env")
	if err != nil {
		t.Fatal(err)
	}
	if waited := time.Since(start); waited > time.Second || len(snap.Samples) != 0 {
		t.Fatalf("cold read waited %s and returned %d samples, want an empty answer after FirstReadWait", waited, len(snap.Samples))
	}
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for {
		snap, err = h.Snapshot(context.Background(), "env")
		if err == nil && len(snap.Samples) == 5 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the sweep that outlived the first read never reached later reads")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSubscribedAgentIsNotPulledWhileItsFirstPushIsDue(t *testing.T) {
	clk := newClock()
	var calls atomic.Int32
	h := New(Options{Now: clk.Now, PushGrace: time.Minute, Pull: func(context.Context, string) ([]Sample, error) {
		calls.Add(1)
		return samples(1, "pull"), nil
	}})
	defer h.Close()
	h.MarkSubscribed("env")
	if _, err := h.Snapshot(context.Background(), "env"); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatal("a sweep ran while the subscribed agent's first push was still due")
	}
	clk.Advance(2 * time.Minute)
	if _, err := h.Snapshot(context.Background(), "env"); err != nil {
		t.Fatal(err)
	}
	h.Close()
	if calls.Load() != 1 {
		t.Fatalf("after the grace window %d sweeps ran, want 1 fallback pull", calls.Load())
	}
}
