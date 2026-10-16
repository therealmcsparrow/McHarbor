// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package instancebackup

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

// TestVerifyChainCrossesSealedTombstones guards rehearsals of pruned installs: a
// gap retention sealed verifies, while the same gap with a forged seal does not.
func TestVerifyChainCrossesSealedTombstones(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeTest(t, database) })
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	const secret = "s3cret"
	ctx := context.Background()
	prev := "hash-of-pruned-row"
	for i, id := range []string{"a1", "a2"} {
		r := audit.HashRow{ID: id, PrevHash: prev, Action: "update", Severity: "info",
			Timestamp: "2026-09-2" + string(rune('1'+i)) + "T00:00:00Z", CreatedAt: "2026-09-21T00:00:00Z", UpdatedAt: "2026-09-21T00:00:00Z"}
		h, err := audit.ComputeRowHash(secret, r)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.Exec(`INSERT INTO audit_logs (id, action, severity, timestamp, created_at, updated_at, prev_hash, row_hash)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, r.ID, r.Action, r.Severity, r.Timestamp, r.CreatedAt, r.UpdatedAt, r.PrevHash, h); err != nil {
			t.Fatal(err)
		}
		prev = h
	}
	if ok, _, _, _ := verifyChain(ctx, database, secret); ok {
		t.Fatal("an unexplained gap must not verify")
	}

	stone := audit.Tombstone{ID: "t1", EntryHash: "", ExitHash: "hash-of-pruned-row", RowCount: 3, CreatedAt: "2026-09-21T00:00:00Z"}
	insert := func(seal string) {
		if _, err := database.Exec(`DELETE FROM audit_chain_tombstones`); err != nil {
			t.Fatal(err)
		}
		if _, err := database.Exec(`INSERT INTO audit_chain_tombstones (id, entry_hash, exit_hash, row_count, seal, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			stone.ID, stone.EntryHash, stone.ExitHash, stone.RowCount, seal, stone.CreatedAt); err != nil {
			t.Fatal(err)
		}
	}
	insert("forged")
	if ok, _, _, _ := verifyChain(ctx, database, secret); ok {
		t.Fatal("a forged tombstone must not bridge the gap")
	}
	seal, err := audit.SealTombstone(secret, stone)
	if err != nil {
		t.Fatal(err)
	}
	insert(seal)
	if ok, rows, _, problem := verifyChain(ctx, database, secret); !ok || rows != 2 {
		t.Fatalf("a sealed gap must verify: ok=%v rows=%d %s", ok, rows, problem)
	}
}
