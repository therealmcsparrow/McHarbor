// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// localCopyFixture is one finished plan run whose archive was copied to a local destination.
type localCopyFixture struct {
	svc      *Service
	database *sql.DB
	runID    string
	archive  string
	remote   string
	content  []byte
}

func newLocalCopyFixture(t *testing.T, keepLocal bool, destStatus string) localCopyFixture {
	t.Helper()
	svc, database := newStageService(t)
	f := localCopyFixture{svc: svc, database: database, runID: "run1", content: []byte("encrypted-archive-bytes")}
	f.archive = svc.managedArchivePath(f.runID)
	if err := os.MkdirAll(filepath.Dir(f.archive), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.archive, f.content, 0o600); err != nil {
		t.Fatal(err)
	}
	f.remote = filepath.Join(t.TempDir(), "remote", "mcharbor.tar")
	if err := os.MkdirAll(filepath.Dir(f.remote), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.remote, f.content, 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	mustExec(t, database, `INSERT INTO container_backup_plans (id, name, environment_id, container_id, container_name, selected_mounts, keep_local_copy, created_at, updated_at)
		VALUES ('plan1', 'p', 'envA', 'web', 'web', '[]', ?, ?, ?)`, keepLocal, now, now)
	mustExec(t, database, `INSERT INTO container_backup_runs (id, plan_id, environment_id, container_id, status, archive_path, archive_size, started_at, created_at, updated_at, operation)
		VALUES (?, 'plan1', 'envA', 'web', 'success', ?, ?, ?, ?, ?, 'backup')`, f.runID, f.archive, len(f.content), now, now, now)
	mustExec(t, database, `INSERT INTO container_backup_run_destinations (id, run_id, storage_location_name, location_type, status, remote_path, created_at, updated_at)
		VALUES ('dest1', ?, 'Local', 'local', ?, ?, ?, ?)`, f.runID, destStatus, f.remote, now, now)
	return f
}

func mustExec(t *testing.T, database *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := database.Exec(query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func (f localCopyFixture) finish(t *testing.T) *BackupRun {
	t.Helper()
	run, err := f.svc.runByID(context.Background(), f.runID)
	if err != nil {
		t.Fatal(err)
	}
	f.svc.dropLocalCopyAfterRun(context.Background(), run)
	return run
}

func (f localCopyFixture) dropped(t *testing.T) bool {
	t.Helper()
	var dropped bool
	if err := f.database.QueryRow(`SELECT local_copy_dropped FROM container_backup_runs WHERE id = ?`, f.runID).Scan(&dropped); err != nil {
		t.Fatal(err)
	}
	return dropped
}

// TestDropLocalCopyRemovesTheServerArchive: a plan that does not keep a local copy loses the server's
// archive once every destination holds it, and the run says so.
func TestDropLocalCopyRemovesTheServerArchive(t *testing.T) {
	f := newLocalCopyFixture(t, false, "success")
	f.finish(t)
	if _, err := os.Stat(f.archive); !os.IsNotExist(err) {
		t.Fatalf("the server archive is still there (%v)", err)
	}
	if !f.dropped(t) {
		t.Fatal("the run does not record that its local copy was dropped")
	}
	if _, err := os.Stat(f.remote); err != nil {
		t.Fatalf("the destination copy must stay: %v", err)
	}
}

// TestDropLocalCopyKeepsTheArchive covers the two cases where the server copy is the one to keep.
func TestDropLocalCopyKeepsTheArchive(t *testing.T) {
	for name, f := range map[string]localCopyFixture{
		"plan keeps the local copy": newLocalCopyFixture(t, true, "success"),
		"a destination failed":      newLocalCopyFixture(t, false, "failure"),
	} {
		f.finish(t)
		if _, err := os.Stat(f.archive); err != nil {
			t.Errorf("%s: the server archive was removed: %v", name, err)
		}
		if f.dropped(t) {
			t.Errorf("%s: the run claims its local copy was dropped", name)
		}
	}
}

// TestEnsureLocalArchiveFetchesItBack: restore and download read the archive from disk, so a
// dropped copy comes back from the destination byte for byte.
func TestEnsureLocalArchiveFetchesItBack(t *testing.T) {
	f := newLocalCopyFixture(t, false, "success")
	f.finish(t)
	if err := f.svc.ensureLocalArchive(context.Background(), f.runID, f.archive); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(f.archive)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, f.content) {
		t.Fatalf("fetched %q, want %q", got, f.content)
	}
	download, err := f.svc.Download(context.Background(), f.runID)
	if err != nil || download == nil || download.Size != int64(len(f.content)) {
		t.Fatalf("download after fetch: %+v, %v", download, err)
	}
}

// TestEnsureLocalArchiveRefusesAShortCopy: a destination file that does not match the recorded size
// is never put where a restore would read it.
func TestEnsureLocalArchiveRefusesAShortCopy(t *testing.T) {
	f := newLocalCopyFixture(t, false, "success")
	f.finish(t)
	if err := os.WriteFile(f.remote, f.content[:5], 0o600); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.ensureLocalArchive(context.Background(), f.runID, f.archive); err == nil {
		t.Fatal("a truncated destination copy was accepted")
	}
	entries, err := os.ReadDir(filepath.Dir(f.archive))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("a refused fetch left %d files behind", len(entries))
	}
}

// TestSweepRemovesOnlyStaleFetchedCopies: a fetched archive goes once it has sat unused past the grace.
func TestSweepRemovesOnlyStaleFetchedCopies(t *testing.T) {
	f := newLocalCopyFixture(t, false, "success")
	f.finish(t)
	if err := f.svc.ensureLocalArchive(context.Background(), f.runID, f.archive); err != nil {
		t.Fatal(err)
	}
	f.svc.sweepRefetchedCopies(context.Background())
	if _, err := os.Stat(f.archive); err != nil {
		t.Fatalf("a fresh fetch was swept: %v", err)
	}
	old := time.Now().Add(-2 * localCopyRefetchGrace)
	if err := os.Chtimes(f.archive, old, old); err != nil {
		t.Fatal(err)
	}
	f.svc.sweepRefetchedCopies(context.Background())
	if _, err := os.Stat(f.archive); !os.IsNotExist(err) {
		t.Fatalf("a stale fetch survived the sweep (%v)", err)
	}
}
