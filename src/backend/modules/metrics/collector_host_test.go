// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/statshub"
)

type hostRow struct {
	cpu, memPct       int
	memUsed, memTotal int64
	netRx, blockRead  sql.NullInt64
}

func newHostTestCollector(t *testing.T, db *sql.DB, samples []statshub.Sample, host *HostReading) *Collector {
	t.Helper()
	hub := statshub.New(statshub.Options{Pull: func(context.Context, string) ([]statshub.Sample, error) { return samples, nil }})
	t.Cleanup(hub.Close)
	c := NewCollector(db, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	c.SetStatsHub(hub)
	c.hostReader = func(context.Context, string) (HostReading, bool) {
		if host == nil {
			return HostReading{}, false
		}
		return *host, true
	}
	return c
}

func tick(t *testing.T, c *Collector, isAgent bool, at time.Time) {
	t.Helper()
	c.collectEnvironment(context.Background(), NewService(nil), "env", isAgent, at, false)
}

func readHostRows(t *testing.T, db *sql.DB) []hostRow {
	t.Helper()
	rows, err := db.Query(`SELECT cpu_percent, memory_percent, memory_used, memory_total, net_rx, block_read
		FROM host_metrics WHERE environment_id = 'env' ORDER BY timestamp`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("closing rows: %v", err)
		}
	}()
	var out []hostRow
	for rows.Next() {
		var r hostRow
		if err := rows.Scan(&r.cpu, &r.memPct, &r.memUsed, &r.memTotal, &r.netRx, &r.blockRead); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

var twoContainers = []statshub.Sample{
	{ID: "a", Name: "a", Key: "a", CPUPercent: 10, MemUsage: 100, MemLimit: 1000, NetRx: 7, BlockRead: 3},
	{ID: "b", Name: "b", Key: "b", CPUPercent: 20, MemUsage: 100, MemLimit: 1000, NetRx: 5, BlockRead: 2},
}

func TestCollectorRecordsHostRowForAHostWithNoContainers(t *testing.T) {
	db := openPushSinkDB(t)
	c := newHostTestCollector(t, db, nil, &HostReading{CPUPercent: 42.4, MemUsed: 4_000, MemTotal: 8_000})
	tick(t, c, false, time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC))

	rows := readHostRows(t, db)
	if len(rows) != 1 {
		t.Fatalf("zero containers wrote %d host rows, want 1", len(rows))
	}
	r := rows[0]
	if r.cpu != 42 || r.memUsed != 4_000 || r.memTotal != 8_000 || r.memPct != 50 {
		t.Fatalf("host row = %+v, want the host reading (cpu 42, 4000/8000, 50%%)", r)
	}
	if r.netRx.Valid || r.blockRead.Valid {
		t.Fatalf("I/O columns = %+v, want NULL with no containers", r)
	}
}

func TestCollectorTakesCPUAndMemoryFromTheHostButIOFromContainers(t *testing.T) {
	db := openPushSinkDB(t)
	c := newHostTestCollector(t, db, twoContainers, &HostReading{CPUPercent: 5, MemUsed: 4_000, MemTotal: 8_000})
	tick(t, c, false, time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC))

	rows := readHostRows(t, db)
	if len(rows) != 1 {
		t.Fatalf("host rows = %d, want 1", len(rows))
	}
	r := rows[0]
	if r.cpu != 5 || r.memUsed != 4_000 || r.memTotal != 8_000 {
		t.Fatalf("host row = %+v, want host CPU/memory, not the container sums (30, 200, 1000)", r)
	}
	if r.netRx.Int64 != 12 || r.blockRead.Int64 != 5 {
		t.Fatalf("I/O = %d/%d, want the container sums 12/5", r.netRx.Int64, r.blockRead.Int64)
	}
}

func TestCollectorFallsBackToTheContainerAggregateWithoutAHostReading(t *testing.T) {
	db := openPushSinkDB(t)
	c := newHostTestCollector(t, db, twoContainers, nil)
	tick(t, c, false, time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC))

	rows := readHostRows(t, db)
	if len(rows) != 1 || rows[0].cpu != 30 || rows[0].memUsed != 200 || rows[0].memTotal != 1000 {
		t.Fatalf("rows = %+v, want one aggregate row (cpu 30, 200/1000)", rows)
	}

	empty := openPushSinkDB(t)
	tick(t, newHostTestCollector(t, empty, nil, nil), false, time.Now())
	if got := readHostRows(t, empty); len(got) != 0 {
		t.Fatalf("no containers and no host reading wrote %d rows, want 0", len(got))
	}
}

func TestPushActiveEnvironmentGetsExactlyOneHostRowPerTick(t *testing.T) {
	db := openPushSinkDB(t)
	host := &HostReading{CPUPercent: 9, MemUsed: 1_000, MemTotal: 4_000}
	hub := statshub.New(statshub.Options{Pull: func(context.Context, string) ([]statshub.Sample, error) { return nil, nil }})
	defer hub.Close()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sink := NewPushSink(hub, db, logger)
	sink.SetHostReader(func(context.Context, string) (HostReading, bool) { return *host, true })
	c := NewCollector(db, nil, logger)
	c.SetStatsHub(hub)
	c.hostReader = func(context.Context, string) (HostReading, bool) { return *host, true }

	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	sink.IngestStatsPush(context.Background(), "env", pushBatch(1, at))
	tick(t, c, true, at)

	rows := readHostRows(t, db)
	if len(rows) != 1 {
		t.Fatalf("push-active environment has %d host rows after a tick, want exactly 1", len(rows))
	}
	if rows[0].cpu != 9 || rows[0].memTotal != 4_000 {
		t.Fatalf("pushed host row = %+v, want the host reading", rows[0])
	}

}

func TestPushedBatchWithNoContainersStillRecordsTheHost(t *testing.T) {
	db := openPushSinkDB(t)
	hub := statshub.New(statshub.Options{Pull: func(context.Context, string) ([]statshub.Sample, error) { return nil, nil }})
	defer hub.Close()
	sink := NewPushSink(hub, db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	sink.SetHostReader(func(context.Context, string) (HostReading, bool) {
		return HostReading{CPUPercent: 3, MemUsed: 10, MemTotal: 100}, true
	})
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	batch := pushBatch(1, at)
	batch.Samples = nil
	if ack := sink.IngestStatsPush(context.Background(), "env", batch); !ack.Accepted {
		t.Fatalf("ack = %+v", ack)
	}
	rows := readHostRows(t, db)
	if len(rows) != 1 || rows[0].cpu != 3 || rows[0].memPct != 10 {
		t.Fatalf("rows = %+v, want one host row with 3%% CPU and 10%% memory", rows)
	}
}
