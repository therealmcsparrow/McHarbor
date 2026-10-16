// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func openIdleTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing db: %v", err)
		}
	})
	_, err = db.Exec(`
		CREATE TABLE container_state_samples (
			id              TEXT PRIMARY KEY,
			environment_id  TEXT,
			container_id    TEXT NOT NULL,
			container_name  TEXT,
			state           TEXT NOT NULL,
			running         INTEGER NOT NULL DEFAULT 0,
			health          TEXT,
			sampled_at      TEXT NOT NULL
		);
		CREATE TABLE container_metrics_rollup (
			id                TEXT PRIMARY KEY,
			environment_id    TEXT,
			container_id      TEXT NOT NULL,
			container_name    TEXT,
			bucket_start      TEXT NOT NULL,
			bucket_seconds    INTEGER NOT NULL DEFAULT 600,
			samples           INTEGER NOT NULL DEFAULT 0,
			cpu_avg           REAL NOT NULL DEFAULT 0,
			cpu_max           REAL NOT NULL DEFAULT 0,
			mem_used_avg      INTEGER NOT NULL DEFAULT 0,
			net_rx_delta      INTEGER NOT NULL DEFAULT 0,
			net_tx_delta      INTEGER NOT NULL DEFAULT 0,
			size_rw_avg       INTEGER,
			size_root_fs_avg  INTEGER
		);`)
	if err != nil {
		t.Fatalf("creating tables: %v", err)
	}
	return db
}

const idleLayout = "2006-01-02 15:04:05"

// state seeds one container's history: running until stoppedAgo, observed until
// lastSeenAgo.
func seedState(t *testing.T, db *sql.DB, id string, stoppedAgo, lastSeenAgo time.Duration, now time.Time) {
	t.Helper()
	insert := func(suffix string, at time.Time, running int) {
		_, err := db.Exec(
			`INSERT INTO container_state_samples (id, environment_id, container_id, container_name, state, running, sampled_at)
			 VALUES (?, 'env1', ?, ?, ?, ?, ?)`,
			id+suffix, id, id, map[int]string{1: "running", 0: "exited"}[running], running, at.Format(idleLayout))
		if err != nil {
			t.Fatalf("seeding state sample: %v", err)
		}
	}
	insert("-ran", now.Add(-stoppedAgo), 1)
	insert("-seen", now.Add(-lastSeenAgo), 0)
}

// The boundary is the assertion that matters: a container stopped for exactly the
// threshold has reached it and not passed it. Offering it for deletion on the
// boundary is how an off-by-one becomes a deleted container.
func TestIdleBoundaryExactlyNDaysIsNotYetIdle(t *testing.T) {
	db := openIdleTestDB(t)
	now := time.Now().UTC()
	svc := NewCapacityService(db, nil)

	seedState(t, db, "exactly", 14*24*time.Hour, time.Minute, now)
	seedState(t, db, "justover", 14*24*time.Hour+2*time.Hour, time.Minute, now)

	report, err := svc.Idle(context.Background(), "", 14)
	if err != nil {
		t.Fatalf("idle: %v", err)
	}
	names := map[string]bool{}
	for _, row := range report.Containers {
		names[row.ContainerID] = true
	}
	if names["exactly"] {
		t.Fatal("a container stopped for exactly the threshold is not yet idle")
	}
	if !names["justover"] {
		t.Fatal("a container stopped past the threshold must be listed")
	}
}

func TestIdleSkipsAContainerStillRunning(t *testing.T) {
	db := openIdleTestDB(t)
	now := time.Now().UTC()
	// Its newest sample is a running one, so it is not stopped at all.
	_, err := db.Exec(
		`INSERT INTO container_state_samples (id, environment_id, container_id, container_name, state, running, sampled_at)
		 VALUES ('a', 'env1', 'busy', 'busy', 'running', 1, ?)`, now.Add(-time.Minute).Format(idleLayout))
	if err != nil {
		t.Fatalf("seeding: %v", err)
	}
	report, err := NewCapacityService(db, nil).Idle(context.Background(), "", 14)
	if err != nil {
		t.Fatalf("idle: %v", err)
	}
	if len(report.Containers) != 0 {
		t.Fatalf("a running container was listed as idle: %+v", report.Containers)
	}
}

// A container that left the history is not a reclaim candidate, it was removed.
func TestIdleSkipsAContainerThatLeftTheHistory(t *testing.T) {
	db := openIdleTestDB(t)
	now := time.Now().UTC()
	seedState(t, db, "gone", 60*24*time.Hour, 40*24*time.Hour, now)
	report, err := NewCapacityService(db, nil).Idle(context.Background(), "", 14)
	if err != nil {
		t.Fatalf("idle: %v", err)
	}
	if len(report.Containers) != 0 {
		t.Fatalf("a removed container was offered as a candidate: %+v", report.Containers)
	}
}

// Sizes are not collected for agent environments and are NULL on pushed samples. A
// row with no size says so; it never reports 0 B, which is a number the feature
// would be trusted for and silently wrong.
func TestIdleReportsAnUnknownSizeRatherThanZero(t *testing.T) {
	db := openIdleTestDB(t)
	now := time.Now().UTC()
	seedState(t, db, "nosize", 30*24*time.Hour, time.Minute, now)

	report, err := NewCapacityService(db, nil).Idle(context.Background(), "", 14)
	if err != nil {
		t.Fatalf("idle: %v", err)
	}
	if len(report.Containers) != 1 {
		t.Fatalf("got %d candidates, want 1", len(report.Containers))
	}
	row := report.Containers[0]
	if row.SizeKnown {
		t.Fatal("a container with no size sample must not claim a known size")
	}
	if row.SizeReason == "" {
		t.Fatal("an unknown size must name its reason")
	}
	if !report.SizesUnavailable {
		t.Fatal("an environment with no size samples at all must say so once")
	}
}

func TestIdleReportsAKnownSize(t *testing.T) {
	db := openIdleTestDB(t)
	now := time.Now().UTC()
	seedState(t, db, "sized", 30*24*time.Hour, time.Minute, now)
	_, err := db.Exec(
		`INSERT INTO container_metrics_rollup (id, environment_id, container_id, container_name, bucket_start, size_rw_avg, size_root_fs_avg)
		 VALUES ('r1', 'env1', 'sized', 'sized', ?, 4096, 8192)`, now.Add(-31*24*time.Hour).Format(idleLayout))
	if err != nil {
		t.Fatalf("seeding size: %v", err)
	}
	report, err := NewCapacityService(db, nil).Idle(context.Background(), "", 14)
	if err != nil {
		t.Fatalf("idle: %v", err)
	}
	if len(report.Containers) != 1 {
		t.Fatalf("got %d candidates, want 1", len(report.Containers))
	}
	row := report.Containers[0]
	if !row.SizeKnown || row.SizeRwBytes != 4096 || row.SizeRootBytes != 8192 {
		t.Fatalf("size not reported: %+v", row)
	}
	if report.SizesUnavailable {
		t.Fatal("an environment that does collect sizes must not claim otherwise")
	}
}

// The weaker signal is kept separate and labelled differently, so a judgement call
// never arrives looking like a fact.
func TestIdleLabelsAQuietRunningContainerSeparately(t *testing.T) {
	db := openIdleTestDB(t)
	now := time.Now().UTC()
	for i := 0; i <= 20; i++ {
		at := now.Add(-time.Duration(20-i) * 24 * time.Hour)
		_, err := db.Exec(
			`INSERT INTO container_metrics_rollup (id, environment_id, container_id, container_name, bucket_start, cpu_avg, net_rx_delta, net_tx_delta, size_rw_avg)
			 VALUES (?, 'env1', 'quiet', 'quiet', ?, 0, 0, 0, 512)`,
			"q"+time.Duration(i).String(), at.Format(idleLayout))
		if err != nil {
			t.Fatalf("seeding rollup: %v", err)
		}
	}
	report, err := NewCapacityService(db, nil).Idle(context.Background(), "", 14)
	if err != nil {
		t.Fatalf("idle: %v", err)
	}
	if len(report.Containers) != 1 {
		t.Fatalf("got %d candidates, want 1", len(report.Containers))
	}
	if report.Containers[0].Reason != "quiet" {
		t.Fatalf("reason = %q, want \"quiet\"", report.Containers[0].Reason)
	}
}

func TestIdleSkipsAContainerWithTraffic(t *testing.T) {
	db := openIdleTestDB(t)
	now := time.Now().UTC()
	for i := 0; i <= 20; i++ {
		at := now.Add(-time.Duration(20-i) * 24 * time.Hour)
		_, err := db.Exec(
			`INSERT INTO container_metrics_rollup (id, environment_id, container_id, container_name, bucket_start, cpu_avg, net_rx_delta, net_tx_delta)
			 VALUES (?, 'env1', 'chatty', 'chatty', ?, 0, 1024, 0)`,
			"c"+time.Duration(i).String(), at.Format(idleLayout))
		if err != nil {
			t.Fatalf("seeding rollup: %v", err)
		}
	}
	report, err := NewCapacityService(db, nil).Idle(context.Background(), "", 14)
	if err != nil {
		t.Fatalf("idle: %v", err)
	}
	if len(report.Containers) != 0 {
		t.Fatalf("a container moving traffic was listed as idle: %+v", report.Containers)
	}
}
