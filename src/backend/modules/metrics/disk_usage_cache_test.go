// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moby/moby/client"
)

func TestDiskUsageIsFetchedOncePerEnvironmentAndMinute(t *testing.T) {
	var clockMu sync.Mutex
	clock := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	advance := func(d time.Duration) { clockMu.Lock(); clock = clock.Add(d); clockMu.Unlock() }
	c := &diskUsageCache{entries: map[string]*diskUsageEntry{}, now: func() time.Time { clockMu.Lock(); defer clockMu.Unlock(); return clock }}
	var calls atomic.Int32
	release := make(chan struct{})
	fetch := func(context.Context) (client.DiskUsageResult, error) {
		calls.Add(1)
		<-release
		return client.DiskUsageResult{Images: client.ImagesDiskUsage{TotalSize: 42}}, nil
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := c.get(context.Background(), "env1", fetch)
			if err != nil || v.Images.TotalSize != 42 {
				t.Errorf("got %+v %v", v, err)
			}
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("ten concurrent polls ran %d disk usage calls", calls.Load())
	}

	advance(59 * time.Second)
	if _, err := c.get(context.Background(), "env1", fetch); err != nil || calls.Load() != 1 {
		t.Fatalf("within the minute: %d calls", calls.Load())
	}
	if _, err := c.get(context.Background(), "env2", fetch); err != nil || calls.Load() != 2 {
		t.Fatalf("another environment shares nothing: %d calls", calls.Load())
	}
	advance(2 * time.Second)
	if v, err := c.get(context.Background(), "env1", fetch); err != nil || v.Images.TotalSize != 42 {
		t.Fatalf("after the minute the last value must still be served: %+v %v", v, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for calls.Load() != 3 {
		if time.Now().After(deadline) {
			t.Fatalf("after the minute: %d calls, want one background refresh", calls.Load())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestAnExpiredValueIsServedWithoutWaitingForTheRefresh(t *testing.T) {
	clock := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	now := func() time.Time { mu.Lock(); defer mu.Unlock(); return clock }
	c := &diskUsageCache{entries: map[string]*diskUsageEntry{}, now: now}
	slow := make(chan struct{})
	var calls atomic.Int32
	fetch := func(context.Context) (client.DiskUsageResult, error) {
		if calls.Add(1) > 1 {
			<-slow
			return client.DiskUsageResult{Images: client.ImagesDiskUsage{TotalSize: 2}}, nil
		}
		return client.DiskUsageResult{Images: client.ImagesDiskUsage{TotalSize: 1}}, nil
	}
	if _, err := c.get(context.Background(), "env1", fetch); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	clock = clock.Add(2 * diskUsageTTL)
	mu.Unlock()

	for range 5 {
		start := time.Now()
		v, err := c.get(context.Background(), "env1", fetch)
		if err != nil || v.Images.TotalSize != 1 || time.Since(start) > 500*time.Millisecond {
			t.Fatalf("expired read returned %+v %v after %s; want the old value at once", v, err, time.Since(start))
		}
	}
	waitUntil := time.Now().Add(2 * time.Second)
	for calls.Load() < 2 && time.Now().Before(waitUntil) {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
	if calls.Load() != 2 {
		t.Fatalf("five expired reads started %d fetches, want 1 initial + 1 refresh", calls.Load())
	}
	close(slow)
	deadline := time.Now().Add(2 * time.Second)
	for {
		if v, err := c.get(context.Background(), "env1", fetch); err == nil && v.Images.TotalSize == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the background refresh never replaced the value")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestACancelledFirstReaderDoesNotFailTheOthers(t *testing.T) {
	c := &diskUsageCache{entries: map[string]*diskUsageEntry{}, now: time.Now}
	release := make(chan struct{})
	fetch := func(ctx context.Context) (client.DiskUsageResult, error) {
		select {
		case <-release:
			return client.DiskUsageResult{Images: client.ImagesDiskUsage{TotalSize: 9}}, nil
		case <-ctx.Done():
			return client.DiskUsageResult{}, ctx.Err()
		}
	}
	first, cancel := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		_, err := c.get(first, "env1", fetch)
		firstDone <- err
	}()
	time.Sleep(20 * time.Millisecond)
	type result struct {
		v   client.DiskUsageResult
		err error
	}
	secondDone := make(chan result, 1)
	go func() {
		v, err := c.get(context.Background(), "env1", fetch)
		secondDone <- result{v, err}
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	if err := <-firstDone; err == nil {
		t.Fatal("the cancelled reader got a value")
	}
	close(release)
	if r := <-secondDone; r.err != nil || r.v.Images.TotalSize != 9 {
		t.Fatalf("a reader waiting on the same load inherited the first reader's cancellation: %+v %v", r.v, r.err)
	}
}

func TestAFailedDiskUsageIsNotCached(t *testing.T) {
	c := &diskUsageCache{entries: map[string]*diskUsageEntry{}, now: time.Now}
	fail := true
	fetch := func(context.Context) (client.DiskUsageResult, error) {
		if fail {
			return client.DiskUsageResult{}, errors.New("daemon busy")
		}
		return client.DiskUsageResult{Images: client.ImagesDiskUsage{TotalSize: 7}}, nil
	}
	if _, err := c.get(context.Background(), "env1", fetch); err == nil {
		t.Fatal("the error was swallowed")
	}
	fail = false
	if v, err := c.get(context.Background(), "env1", fetch); err != nil || v.Images.TotalSize != 7 {
		t.Fatalf("a failure was cached: %+v %v", v, err)
	}
}
