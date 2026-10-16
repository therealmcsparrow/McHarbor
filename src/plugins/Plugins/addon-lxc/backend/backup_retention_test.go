// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"testing"
	"time"
)

// retentionNow is the fixed clock every case below is read against.
var retentionNow = time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

// fullRun builds a successful full backup that many days ago.
func fullRun(id, chain string, daysAgo int) BackupRun {
	return BackupRun{
		ID: id, ChainID: chain, ContainerID: "ct-1",
		Operation: BackupOpBackup, Status: BackupStatusSuccess,
		Kind: BackupKindFull, StartedAt: retentionNow.AddDate(0, 0, -daysAgo),
	}
}

// incrementalRun builds a successful incremental that many days ago.
func incrementalRun(id, chain string, daysAgo int) BackupRun {
	run := fullRun(id, chain, daysAgo)
	run.Kind = BackupKindIncremental
	return run
}

func TestBaselineRunsKeepsTheNewestFullAndEverythingAfterIt(t *testing.T) {
	svc := &Service{}
	// Newest first, the order successfulRunsForPlan returns.
	runs := []BackupRun{
		incrementalRun("i3", "chain-a", 1),
		incrementalRun("i2", "chain-a", 2),
		fullRun("f2", "chain-a", 3),
		incrementalRun("i1", "chain-a", 10),
		fullRun("f1", "chain-a", 11),
	}

	keep := svc.baselineRuns(runs)

	for _, id := range []string{"f2", "i2", "i3"} {
		if !keep[id] {
			t.Fatalf("%s rests on or is the newest full copy and must be kept", id)
		}
	}
	for _, id := range []string{"f1", "i1"} {
		if keep[id] {
			t.Fatalf("%s belongs to a superseded baseline and is an ordinary retention candidate", id)
		}
	}
}

func TestBaselineRunsKeepsNothingWhenNoFullCopySurvives(t *testing.T) {
	svc := &Service{}
	// A chain whose full copy has already gone. Nothing here is a baseline, and
	// claiming otherwise would pin unrestorable rows forever.
	keep := svc.baselineRuns([]BackupRun{
		incrementalRun("i2", "chain-a", 1),
		incrementalRun("i1", "chain-a", 2),
	})
	if len(keep) != 0 {
		t.Fatalf("expected no baseline, got %v", keep)
	}
}

func TestRetentionCandidatesKeepThreeDeletesTheFourth(t *testing.T) {
	// Four full copies on four separate chains, so the baseline rule protects
	// only each chain's own newest — which is each run itself. The count rule is
	// what has to be read here, so every run is its own chain's only member and
	// baselineRuns protects all four; the candidate set is therefore computed
	// with the chain cleared, which is what a plan whose runs predate chains
	// looks like.
	runs := []BackupRun{
		fullRun("r1", "", 1),
		fullRun("r2", "", 2),
		fullRun("r3", "", 3),
		fullRun("r4", "", 4),
	}
	plan := BackupPlan{ID: "plan-1", RetentionCount: 3}

	doomed := retentionCandidates(plan, runs, map[string]bool{"r1": true}, retentionNow)

	if len(doomed) != 1 || doomed[0].ID != "r4" {
		t.Fatalf("expected only the fourth-newest to be pruned, got %v", ids(doomed))
	}
}

func TestRetentionNeverDeletesTheOnlySurvivingBackup(t *testing.T) {
	runs := []BackupRun{fullRun("only", "", 900)}
	plan := BackupPlan{ID: "plan-1", RetentionCount: 1, RetentionDays: 7}

	// The newest run of a container that still exists is protected before the
	// rules are applied, which is what pruneBackupRuns passes in.
	doomed := retentionCandidates(plan, runs, map[string]bool{"only": true}, retentionNow)

	if len(doomed) != 0 {
		t.Fatalf("the only surviving backup of a live container must never be pruned, got %v", ids(doomed))
	}
}

func TestRetentionAgeRuleDeletesWhatIsOlderThanTheWindow(t *testing.T) {
	runs := []BackupRun{
		fullRun("fresh", "", 1),
		fullRun("stale", "", 40),
	}
	plan := BackupPlan{ID: "plan-1", RetentionDays: 30}

	doomed := retentionCandidates(plan, runs, map[string]bool{"fresh": true}, retentionNow)

	if len(doomed) != 1 || doomed[0].ID != "stale" {
		t.Fatalf("expected only the run outside the window to be pruned, got %v", ids(doomed))
	}
}

func ids(runs []BackupRun) []string {
	out := make([]string, 0, len(runs))
	for _, run := range runs {
		out = append(out, run.ID)
	}
	return out
}
