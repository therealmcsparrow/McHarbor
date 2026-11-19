// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/xid"
)

// settleFixture is one run with a plan, its destinations and an archive built in a temp folder.
type settleFixture struct {
	svc      *Service
	database *sql.DB
	runID    string
	staged   string
}

func newSettleFixture(t *testing.T, planKeeps *bool, destStatuses ...string) settleFixture {
	t.Helper()
	svc, database := newStageService(t)
	f := settleFixture{svc: svc, database: database, runID: xid.New().String()}
	f.staged = filepath.Join(t.TempDir(), "mcharbor-run-"+f.runID, "mcharbor.tar")
	if err := os.MkdirAll(filepath.Dir(f.staged), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.staged, []byte("archive-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	planID := any(nil)
	if planKeeps != nil {
		planID = "plan1"
		mustExec(t, database, `INSERT INTO container_backup_plans (id, name, environment_id, container_id, container_name, selected_mounts, keep_local_copy, created_at, updated_at)
			VALUES ('plan1', 'p', 'envA', 'web', 'web', '[]', ?, ?, ?)`, *planKeeps, now, now)
	}
	mustExec(t, database, `INSERT INTO container_backup_runs (id, plan_id, environment_id, container_id, status, started_at, created_at, updated_at, operation)
		VALUES (?, ?, 'envA', 'web', 'running', ?, ?, ?, 'backup')`, f.runID, planID, now, now, now)
	for i, status := range destStatuses {
		mustExec(t, database, `INSERT INTO container_backup_run_destinations (id, run_id, storage_location_name, location_type, status, remote_path, created_at, updated_at)
			VALUES (?, ?, 'Mount', 'local', ?, ?, ?, ?)`, xid.New().String(), f.runID, status, filepath.Join("/mnt/backup", f.runID, string(rune('a'+i))+".tar"), now, now)
	}
	return f
}

func (f settleFixture) settle(t *testing.T, planID string, complete bool) archiveResult {
	t.Helper()
	got, err := f.svc.settleArchive(context.Background(), planID, f.runID, archiveResult{path: f.staged, size: 13}, complete)
	if err != nil {
		t.Fatalf("settleArchive: %v", err)
	}
	return got
}

func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return err == nil
}

func ptr(b bool) *bool { return &b }

// TestSettleNeverCopiesARedundantArchiveToTheServer is the fix for the full system disk: an archive
// every destination holds, from a plan that does not keep a copy, never reaches the data directory.
func TestSettleNeverCopiesARedundantArchiveToTheServer(t *testing.T) {
	f := newSettleFixture(t, ptr(false), "success", "success")
	got := f.settle(t, "plan1", true)
	managed := f.svc.managedArchivePath(f.runID)
	if exists(t, managed) {
		t.Fatal("the archive was copied to the server although every destination holds it")
	}
	if exists(t, f.staged) {
		t.Fatal("the staged archive was left in the temp folder")
	}
	if !got.dropped || got.path != managed {
		t.Fatalf("settled = %+v, want the managed path marked dropped so restore fetches it back", got)
	}
}

// TestSettleKeepsTheArchiveWhenItIsNeeded covers the cases where the server copy is the one to keep:
// the plan asks for it, or a destination failed and a retry will read it.
func TestSettleKeepsTheArchiveWhenItIsNeeded(t *testing.T) {
	cases := map[string]struct {
		keep   *bool
		planID string
		dests  []string
	}{
		"plan keeps a copy":          {keep: ptr(true), planID: "plan1", dests: []string{"success"}},
		"a destination failed":       {keep: ptr(false), planID: "plan1", dests: []string{"success", "failure"}},
		"ad-hoc run with a failure":  {keep: nil, planID: "", dests: []string{"failure"}},
		"ad-hoc run, no destination": {keep: nil, planID: "", dests: nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newSettleFixture(t, tc.keep, tc.dests...)
			got := f.settle(t, tc.planID, true)
			managed := f.svc.managedArchivePath(f.runID)
			if !exists(t, managed) || got.path != managed || got.dropped {
				t.Fatalf("settled = %+v, archive kept = %v; want it moved to %s", got, exists(t, managed), managed)
			}
			if exists(t, f.staged) {
				t.Fatal("the staged archive was left in the temp folder")
			}
		})
	}
}

// TestSettleDropsAnAdhocRunEveryDestinationHolds: an ad-hoc run follows the not-kept default.
func TestSettleDropsAnAdhocRunEveryDestinationHolds(t *testing.T) {
	f := newSettleFixture(t, nil, "success")
	got := f.settle(t, "", true)
	if exists(t, f.svc.managedArchivePath(f.runID)) || !got.dropped {
		t.Fatalf("settled = %+v; an ad-hoc run every destination holds must not keep a server copy", got)
	}
}

// TestSettleDiscardsAnIncompleteArchive: a half-written archive is never kept anywhere.
func TestSettleDiscardsAnIncompleteArchive(t *testing.T) {
	f := newSettleFixture(t, ptr(true), "failure")
	got := f.settle(t, "plan1", false)
	if got.path != "" || exists(t, f.staged) || exists(t, f.svc.managedArchivePath(f.runID)) {
		t.Fatalf("settled = %+v; an incomplete archive must be removed and not recorded", got)
	}
}

// copySweepFixture seeds managed run folders and rows for sweepLocalCopies.
type copySweepFixture struct {
	svc      *Service
	database *sql.DB
	dataDir  string
	now      time.Time
}

func newCopySweepFixture(t *testing.T) copySweepFixture {
	t.Helper()
	svc, database := newStageService(t)
	now := time.Now().UTC()
	stamp := now.Format(time.RFC3339)
	mustExec(t, database, `INSERT INTO container_backup_plans (id, name, environment_id, container_id, container_name, selected_mounts, keep_local_copy, created_at, updated_at)
		VALUES ('keep', 'k', 'envA', 'web', 'web', '[]', 1, ?, ?), ('nokeep', 'n', 'envA', 'web', 'web', '[]', 0, ?, ?)`, stamp, stamp, stamp, stamp)
	return copySweepFixture{svc: svc, database: database, dataDir: svc.dataDir, now: now}
}

// run seeds one run with a managed archive on disk and returns its id.
func (f copySweepFixture) run(t *testing.T, planID any, status string, completed time.Time, destStatus string) string {
	t.Helper()
	id := xid.New().String()
	archive := f.svc.managedArchivePath(id)
	if err := os.MkdirAll(filepath.Dir(archive), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archive, []byte("archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	stamp := completed.UTC().Format(time.RFC3339)
	mustExec(t, f.database, `INSERT INTO container_backup_runs (id, plan_id, environment_id, container_id, status, archive_path, archive_size, started_at, completed_at, created_at, updated_at, operation)
		VALUES (?, ?, 'envA', 'web', ?, ?, 7, ?, ?, ?, ?, 'backup')`, id, planID, status, archive, stamp, stamp, stamp, stamp)
	if destStatus != "" {
		mustExec(t, f.database, `INSERT INTO container_backup_run_destinations (id, run_id, storage_location_name, location_type, status, remote_path, created_at, updated_at)
			VALUES (?, ?, 'Mount', 'local', ?, '/mnt/backup/x.tar', ?, ?)`, xid.New().String(), id, destStatus, stamp, stamp)
	}
	return id
}

func (f copySweepFixture) sweep(t *testing.T) localCopySweepResult {
	t.Helper()
	res, err := sweepLocalCopies(context.Background(), f.database, f.dataDir, f.now)
	if err != nil {
		t.Fatalf("sweepLocalCopies: %v", err)
	}
	return res
}

func (f copySweepFixture) archiveThere(t *testing.T, id string) bool {
	t.Helper()
	return exists(t, f.svc.managedArchivePath(id))
}

// TestSweepClearsTheCopiesNothingNeeds is the cleanup of copies already on disk: redundant copies of
// plans that do not keep one and of ad-hoc runs go, and the row says so; kept and still-needed ones stay.
func TestSweepClearsTheCopiesNothingNeeds(t *testing.T) {
	f := newCopySweepFixture(t)
	redundant := f.run(t, "nokeep", "success", f.now, "success")
	adhoc := f.run(t, nil, "success", f.now, "success")
	kept := f.run(t, "keep", "success", f.now, "success")
	noDestination := f.run(t, nil, "success", f.now, "")
	recentFailure := f.run(t, "nokeep", "failure", f.now.Add(-time.Hour), "failure")
	oldFailure := f.run(t, "nokeep", "failure", f.now.Add(-8*24*time.Hour), "failure")

	res := f.sweep(t)

	for name, id := range map[string]string{"redundant": redundant, "ad-hoc": adhoc, "old failure": oldFailure} {
		if f.archiveThere(t, id) {
			t.Errorf("%s: archive survived the sweep", name)
		}
	}
	for name, id := range map[string]string{"kept": kept, "no destination": noDestination, "recent failure": recentFailure} {
		if !f.archiveThere(t, id) {
			t.Errorf("%s: archive was swept but is still needed", name)
		}
	}
	if res.removed != 3 {
		t.Errorf("removed = %d, want 3", res.removed)
	}
	var dropped bool
	if err := f.database.QueryRow(`SELECT local_copy_dropped FROM container_backup_runs WHERE id = ?`, redundant).Scan(&dropped); err != nil || !dropped {
		t.Errorf("redundant run is not marked dropped (%v, %v), so restore would not fetch it back", dropped, err)
	}
	var path sql.NullString
	if err := f.database.QueryRow(`SELECT archive_path FROM container_backup_runs WHERE id = ?`, oldFailure).Scan(&path); err != nil || path.Valid {
		t.Errorf("old failure still records archive path %v (%v)", path, err)
	}
}

// TestSweepLeavesARunRecordedElsewhereAlone: a run whose archive path is not the managed one keeps its row.
func TestSweepLeavesARunRecordedElsewhereAlone(t *testing.T) {
	f := newCopySweepFixture(t)
	id := f.run(t, "nokeep", "success", f.now, "success")
	mustExec(t, f.database, `UPDATE container_backup_runs SET archive_path = '/elsewhere/mcharbor.tar' WHERE id = ?`, id)
	f.sweep(t)
	var dropped bool
	if err := f.database.QueryRow(`SELECT local_copy_dropped FROM container_backup_runs WHERE id = ?`, id).Scan(&dropped); err != nil || dropped {
		t.Fatalf("a run recorded outside the managed directory was marked dropped (%v, %v)", dropped, err)
	}
}

// TestSweepRemovesOnlyOldOrphanArchiveFolders: a folder with no run row goes once it is old and holds
// only archive files; anything else in the backup directory is left untouched.
func TestSweepRemovesOnlyOldOrphanArchiveFolders(t *testing.T) {
	f := newCopySweepFixture(t)
	root := LocalBackupDir(f.dataDir)
	mk := func(name string, age time.Duration, files ...string) string {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			if err := os.WriteFile(filepath.Join(dir, file), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		old := f.now.Add(-age)
		if err := os.Chtimes(dir, old, old); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	orphan := mk(xid.New().String(), 48*time.Hour, "mcharbor.tar")
	fresh := mk(xid.New().String(), time.Hour, "mcharbor.tar")
	foreign := mk(xid.New().String(), 48*time.Hour, "mcharbor.tar", "notes.txt")
	notARun := mk("envA", 48*time.Hour, "mcharbor.tar")

	f.sweep(t)

	if exists(t, orphan) {
		t.Error("an old orphan archive folder survived")
	}
	for name, dir := range map[string]string{"fresh": fresh, "foreign file": foreign, "not a run id": notARun} {
		if !exists(t, dir) {
			t.Errorf("%s: folder was removed", name)
		}
	}
}
