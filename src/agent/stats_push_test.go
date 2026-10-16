// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

type fakeDaemon struct {
	mu         sync.Mutex
	running    []string
	openStream atomic.Int32
}

func (d *fakeDaemon) setRunning(ids ...string) {
	d.mu.Lock()
	d.running = ids
	d.mu.Unlock()
}

func (d *fakeDaemon) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/containers/json":
		d.mu.Lock()
		items := make([]dockerContainerListItem, 0, len(d.running))
		for _, id := range d.running {
			items = append(items, dockerContainerListItem{ID: id, Names: []string{"/" + id}})
		}
		d.mu.Unlock()
		if err := json.NewEncoder(w).Encode(items); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	case strings.HasSuffix(r.URL.Path, "/stats"):
		d.openStream.Add(1)
		defer d.openStream.Add(-1)
		enc := json.NewEncoder(w)
		for tick := uint64(1); ; tick++ {
			var doc dockerStats
			doc.CPUStats.CPUUsage.TotalUsage = tick * 200
			doc.PreCPUStats.CPUUsage.TotalUsage = (tick - 1) * 200
			doc.CPUStats.SystemUsage = tick * 1000
			doc.PreCPUStats.SystemUsage = (tick - 1) * 1000
			doc.CPUStats.OnlineCPUs = 1
			doc.MemoryStats.Usage = 64
			doc.MemoryStats.Limit = 128
			if err := enc.Encode(doc); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
	default:
		http.NotFound(w, r)
	}
}

type harness struct {
	pusher *statsPusher
	daemon *fakeDaemon
	store  *OfflineStore
	sent   chan *StatsPushPayload
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	daemon := &fakeDaemon{}
	srv := httptest.NewServer(daemon)
	t.Cleanup(srv.Close)
	addr := strings.TrimPrefix(srv.URL, "http://")
	proxy := &Proxy{httpClient: &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", addr)
		},
	}}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store, err := OpenOfflineStore(filepath.Join(t.TempDir(), "offline.db"), logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { errlog.Close(store) })
	sent := make(chan *StatsPushPayload, 16)
	send := func(msg WSMessage) error {
		if msg.Type != MsgStatsPush {
			return fmt.Errorf("unexpected message %s", msg.Type)
		}
		sent <- msg.StatsPush
		return nil
	}
	p := newStatsPusher(proxy, NewOfflineBuffer(store, proxy, time.Minute, logger), send, 10*time.Second, logger)
	p.ackTimeout = 200 * time.Millisecond
	return &harness{pusher: p, daemon: daemon, store: store, sent: sent}
}

func (h *harness) waitForSamples(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for len(h.pusher.snapshot()) != n {
		if time.Now().After(deadline) {
			t.Fatalf("have %d streamed samples, want %d", len(h.pusher.snapshot()), n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (h *harness) push(t *testing.T, ctx context.Context, answer func(*StatsPushPayload) *StatsPushPayload) (*StatsPushPayload, bool) {
	t.Helper()
	type result struct{ stop bool }
	done := make(chan result, 1)
	go func() { done <- result{h.pusher.pushOnce(ctx)} }()
	var batch *StatsPushPayload
	select {
	case batch = <-h.sent:
	case <-time.After(2 * time.Second):
		t.Fatal("no batch was sent")
	}
	if ack := answer(batch); ack != nil {
		h.pusher.DeliverAck(ack)
	}
	return batch, (<-done).stop
}

func metricRows(t *testing.T, s *OfflineStore) int64 {
	t.Helper()
	_, _, metrics := s.Counts(context.Background())
	return metrics
}

func TestStatsPushBatchesAreOrderedAndAcked(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.daemon.setRunning("a", "b", "c")
	h.pusher.reconcile(ctx)
	h.waitForSamples(t, 3)

	first, _ := h.push(t, ctx, func(b *StatsPushPayload) *StatsPushPayload {
		return &StatsPushPayload{Session: b.Session, Seq: b.Seq, Accepted: true, IntervalSeconds: 20}
	})
	second, _ := h.push(t, ctx, func(b *StatsPushPayload) *StatsPushPayload {
		return &StatsPushPayload{Session: b.Session, Seq: b.Seq, Accepted: true, Duplicate: true, IntervalSeconds: 20}
	})
	if first.Seq != 1 || second.Seq != 2 || first.Session != second.Session {
		t.Fatalf("batches (%s,%d) then (%s,%d); want one session with seq 1 then 2", first.Session, first.Seq, second.Session, second.Seq)
	}
	if len(first.Samples) != 3 || first.Samples[0].SampledAt != first.SampledAt {
		t.Fatalf("batch carried %d samples stamped %q, want 3 stamped with the batch time", len(first.Samples), first.Samples[0].SampledAt)
	}
	if first.Samples[0].CPUPercent <= 0 {
		t.Fatal("streamed samples report 0% CPU; the precpu pair was not used")
	}
	if h.pusher.interval != 20*time.Second {
		t.Fatalf("interval %s after an ack asking for 20s", h.pusher.interval)
	}
	if n := metricRows(t, h.store); n != 0 {
		t.Fatalf("%d rows spooled although every batch was acked", n)
	}
}

func TestStatsPushSpoolsWhatIsNotAcked(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.daemon.setRunning("a", "b")
	h.pusher.reconcile(ctx)
	h.waitForSamples(t, 2)

	h.push(t, ctx, func(b *StatsPushPayload) *StatsPushPayload {
		return &StatsPushPayload{Session: b.Session, Seq: b.Seq + 7, Accepted: true}
	})
	if n := metricRows(t, h.store); n != 2 {
		t.Fatalf("a batch acked only for another seq left %d spooled rows, want 2", n)
	}
	h.push(t, ctx, func(b *StatsPushPayload) *StatsPushPayload {
		return &StatsPushPayload{Session: b.Session, Seq: b.Seq, Error: "history write failed"}
	})
	if n := metricRows(t, h.store); n != 4 {
		t.Fatalf("a rejected batch left %d spooled rows, want 4", n)
	}
}

func TestStatsPushStopsWhenTold(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.daemon.setRunning("a")
	h.pusher.reconcile(ctx)
	h.waitForSamples(t, 1)
	_, stop := h.push(t, ctx, func(b *StatsPushPayload) *StatsPushPayload {
		return &StatsPushPayload{Session: b.Session, Seq: b.Seq, Accepted: true, Stop: true}
	})
	if !stop {
		t.Fatal("pusher kept going after the server said stop")
	}
}

func TestStatsPushFollowsTheRunningSet(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	h.daemon.setRunning("a", "b", "c")
	h.pusher.reconcile(ctx)
	h.waitForSamples(t, 3)

	h.daemon.setRunning("a")
	h.pusher.reconcile(ctx)
	h.waitForSamples(t, 1)
	if got := h.pusher.snapshot()[0].ContainerID; got != "a" {
		t.Fatalf("remaining sample is %q, want the still-running container", got)
	}

	cancel()
	h.pusher.wg.Wait()
	deadline := time.Now().Add(2 * time.Second)
	for h.daemon.openStream.Load() != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("%d stats streams still open after the connection ended", h.daemon.openStream.Load())
		}
		time.Sleep(10 * time.Millisecond)
	}
}
