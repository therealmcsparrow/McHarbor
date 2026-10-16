// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package compliancepacks

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// openPackTestDB applies the pack migration verbatim rather than a hand-written
// schema, so a column added to the migration and not to the service is a failure
// here rather than at runtime.
func openPackTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "packs.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close sqlite: %v", err)
		}
	})
	schema, err := os.ReadFile("../../core/db/migrations/106_compliance_packs.sql")
	if err != nil {
		t.Fatalf("read migration 106: %v", err)
	}
	if _, err := database.Exec(string(schema)); err != nil {
		t.Fatalf("apply migration 106: %v", err)
	}
	return database
}

func TestRunRoundTripsThroughTheDatabase(t *testing.T) {
	ctx := context.Background()
	svc := NewService(openPackTestDB(t), nil)

	runID, err := svc.StartRun(ctx, FrameworkCIS, "env-1", TriggerManual, "user-1")
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	// A run is `running` before any check executes, so a crash is visible.
	detail, ok, err := svc.RunDetail(ctx, runID)
	if err != nil || !ok {
		t.Fatalf("RunDetail on a fresh run: ok=%v err=%v", ok, err)
	}
	if detail.Run.Status != RunRunning {
		t.Fatalf("a fresh run is %q, want %q", detail.Run.Status, RunRunning)
	}

	f, _ := FrameworkByKey(FrameworkCIS)
	results := Evaluate(f, goodDaemon())
	if err := svc.SaveResults(ctx, runID, results); err != nil {
		t.Fatalf("SaveResults: %v", err)
	}

	detail, ok, err = svc.RunDetail(ctx, runID)
	if err != nil || !ok {
		t.Fatalf("RunDetail after saving: ok=%v err=%v", ok, err)
	}
	if detail.Run.Status != RunCompleted {
		t.Fatalf("a saved run is %q, want %q", detail.Run.Status, RunCompleted)
	}
	if len(detail.Results) != len(results) {
		t.Fatalf("read back %d results, wrote %d", len(detail.Results), len(results))
	}
	if detail.Run.StartedBy != "user-1" {
		t.Fatalf("StartedBy = %q, want user-1", detail.Run.StartedBy)
	}

	// The stored summary must agree with the rows behind it; a summary that
	// disagrees is the inconsistency an auditor notices first.
	want := Summarize(results)
	if detail.Run.Summary != want {
		t.Fatalf("stored summary %+v, recomputed %+v", detail.Run.Summary, want)
	}

	// Facts and reason codes survive the JSON round trip, and reasons stay codes
	// rather than prose so they can be translated at read time.
	var found bool
	for _, r := range detail.Results {
		if r.ID != "2.13" {
			continue
		}
		found = true
		if len(r.Facts) == 0 || r.Facts[0].Key != "liveRestore" {
			t.Errorf("2.13 read back with facts %+v", r.Facts)
		}
	}
	if !found {
		t.Error("control 2.13 did not read back")
	}
}

func TestResultsReadBackInNumericControlOrder(t *testing.T) {
	ctx := context.Background()
	svc := NewService(openPackTestDB(t), nil)
	runID, err := svc.StartRun(ctx, FrameworkCIS, "", TriggerScheduled, "")
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	f, _ := FrameworkByKey(FrameworkCIS)
	if err := svc.SaveResults(ctx, runID, Evaluate(f, goodDaemon())); err != nil {
		t.Fatalf("SaveResults: %v", err)
	}
	detail, _, err := svc.RunDetail(ctx, runID)
	if err != nil {
		t.Fatalf("RunDetail: %v", err)
	}
	var nine, ten int
	for i, r := range detail.Results {
		switch r.ID {
		case "5.9":
			nine = i
		case "5.10":
			ten = i
		}
	}
	if nine >= ten {
		t.Fatalf("5.9 read back at %d and 5.10 at %d; the SQL ORDER BY is lexical and the reorder must fix it", nine, ten)
	}
}

func TestFailRunKeepsTheMessage(t *testing.T) {
	ctx := context.Background()
	svc := NewService(openPackTestDB(t), nil)
	runID, err := svc.StartRun(ctx, FrameworkCIS, "", TriggerManual, "user-1")
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if err := svc.FailRun(ctx, runID, "the check results could not be recorded"); err != nil {
		t.Fatalf("FailRun: %v", err)
	}
	detail, _, err := svc.RunDetail(ctx, runID)
	if err != nil {
		t.Fatalf("RunDetail: %v", err)
	}
	if detail.Run.Status != RunFailed {
		t.Fatalf("status %q, want %q", detail.Run.Status, RunFailed)
	}
	if detail.Run.LastError == "" {
		t.Fatal("a failed run must keep its message so the row can render it beside the state")
	}
	if err := svc.FailRun(ctx, "no-such-run", "x"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("failing an unknown run returned %v, want sql.ErrNoRows", err)
	}
}

func TestListRunsIsNewestFirstAndPaginated(t *testing.T) {
	ctx := context.Background()
	svc := NewService(openPackTestDB(t), nil)
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	var ids []string
	for i := 0; i < 5; i++ {
		svc.now = func() time.Time { return base.Add(time.Duration(i) * time.Hour) }
		id, err := svc.StartRun(ctx, FrameworkCIS, "", TriggerScheduled, "")
		if err != nil {
			t.Fatalf("StartRun: %v", err)
		}
		ids = append(ids, id)
	}
	runs, total, err := svc.ListRuns(ctx, FrameworkCIS, 2, 0)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if total != 5 {
		t.Fatalf("total = %d, want 5", total)
	}
	if len(runs) != 2 {
		t.Fatalf("got %d runs for a page of 2", len(runs))
	}
	if runs[0].ID != ids[4] {
		t.Fatalf("newest run is %s, want %s", runs[0].ID, ids[4])
	}
	latest, ok, err := svc.LatestRun(ctx, FrameworkCIS)
	if err != nil || !ok {
		t.Fatalf("LatestRun: ok=%v err=%v", ok, err)
	}
	if latest.ID != ids[4] {
		t.Fatalf("LatestRun is %s, want %s", latest.ID, ids[4])
	}
}

// TestLatestRunOnAPackNeverRun asserts the empty case is an absence rather than a
// zero-valued run, so the UI says "never run" instead of rendering a control table
// that reads as nothing being wrong.
func TestLatestRunOnAPackNeverRun(t *testing.T) {
	svc := NewService(openPackTestDB(t), nil)
	_, ok, err := svc.LatestRun(context.Background(), FrameworkCIS)
	if err != nil {
		t.Fatalf("LatestRun: %v", err)
	}
	if ok {
		t.Fatal("a pack that has never run must report no latest run")
	}
}

func TestPruneRemovesOldRunsAndTheirResults(t *testing.T) {
	ctx := context.Background()
	database := openPackTestDB(t)
	svc := NewService(database, nil)
	f, _ := FrameworkByKey(FrameworkCIS)

	svc.now = func() time.Time { return time.Now().UTC().Add(-2 * runRetention) }
	oldID, err := svc.StartRun(ctx, FrameworkCIS, "", TriggerScheduled, "")
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if err := svc.SaveResults(ctx, oldID, Evaluate(f, goodDaemon())); err != nil {
		t.Fatalf("SaveResults: %v", err)
	}

	svc.now = time.Now
	keepID, err := svc.StartRun(ctx, FrameworkCIS, "", TriggerScheduled, "")
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if err := svc.SaveResults(ctx, keepID, Evaluate(f, goodDaemon())); err != nil {
		t.Fatalf("SaveResults: %v", err)
	}

	if err := svc.Prune(ctx); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if _, ok, err := svc.RunDetail(ctx, oldID); err != nil || ok {
		t.Error("a run past the retention window must be pruned")
	}
	if _, ok, err := svc.RunDetail(ctx, keepID); err != nil || !ok {
		t.Error("a run inside the retention window must survive")
	}
	var orphans int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM compliance_pack_results WHERE run_id = ?`, oldID).Scan(&orphans); err != nil {
		t.Fatalf("count orphans: %v", err)
	}
	if orphans != 0 {
		t.Errorf("%d result rows survived their pruned run; orphaned rows are invisible and accumulate forever", orphans)
	}
}
