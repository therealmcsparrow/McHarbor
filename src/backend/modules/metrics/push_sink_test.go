// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/statshub"
)

func openPushSinkDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := coredb.OpenSQLite(filepath.Join(t.TempDir(), "push.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing db: %v", err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO environments (id, name) VALUES ('env', 'env')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO settings (key, value, category) VALUES ('agent_metrics_enabled', 'true', 'agent')`); err != nil {
		t.Fatal(err)
	}
	return database
}

func pushBatch(seq uint64, at time.Time) *coreagent.StatsPushPayload {
	ts := at.UTC().Format(time.RFC3339)
	return &coreagent.StatsPushPayload{
		Session: "s", Seq: seq, SampledAt: ts,
		Samples: []coreagent.OfflineMetricEntry{
			{ContainerID: "a", ContainerName: "a", CPUPercent: 10, MemUsed: 5, MemLimit: 10, SampledAt: ts},
			{ContainerID: "b", ContainerName: "b", CPUPercent: 20, MemUsed: 5, MemLimit: 10, SampledAt: ts},
		},
	}
}

func count(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table + ` WHERE environment_id = 'env'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPushSinkWritesHistoryAtTheRawCadence(t *testing.T) {
	db := openPushSinkDB(t)
	hub := statshub.New(statshub.Options{Pull: func(context.Context, string) ([]statshub.Sample, error) { return nil, nil }})
	defer hub.Close()
	sink := NewPushSink(hub, db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	start := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

	for i := range 7 {
		ack := sink.IngestStatsPush(context.Background(), "env", pushBatch(uint64(i+1), start.Add(time.Duration(i)*10*time.Second)))
		if !ack.Accepted || ack.IntervalSeconds <= 0 {
			t.Fatalf("batch %d ack = %+v", i+1, ack)
		}
	}
	if got := count(t, db, "host_metrics"); got != 3 {
		t.Fatalf("seven 10s batches wrote %d host rows, want 3 (0s, 30s, 60s)", got)
	}
	if got := count(t, db, "container_metrics"); got != 6 {
		t.Fatalf("container rows = %d, want 2 per written batch", got)
	}
	if !hub.PushActive("env") {
		t.Fatal("hub does not report the environment as pushing")
	}
}

func TestPushSinkAndReplayDoNotDuplicate(t *testing.T) {
	db := openPushSinkDB(t)
	hub := statshub.New(statshub.Options{Pull: func(context.Context, string) ([]statshub.Sample, error) { return nil, nil }})
	defer hub.Close()
	sink := NewPushSink(hub, db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	at := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

	batch := pushBatch(1, at)
	sink.IngestStatsPush(context.Background(), "env", batch)
	ack := sink.IngestStatsPush(context.Background(), "env", batch)
	if !ack.Accepted || !ack.Duplicate {
		t.Fatalf("resent batch ack = %+v, want accepted duplicate", ack)
	}
	if err := IngestSamples(context.Background(), db, "env", at, offlineToMetrics(batch.Samples)); err != nil {
		t.Fatal(err)
	}
	if got := count(t, db, "host_metrics"); got != 1 {
		t.Fatalf("one batch pushed, resent and replayed offline left %d host rows, want 1", got)
	}
	if got := count(t, db, "container_metrics"); got != 2 {
		t.Fatalf("container rows = %d, want 2", got)
	}
}

func TestPushSinkStopsAgentsWhenAgentMetricsAreOff(t *testing.T) {
	db := openPushSinkDB(t)
	if _, err := db.Exec(`UPDATE settings SET value = 'false' WHERE key = 'agent_metrics_enabled'`); err != nil {
		t.Fatal(err)
	}
	hub := statshub.New(statshub.Options{Pull: func(context.Context, string) ([]statshub.Sample, error) { return nil, nil }})
	defer hub.Close()
	sink := NewPushSink(hub, db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ack := sink.IngestStatsPush(context.Background(), "env", pushBatch(1, time.Now()))
	if !ack.Stop {
		t.Fatalf("ack = %+v, want stop while the agent metrics switch is off", ack)
	}
	if hub.PushActive("env") || count(t, db, "host_metrics") != 0 {
		t.Fatal("a batch was applied while agent metrics are off")
	}
}
