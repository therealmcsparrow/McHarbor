// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package capacity

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// openCapacityTestDB creates the one table this package touches. The migration's
// own shape is asserted here rather than mirrored loosely: the unique index is the
// mechanism the bucket-collision test is checking.
func openCapacityTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	t.Cleanup(func() { ignoreErr(db.Close()) })
	_, err = db.Exec(`
		CREATE TABLE capacity_samples (
			id              TEXT PRIMARY KEY,
			environment_id  TEXT NOT NULL,
			subject_kind    TEXT NOT NULL,
			subject_id      TEXT NOT NULL,
			subject_name    TEXT NOT NULL DEFAULT '',
			sampled_at      TEXT NOT NULL,
			bucket_seconds  INTEGER NOT NULL DEFAULT 3600,
			used_bytes      INTEGER NOT NULL DEFAULT 0,
			total_bytes     INTEGER NOT NULL DEFAULT 0,
			created_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
			updated_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
		);
		CREATE UNIQUE INDEX idx_capacity_samples_unique
			ON capacity_samples(environment_id, subject_kind, subject_id, sampled_at);`)
	if err != nil {
		t.Fatalf("creating capacity_samples: %v", err)
	}
	return db
}

func countSamples(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM capacity_samples").Scan(&n); err != nil {
		t.Fatalf("counting samples: %v", err)
	}
	return n
}

func rootFS(used, total int64) Reading {
	return Reading{EnvironmentID: "env1", SubjectKind: KindRootFS, SubjectID: "env1", SubjectName: "/", UsedBytes: used, TotalBytes: total}
}

// Two writers in the same hour must correct one row rather than add a second
// point, or the fit sees a vertical pair of samples at the same instant.
func TestWriteCollapsesTwoWritersInOneBucket(t *testing.T) {
	db := openCapacityTestDB(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 27, 3, 14, 0, 0, time.UTC)

	if err := Write(ctx, db, rootFS(100, 1000), at); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := Write(ctx, db, rootFS(200, 1000), at.Add(41*time.Minute)); err != nil {
		t.Fatalf("second write in the same hour: %v", err)
	}
	if n := countSamples(t, db); n != 1 {
		t.Fatalf("two writes in one bucket produced %d rows, want 1", n)
	}
	var used int64
	if err := db.QueryRow("SELECT used_bytes FROM capacity_samples").Scan(&used); err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if used != 200 {
		t.Fatalf("used_bytes = %d, want the later reading 200", used)
	}
}

func TestWriteKeepsSeparateBucketsSeparate(t *testing.T) {
	db := openCapacityTestDB(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 27, 3, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		if err := Write(ctx, db, rootFS(int64(100+i), 1000), at.Add(time.Duration(i)*time.Hour)); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	if n := countSamples(t, db); n != 3 {
		t.Fatalf("three hourly writes produced %d rows, want 3", n)
	}
}

// A destination whose provider cannot report a ceiling is absent from the series.
// "0 of 0" would read as a full disk on every surface that divides by total.
func TestWriteSkipsAnUnmeasurableSubject(t *testing.T) {
	db := openCapacityTestDB(t)
	ctx := context.Background()
	at := time.Now()

	unsupported := Reading{EnvironmentID: "env1", SubjectKind: KindStorageLocation, SubjectID: "loc1", UsedBytes: 0, TotalBytes: 0}
	if err := Write(ctx, db, unsupported, at); err != nil {
		t.Fatalf("writing an unmeasurable subject must not error: %v", err)
	}
	if n := countSamples(t, db); n != 0 {
		t.Fatalf("an unmeasurable subject wrote %d rows, want 0", n)
	}

	noEnv := rootFS(1, 2)
	noEnv.EnvironmentID = ""
	if err := Write(ctx, db, noEnv, at); err != nil {
		t.Fatalf("writing without an environment must not error: %v", err)
	}
	if n := countSamples(t, db); n != 0 {
		t.Fatalf("a sample with no environment wrote %d rows, want 0", n)
	}
}

func TestPruneDropsOldAndKeepsNewest(t *testing.T) {
	db := openCapacityTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// One sample well past a seven-day horizon, one inside it, one now.
	for _, age := range []time.Duration{40 * 24 * time.Hour, 3 * 24 * time.Hour, 0} {
		if err := Write(ctx, db, rootFS(10, 100), now.Add(-age)); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}
	if n := countSamples(t, db); n != 3 {
		t.Fatalf("seeded %d rows, want 3", n)
	}

	deleted, err := Prune(ctx, db, 7)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("prune deleted %d rows, want 1", deleted)
	}
	if n := countSamples(t, db); n != 2 {
		t.Fatalf("%d rows survived, want 2", n)
	}

	// The newest sample is never pruned, whatever horizon is asked for: a
	// retention of one day must still leave the series something to read.
	if _, err := Prune(ctx, db, 0); err != nil {
		t.Fatalf("prune with a zero horizon: %v", err)
	}
	if n := countSamples(t, db); n != 1 {
		t.Fatalf("after a one-day prune %d rows remain, want the newest 1", n)
	}
}

func TestSeriesGroupsBySubjectAndOrdersByTime(t *testing.T) {
	db := openCapacityTestDB(t)
	ctx := context.Background()
	base := time.Now().UTC().Add(-10 * 24 * time.Hour)

	for i := 0; i < 5; i++ {
		at := base.Add(time.Duration(i) * 24 * time.Hour)
		if err := Write(ctx, db, rootFS(int64(100+i*10), 1000), at); err != nil {
			t.Fatalf("seeding rootfs: %v", err)
		}
		loc := Reading{EnvironmentID: "env1", SubjectKind: KindStorageLocation, SubjectID: "loc1",
			SubjectName: "Backups", UsedBytes: int64(500 + i), TotalBytes: 2000}
		if err := Write(ctx, db, loc, at); err != nil {
			t.Fatalf("seeding destination: %v", err)
		}
	}

	subjects, err := Series(ctx, db, "env1", base.Add(-time.Hour))
	if err != nil {
		t.Fatalf("series: %v", err)
	}
	if len(subjects) != 2 {
		t.Fatalf("got %d subjects, want 2", len(subjects))
	}
	for _, s := range subjects {
		if len(s.Points) != 5 {
			t.Fatalf("subject %s/%s has %d points, want 5", s.Kind, s.ID, len(s.Points))
		}
		for i := 1; i < len(s.Points); i++ {
			if s.Points[i].At.Before(s.Points[i-1].At) {
				t.Fatalf("subject %s/%s points are not ordered by time", s.Kind, s.ID)
			}
		}
	}
}
