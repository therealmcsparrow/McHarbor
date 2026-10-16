// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

func chainRows(ids ...string) []PrunedRow {
	rows := make([]PrunedRow, 0, len(ids))
	prev := ""
	for i, id := range ids {
		hash := "h-" + id
		rows = append(rows, PrunedRow{ID: id, Severity: "info", Timestamp: time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC).Format(time.RFC3339), PrevHash: prev, RowHash: hash})
		prev = hash
	}
	return rows
}

func TestPlanTombstonesGroupsContiguousRuns(t *testing.T) {
	chain := chainRows("a", "b", "c", "d", "e")
	unsealed := PrunedRow{ID: "u", Severity: "info"}
	victims := []PrunedRow{chain[3], chain[0], unsealed, chain[1]}

	tombstones, err := PlanTombstones("secret", victims, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(tombstones) != 2 {
		t.Fatalf("want 2 runs (a-b, d), got %d: %+v", len(tombstones), tombstones)
	}
	byEntry := map[string]Tombstone{}
	for _, ts := range tombstones {
		byEntry[ts.EntryHash] = ts
	}
	head := byEntry[""]
	if head.ExitHash != "h-b" || head.RowCount != 2 || head.FirstRowID != "a" || head.LastRowID != "b" {
		t.Fatalf("head run: %+v", head)
	}
	mid := byEntry["h-c"]
	if mid.ExitHash != "h-d" || mid.RowCount != 1 {
		t.Fatalf("middle run: %+v", mid)
	}

	bridge := NewChainBridge("secret", tombstones)
	if rows, ok := bridge.Cross("", "h-b"); !ok || rows != 2 {
		t.Fatalf("cross head gap: rows=%d ok=%t", rows, ok)
	}
	if _, ok := bridge.Cross("h-b", "h-d"); ok {
		t.Fatal("a gap no tombstone covers must not be crossable")
	}
}

func TestChainBridgeRejectsForgedTombstone(t *testing.T) {
	tombstones, err := PlanTombstones("secret", chainRows("a", "b"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	forged := tombstones[0]
	forged.RowCount = 99
	bridge := NewChainBridge("secret", []Tombstone{forged})
	if bridge.Invalid != 1 {
		t.Fatalf("forged tombstone should be counted invalid: %+v", bridge)
	}
	if _, ok := bridge.Cross("", "h-b"); ok {
		t.Fatal("a forged tombstone must not bridge a gap")
	}
	wrongKey := NewChainBridge("other-secret", tombstones)
	if _, ok := wrongKey.Cross("", "h-b"); ok {
		t.Fatal("a tombstone sealed under another secret must not bridge a gap")
	}
}

func TestChainBridgeFollowsAdjacentTombstones(t *testing.T) {
	chain := chainRows("a", "b", "c")
	first, err := PlanTombstones("secret", chain[:1], time.Now())
	if err != nil {
		t.Fatal(err)
	}
	second, err := PlanTombstones("secret", chain[1:2], time.Now())
	if err != nil {
		t.Fatal(err)
	}
	bridge := NewChainBridge("secret", append(first, second...))
	if rows, ok := bridge.Cross("", "h-b"); !ok || rows != 2 {
		t.Fatalf("two batches pruned back to back: rows=%d ok=%t", rows, ok)
	}
}

func TestPruneClauseAppliesOverridesAndDefault(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	if _, _, ok := pruneClause(RetentionPolicy{DefaultDays: 0, BySeverity: map[string]int{}}, now); ok {
		t.Fatal("keep-forever everywhere must prune nothing")
	}
	clause, args, ok := pruneClause(RetentionPolicy{DefaultDays: 0, BySeverity: map[string]int{"error": 7}}, now)
	if !ok || clause != "(severity = ? AND timestamp < ?)" || args[0] != "error" || args[1] != "2026-09-19T12:00:00Z" {
		t.Fatalf("an override must prune even when the default keeps forever: %q %v", clause, args)
	}
}

func openRetentionDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "retention.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeTest(t, database) })
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return database
}

func TestRetentionOverridesRoundTrip(t *testing.T) {
	database := openRetentionDB(t)
	ctx := context.Background()
	seven, zero := 7, 0
	if err := WriteRetentionOverrides(ctx, database, map[string]*int{"error": &seven, "info": &zero}); err != nil {
		t.Fatal(err)
	}
	policy, err := ReadRetentionPolicy(ctx, database, 90)
	if err != nil {
		t.Fatal(err)
	}
	if policy.DaysFor("error") != 7 || policy.DaysFor("info") != 0 || policy.DaysFor("warning") != 90 || policy.DaysFor("bogus") != 90 {
		t.Fatalf("effective policy: %+v", policy)
	}
	if err := WriteRetentionOverrides(ctx, database, map[string]*int{"error": nil}); err != nil {
		t.Fatal(err)
	}
	policy, err = ReadRetentionPolicy(ctx, database, 90)
	if err != nil {
		t.Fatal(err)
	}
	if policy.DaysFor("error") != 90 {
		t.Fatalf("a cleared override must inherit the default: %+v", policy)
	}
	tooMany := MaxRetentionDays + 1
	if err := WriteRetentionOverrides(ctx, database, map[string]*int{"error": &tooMany}); err == nil {
		t.Fatal("an out-of-range override must be refused")
	}
}

// walkChain mirrors the module verifier's linkage check so this package can
// prove a pruned chain still links, with and without tombstones.
func walkChain(t *testing.T, database *sql.DB, bridge *ChainBridge) (bool, int64) {
	t.Helper()
	rows, err := database.Query("SELECT COALESCE(prev_hash, ''), COALESCE(row_hash, '') FROM audit_logs ORDER BY timestamp ASC, id ASC")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("close rows: %v", err)
		}
	}()
	previous := ""
	var crossed int64
	for rows.Next() {
		var prev, hash string
		if err := rows.Scan(&prev, &hash); err != nil {
			t.Fatal(err)
		}
		if hash == "" {
			continue
		}
		if prev != previous {
			n, ok := bridge.Cross(previous, prev)
			if !ok {
				return false, crossed
			}
			crossed += n
		}
		previous = hash
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return true, crossed
}

func TestPruneBySeverityKeepsChainLinked(t *testing.T) {
	database := openRetentionDB(t)
	ctx := context.Background()
	logger := NewLogger(database, "secret")
	for _, severity := range []string{"info", "info", "warning", "info", "error", "info", "warning", "info"} {
		if err := logger.Record("", "admin", Entry{Action: "test.event", Severity: severity, EntityType: "test"}); err != nil {
			t.Fatal(err)
		}
	}

	policy := RetentionPolicy{DefaultDays: 30, BySeverity: map[string]int{"info": 1, "error": 0}}
	result, err := PruneWithPolicy(ctx, database, "secret", policy, time.Now().Add(48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 5 || result.BySeverity["info"] != 5 || result.Tombstones != 4 {
		t.Fatalf("prune result: %+v", result)
	}
	var left int
	if err := database.QueryRow("SELECT COUNT(*) FROM audit_logs WHERE severity = 'info'").Scan(&left); err != nil || left != 0 {
		t.Fatalf("info rows left: %d %v", left, err)
	}

	if ok, _ := walkChain(t, database, NewChainBridge("secret", nil)); ok {
		t.Fatal("without tombstones the pruned chain must not link; the test proves nothing otherwise")
	}
	bridge, err := LoadChainBridge(ctx, database, "secret")
	if err != nil {
		t.Fatal(err)
	}
	ok, crossed := walkChain(t, database, bridge)
	if !ok || crossed != 4 {
		t.Fatalf("pruned chain must link through tombstones: ok=%t crossed=%d", ok, crossed)
	}

	if err := logger.Record("", "admin", Entry{Action: "test.after", Severity: "info", EntityType: "test"}); err != nil {
		t.Fatal(err)
	}
	if ok, _ := walkChain(t, database, bridge); !ok {
		t.Fatal("a row written after the prune must still link")
	}
}
