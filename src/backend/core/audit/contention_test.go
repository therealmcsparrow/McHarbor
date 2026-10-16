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
)

// TestAPluginAuditWriteWaitsOutAnotherWriter is the SQLITE_BUSY lost during
// the seventh LXC pass: the hash-chain insert read the previous row and then
// tried to write, and SQLite never runs the busy handler for a reader asking
// to become a writer, so a writer holding the lock for longer than the retry
// budget cost the entry. Taking the write lock first makes busy_timeout apply.
func TestAPluginAuditWriteWaitsOutAnotherWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "contention.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_journal_mode=WAL&_busy_timeout=10000")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeTest(t, db) })
	db.SetMaxOpenConns(4)
	if _, err := db.Exec(`
		CREATE TABLE audit_logs (
			id TEXT PRIMARY KEY, user_id TEXT, username TEXT, action TEXT NOT NULL, severity TEXT NOT NULL,
			entity_type TEXT, entity_id TEXT, entity_name TEXT, details TEXT, state_before TEXT,
			state_after TEXT, changed_keys TEXT, ip_address TEXT, environment_id TEXT, environment_name TEXT,
			timestamp TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, prev_hash TEXT,
			row_hash TEXT, api_key_id TEXT, api_key_name TEXT, node_id TEXT, exec_node_id TEXT
		);
		CREATE TABLE progress (id TEXT PRIMARY KEY, stage TEXT);
	`); err != nil {
		t.Fatal(err)
	}
	logger := NewLogger(db, "test-secret")
	if err := logger.EmitPlugin("addon-lxc", "plugin.lxc.first", nil); err != nil {
		t.Fatalf("the uncontended write failed: %v", err)
	}

	ctx := context.Background()
	holder, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer closeTest(t, holder)
	if _, err := holder.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	if _, err := holder.ExecContext(ctx, `INSERT INTO progress (id, stage) VALUES ('run1', 'uploading')`); err != nil {
		t.Fatal(err)
	}
	released := make(chan error, 1)
	go func() {
		// Far longer than the busy-retry budget, far shorter than busy_timeout:
		// exactly the window a plain progress write can occupy under load.
		time.Sleep(600 * time.Millisecond)
		_, err := holder.ExecContext(ctx, "COMMIT")
		released <- err
	}()

	if err := logger.EmitPluginSeverity("addon-lxc", "plugin.storage.uploaded", "info",
		map[string]any{"runId": "run1"}); err != nil {
		t.Fatalf("the audit entry was lost while another connection held the write lock: %v", err)
	}
	if err := <-released; err != nil {
		t.Fatal(err)
	}

	var rows int
	var prev sql.NullString
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_logs`).Scan(&rows); err != nil || rows != 2 {
		t.Fatalf("audit rows = %d, %v; want 2", rows, err)
	}
	if err := db.QueryRow(`SELECT prev_hash FROM audit_logs WHERE action = 'plugin.storage.uploaded'`).Scan(&prev); err != nil ||
		prev.String == "" {
		t.Fatalf("the contended row did not extend the hash chain: %q, %v", prev.String, err)
	}

	var inTx int
	for i := 0; i < 4; i++ {
		// A connection handed back mid-transaction would make the next writer on
		// it fail; every pooled connection must be able to open one.
		if _, err := db.Exec(`INSERT INTO progress (id, stage) VALUES (?, 'x')`, "probe-"+string(rune('a'+i))); err != nil {
			inTx++
		}
	}
	if inTx != 0 {
		t.Fatalf("%d pooled connections were left inside a transaction", inTx)
	}
}
