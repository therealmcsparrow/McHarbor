// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Seams the sweep (temp_sweep.go) provides, fixed here so the tests compile against one shape:
//
//	type tempSweepResult struct { Removed int; Bytes int64; KeptOwned int; OlderThan2h int }
//	func sweepTemp(ctx, db *sql.DB, dataDir string, getenv func(string) string, now time.Time) (tempSweepResult, error)
//	func cleanupTemp(ctx, db *sql.DB, dataDir string, getenv func(string) string, now time.Time) (BackupTempCleanupResult, error)
//	type BackupTempCleanupResult struct { RemovedEntries int; RemovedBytes int64; KeptOwned, KeptRecent, ActiveRuns int }

type swFixture struct {
	t       *testing.T
	db      *sql.DB
	dataDir string
	root    string
	now     time.Time
}

func newSwFixture(t *testing.T) *swFixture {
	t.Helper()
	f := &swFixture{t: t, db: tempTestDB(t), dataDir: filepath.Join(t.TempDir(), "data"), root: filepath.Join(t.TempDir(), "spool"), now: time.Now()}
	if err := os.MkdirAll(f.root, 0o750); err != nil {
		t.Fatal(err)
	}
	prev := tempCfg.Load()
	configureTempRoot(f.db, f.dataDir, f.getenv)
	t.Cleanup(func() { tempCfg.Store(prev) })
	return f
}

func (f *swFixture) getenv(key string) string {
	if key == tempDirEnv {
		return f.root
	}
	return ""
}

// entry creates a top-level entry in dir: a folder holding one 10-byte file, aged with Chtimes.
func (f *swFixture) entry(dir, name string, age time.Duration) string {
	f.t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(p, 0o700); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "spool.bin"), []byte("0123456789"), 0o600); err != nil {
		f.t.Fatal(err)
	}
	f.age(p, age)
	return p
}

func (f *swFixture) file(dir, name string, age time.Duration) string {
	f.t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		f.t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("abc"), 0o600); err != nil {
		f.t.Fatal(err)
	}
	f.age(p, age)
	return p
}

func (f *swFixture) age(p string, age time.Duration) {
	f.t.Helper()
	mt := f.now.Add(-age)
	if err := os.Chtimes(p, mt, mt); err != nil {
		f.t.Fatal(err)
	}
}

func (f *swFixture) sweep() tempSweepResult {
	f.t.Helper()
	res, err := sweepTemp(context.Background(), f.db, f.dataDir, f.getenv, f.now)
	if err != nil {
		f.t.Fatalf("sweepTemp: %v", err)
	}
	return res
}

func (f *swFixture) cleanup() BackupTempCleanupResult {
	f.t.Helper()
	res, err := cleanupTemp(context.Background(), f.db, f.dataDir, f.getenv, f.now)
	if err != nil {
		f.t.Fatalf("cleanupTemp: %v", err)
	}
	return res
}

func (f *swFixture) setRetired(roots ...string) {
	f.t.Helper()
	raw, err := json.Marshal(roots)
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err := f.db.Exec(`INSERT INTO settings (id, key, value, category) VALUES ('s-ret', 'backup_temp_dir_retired', ?, 'backups')`, string(raw)); err != nil {
		f.t.Fatal(err)
	}
}

func (f *swFixture) retired() []string {
	f.t.Helper()
	roots, err := retiredTempRoots(context.Background(), f.db)
	if err != nil {
		f.t.Fatal(err)
	}
	return roots
}

func (f *swFixture) addLocalLocation(id, basePath string) {
	f.t.Helper()
	_, err := f.db.Exec(`INSERT INTO storage_locations (id, name, location_type, enabled, base_path, created_at, updated_at)
		VALUES (?, ?, 'local', 1, ?, '2026-11-09T00:00:00Z', '2026-11-09T00:00:00Z')`, id, id, basePath)
	if err != nil {
		f.t.Fatal(err)
	}
}

func swExists(t *testing.T, p string) bool {
	t.Helper()
	_, err := os.Lstat(p)
	if err == nil {
		return true
	}
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	t.Fatalf("lstat %s: %v", p, err)
	return false
}

func TestSweepAppliesTheOneHourRule(t *testing.T) {
	f := newSwFixture(t)
	old := f.entry(f.root, "mcharbor-run-old", 61*time.Minute)
	young := f.entry(f.root, "mcharbor-run-young", 59*time.Minute)
	res := f.sweep()
	if swExists(t, old) {
		t.Error("an entry 61 minutes old was kept")
	}
	if !swExists(t, young) {
		t.Error("an entry 59 minutes old was removed")
	}
	if res.Removed != 1 || res.Bytes != 10 {
		t.Errorf("result = %+v, want removed 1 and 10 bytes", res)
	}
}

func TestSweepRemovesEveryMcHarborTempPrefix(t *testing.T) {
	f := newSwFixture(t)
	names := []string{"mcharbor-run-a", "mcharbor-backup-entry-a", "mcharbor-frozen-a", "mcharbor-probe-a", "restore-subset-a"}
	for _, n := range names {
		f.entry(f.root, n, 3*time.Hour)
	}
	res := f.sweep()
	for _, n := range names {
		if swExists(t, filepath.Join(f.root, n)) {
			t.Errorf("%s survived", n)
		}
	}
	if res.Removed != 5 || res.Bytes != 50 {
		t.Errorf("result = %+v, want removed 5 and 50 bytes", res)
	}
}

func TestSweepSkipsOwnedWhateverAge(t *testing.T) {
	f := newSwFixture(t)
	_, dir, release, err := acquireRunTemp(context.Background(), "owned1")
	if err != nil {
		t.Fatalf("acquireRunTemp: %v", err)
	}
	t.Cleanup(release)
	f.age(dir, 100*time.Hour)
	partial := f.file(f.root, "mcharbor-backup-entry-owned.bin", 100*time.Hour)
	done := ownTempPath(partial)
	t.Cleanup(done)
	stale := f.entry(f.root, "mcharbor-run-stale", 2*time.Hour)

	res := f.sweep()

	if !swExists(t, dir) {
		t.Error("the active run folder was removed")
	}
	if !swExists(t, partial) {
		t.Error("an owned partial was removed")
	}
	if swExists(t, stale) {
		t.Error("the unowned stale entry was kept")
	}
	if res.Removed != 1 || res.KeptOwned < 1 {
		t.Errorf("result = %+v, want removed 1 and keptOwned >= 1", res)
	}

	release()
	if swExists(t, dir) {
		t.Error("release must delete the run folder itself")
	}
	done()
	res = f.sweep()
	if swExists(t, partial) {
		t.Error("a released partial older than the limit must go on the next sweep")
	}
	if res.Removed != 1 {
		t.Errorf("after release removed = %d, want 1 (the partial; the run folder is already gone)", res.Removed)
	}
}

func TestSweepNeverTouchesForeignEntries(t *testing.T) {
	f := newSwFixture(t)
	foreign := []string{
		"mcharbor-run", "xmcharbor-run-1", "Mcharbor-run-1", "restore-subset", "restore_subset-1",
		".mcharbor-run-1", "mcharbor-backup-1", "important-data", "mcharbor",
	}
	var paths []string
	for _, n := range foreign {
		paths = append(paths, f.entry(f.root, n, 500*time.Hour))
	}
	paths = append(paths, f.file(f.root, "notes.txt", 500*time.Hour))
	res := f.sweep()
	for _, p := range paths {
		if !swExists(t, p) {
			t.Errorf("foreign entry %s was removed", p)
		}
	}
	if res.Removed != 0 || res.Bytes != 0 {
		t.Errorf("result = %+v, want nothing removed", res)
	}
}

func TestSweepDoesNotFollowSymlinksOutOfTheRoot(t *testing.T) {
	f := newSwFixture(t)
	outside := t.TempDir()
	victimDir := f.entry(outside, "mcharbor-run-victim", 500*time.Hour)
	victimFile := f.file(outside, "mcharbor-backup-entry-victim.bin", 500*time.Hour)
	if err := os.Symlink(victimDir, filepath.Join(f.root, "mcharbor-run-linkdir")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(victimFile, filepath.Join(f.root, "mcharbor-backup-entry-linkfile")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	inner := f.entry(f.root, "mcharbor-run-inner", 3*time.Hour)
	if err := os.Symlink(victimFile, filepath.Join(inner, "escape")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	f.age(inner, 3*time.Hour)

	f.sweep()
	f.cleanup()

	if !swExists(t, victimDir) || !swExists(t, filepath.Join(victimDir, "spool.bin")) {
		t.Error("a folder outside the root was removed through a symlink")
	}
	if !swExists(t, victimFile) {
		t.Error("a file outside the root was removed through a symlink")
	}
}

func TestSweepAgesByTheEntryItself(t *testing.T) {
	f := newSwFixture(t)
	dir := f.entry(f.root, "mcharbor-run-fresh-dir", 5*time.Hour)
	f.age(filepath.Join(dir, "spool.bin"), 5*time.Hour)
	f.age(dir, 10*time.Minute)
	f.sweep()
	if !swExists(t, dir) {
		t.Error("a folder touched ten minutes ago was removed")
	}
}

func TestSweepCountsEntriesOlderThanTwoHoursBeforeRemoval(t *testing.T) {
	f := newSwFixture(t)
	f.entry(f.root, "mcharbor-run-3h", 3*time.Hour)
	f.entry(f.root, "mcharbor-run-90m", 90*time.Minute)
	f.entry(f.root, "mcharbor-run-30m", 30*time.Minute)
	res := f.sweep()
	if res.OlderThan2h != 1 {
		t.Errorf("OlderThan2h = %d, want 1", res.OlderThan2h)
	}
	if res.Removed != 2 {
		t.Errorf("Removed = %d, want 2", res.Removed)
	}
	if res = f.sweep(); res.OlderThan2h != 0 || res.Removed != 0 {
		t.Errorf("second sweep = %+v, want all zero", res)
	}
}

func TestSweepClearsRetiredRootsOnceUnusedAndDropsThemFromTheList(t *testing.T) {
	f := newSwFixture(t)
	cleared := filepath.Join(t.TempDir(), "retired-cleared")
	gone := filepath.Join(t.TempDir(), "retired-gone")
	busy := filepath.Join(t.TempDir(), "retired-busy")
	foreignOnly := filepath.Join(t.TempDir(), "retired-foreign")
	staleEntry := f.entry(cleared, "mcharbor-run-x", 4*time.Hour)
	youngEntry := f.entry(busy, "mcharbor-run-y", 5*time.Minute)
	foreign := f.entry(foreignOnly, "their-data", 500*time.Hour)
	f.setRetired(cleared, gone, busy, foreignOnly)

	f.sweep()

	if swExists(t, staleEntry) {
		t.Error("the stale entry in a retired root was kept")
	}
	if !swExists(t, youngEntry) || !swExists(t, foreign) {
		t.Error("a young or foreign entry in a retired root was removed")
	}
	got := f.retired()
	if len(got) != 1 || got[0] != busy {
		t.Errorf("retired list = %v, want only %q", got, busy)
	}
}

func TestSweepKeepsARetiredRootWhileARunOwnsAnEntryInIt(t *testing.T) {
	f := newSwFixture(t)
	old := filepath.Join(t.TempDir(), "retired-live")
	dir := f.entry(old, "mcharbor-run-live", 9*time.Hour)
	t.Cleanup(ownTempPath(dir))
	f.setRetired(old)
	f.sweep()
	if !swExists(t, dir) {
		t.Error("an owned run folder in a retired root was removed")
	}
	if got := f.retired(); len(got) != 1 || got[0] != old {
		t.Errorf("retired list = %v, want the root kept while owned", got)
	}
}

func TestSweepRemovesDestinationPartialsInLocalDirAndLocalLocations(t *testing.T) {
	f := newSwFixture(t)
	localDir := LocalBackupDir(f.dataDir)
	loc := filepath.Join(t.TempDir(), "nas")
	f.addLocalLocation("loc-1", loc)
	staleA := f.file(localDir, ".mcharbor-backup-1", 2*time.Hour)
	staleB := f.file(localDir, ".mcharbor-agent-backup-1", 2*time.Hour)
	youngA := f.file(localDir, ".mcharbor-backup-2", 5*time.Minute)
	staleC := f.file(loc, ".mcharbor-backup-3", 2*time.Hour)
	staleD := f.file(loc, ".mcharbor-agent-backup-3", 2*time.Hour)
	youngD := f.file(loc, ".mcharbor-agent-backup-4", 5*time.Minute)
	archive := f.file(loc, "backup-2026.tar", 500*time.Hour)
	res := f.sweep()
	for _, p := range []string{staleA, staleB, staleC, staleD} {
		if swExists(t, p) {
			t.Errorf("stale partial %s was kept", p)
		}
	}
	for _, p := range []string{youngA, youngD, archive} {
		if !swExists(t, p) {
			t.Errorf("%s was removed", p)
		}
	}
	if res.Removed != 4 || res.Bytes != 12 {
		t.Errorf("result = %+v, want removed 4 and 12 bytes", res)
	}
}

func TestSweepSkipsAnOwnedDestinationPartial(t *testing.T) {
	f := newSwFixture(t)
	partial := f.file(LocalBackupDir(f.dataDir), ".mcharbor-agent-backup-live", 30*time.Hour)
	t.Cleanup(ownTempPath(partial))
	f.sweep()
	if !swExists(t, partial) {
		t.Error("a partial an upload still owns was removed")
	}
}

func TestSweepIgnoresNonLocalStorageLocations(t *testing.T) {
	f := newSwFixture(t)
	other := filepath.Join(t.TempDir(), "smb-mount")
	victim := f.file(other, ".mcharbor-backup-1", 5*time.Hour)
	if _, err := f.db.Exec(`INSERT INTO storage_locations (id, name, location_type, enabled, base_path, created_at, updated_at)
		VALUES ('smb-1', 'smb', 'smb', 1, ?, '2026-11-09T00:00:00Z', '2026-11-09T00:00:00Z')`, other); err != nil {
		t.Fatal(err)
	}
	f.sweep()
	if !swExists(t, victim) {
		t.Error("a non-local storage location path was swept")
	}
}

func TestSweepWithMissingRootIsNotAnError(t *testing.T) {
	f := newSwFixture(t)
	f.root = filepath.Join(t.TempDir(), "never-created")
	if res := f.sweep(); res.Removed != 0 {
		t.Errorf("result = %+v", res)
	}
}

func TestCleanupWithNoRunRemovesEveryMcHarborEntryAtAnyAge(t *testing.T) {
	f := newSwFixture(t)
	retired := filepath.Join(t.TempDir(), "retired")
	f.entry(f.root, "mcharbor-run-old", 5*time.Hour)
	f.entry(f.root, "mcharbor-frozen-young", time.Minute)
	f.entry(retired, "restore-subset-young", time.Minute)
	foreign := f.entry(f.root, "keep-me", 5*time.Hour)
	stalePartial := f.file(LocalBackupDir(f.dataDir), ".mcharbor-backup-1", 9*time.Hour)
	f.setRetired(retired)

	res := f.cleanup()

	if res.RemovedEntries != 3 || res.RemovedBytes != 30 {
		t.Errorf("result = %+v, want 3 entries and 30 bytes", res)
	}
	if res.KeptOwned != 0 || res.KeptRecent != 0 || res.ActiveRuns != 0 {
		t.Errorf("result = %+v, want no kept entries and no active runs", res)
	}
	for _, p := range []string{filepath.Join(f.root, "mcharbor-run-old"), filepath.Join(f.root, "mcharbor-frozen-young"), filepath.Join(retired, "restore-subset-young")} {
		if swExists(t, p) {
			t.Errorf("%s survived the cleanup", p)
		}
	}
	if !swExists(t, foreign) {
		t.Error("cleanup deleted an entry McHarbor did not name")
	}
	if !swExists(t, stalePartial) {
		t.Error("cleanup touched a destination partial; those belong to the hourly sweep")
	}
}

func TestCleanupWithARunRemovesOnlyOldUnownedAndReportsWhatItKept(t *testing.T) {
	f := newSwFixture(t)
	_, owned, release, err := acquireRunTemp(context.Background(), "live1")
	if err != nil {
		t.Fatalf("acquireRunTemp: %v", err)
	}
	t.Cleanup(release)
	f.age(owned, 7*time.Hour)
	old := f.entry(f.root, "mcharbor-run-old", 3*time.Hour)
	young := f.entry(f.root, "mcharbor-backup-entry-young", 10*time.Minute)

	res := f.cleanup()

	if res.RemovedEntries != 1 || res.RemovedBytes != 10 {
		t.Errorf("result = %+v, want 1 entry and 10 bytes", res)
	}
	if res.KeptOwned != 1 || res.KeptRecent != 1 || res.ActiveRuns != 1 {
		t.Errorf("result = %+v, want keptOwned 1, keptRecent 1, activeRuns 1", res)
	}
	if swExists(t, old) {
		t.Error("the old unowned entry was kept")
	}
	if !swExists(t, owned) || !swExists(t, young) {
		t.Error("an owned or recent entry was removed while a run is active")
	}
}
