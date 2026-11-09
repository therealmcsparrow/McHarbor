// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// sweepDestinationPartials(dir string, now time.Time) (removed int, bytes int64) replaces SweepOrphanAgentBackups:
// the limit is one hour and both partial prefixes go.

func sweepFixture(t *testing.T, dir, name string, age time.Duration, now time.Time) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	mt := now.Add(-age)
	if err := os.Chtimes(p, mt, mt); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSweepDestinationPartialsRemovesOnlyStaleOwnedNamedFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	staleAgent := sweepFixture(t, dir, ".mcharbor-agent-backup-123", 61*time.Minute, now)
	staleLocal := sweepFixture(t, dir, ".mcharbor-backup-123", 61*time.Minute, now)
	tenMinutes := sweepFixture(t, dir, ".mcharbor-agent-backup-456", 30*time.Minute, now)
	fresh := sweepFixture(t, dir, ".mcharbor-backup-456", 59*time.Minute, now)
	other := sweepFixture(t, dir, "backup-2026.tar", 5*time.Hour, now)
	near := sweepFixture(t, dir, "mcharbor-backup-789", 5*time.Hour, now)
	subdir := filepath.Join(dir, ".mcharbor-agent-backup-dir")
	if err := os.Mkdir(subdir, 0o750); err != nil {
		t.Fatal(err)
	}

	removed, bytes := sweepDestinationPartials(dir, now)

	if removed != 2 || bytes != 6 {
		t.Errorf("removed=%d bytes=%d, want 2 and 6", removed, bytes)
	}
	for _, p := range []string{staleAgent, staleLocal} {
		if _, err := os.Lstat(p); err == nil {
			t.Errorf("stale temp file %s was kept", p)
		}
	}
	for _, p := range []string{tenMinutes, fresh, other, near, subdir} {
		if _, err := os.Lstat(p); err != nil {
			t.Errorf("wrongly removed %s", p)
		}
	}
}

func TestSweepDestinationPartialsKeepsAFileOlderThanTenMinutesButUnderAnHour(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	p := sweepFixture(t, dir, ".mcharbor-agent-backup-1", 20*time.Minute, now)
	if removed, _ := sweepDestinationPartials(dir, now); removed != 0 {
		t.Errorf("removed=%d, the ten-minute rule is gone", removed)
	}
	if _, err := os.Lstat(p); err != nil {
		t.Error("a 20-minute-old partial was removed")
	}
}

func TestSweepDestinationPartialsSkipsOwnedFilesWhateverTheirAge(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	owned := sweepFixture(t, dir, ".mcharbor-backup-owned", 40*time.Hour, now)
	t.Cleanup(ownTempPath(owned))
	if removed, _ := sweepDestinationPartials(dir, now); removed != 0 {
		t.Errorf("removed=%d, an owned partial must stay", removed)
	}
	if _, err := os.Lstat(owned); err != nil {
		t.Error("an owned partial was removed")
	}
}

func TestSweepDestinationPartialsMissingDir(t *testing.T) {
	if removed, _ := sweepDestinationPartials(filepath.Join(t.TempDir(), "nope"), time.Now()); removed != 0 {
		t.Errorf("removed=%d", removed)
	}
}

func TestSweepDestinationPartialsDescendsOneLevelIntoRunFolders(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	run := filepath.Join(dir, "run1")
	deep := filepath.Join(run, "nested")
	if err := os.MkdirAll(deep, 0o750); err != nil {
		t.Fatal(err)
	}
	stale := sweepFixture(t, run, ".mcharbor-agent-backup-777", 2*time.Hour, now)
	staleLocal := sweepFixture(t, run, ".mcharbor-backup-777", 2*time.Hour, now)
	fresh := sweepFixture(t, run, ".mcharbor-agent-backup-888", time.Minute, now)
	archive := sweepFixture(t, run, "mcharbor.tar", 2*time.Hour, now)
	tooDeep := sweepFixture(t, deep, ".mcharbor-agent-backup-999", 2*time.Hour, now)

	removed, bytes := sweepDestinationPartials(dir, now)

	if removed != 2 || bytes != 6 {
		t.Errorf("removed=%d bytes=%d, want 2 and 6", removed, bytes)
	}
	for _, p := range []string{stale, staleLocal} {
		if _, err := os.Lstat(p); err == nil {
			t.Errorf("the orphan %s inside the run folder was kept", p)
		}
	}
	for _, p := range []string{fresh, archive, tooDeep} {
		if _, err := os.Lstat(p); err != nil {
			t.Errorf("wrongly removed %s", p)
		}
	}
}

func TestSweepDestinationPartialsDoesNotFollowSymlinkedFolders(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	now := time.Now()
	victim := sweepFixture(t, outside, ".mcharbor-agent-backup-555", 2*time.Hour, now)
	if err := os.Symlink(outside, filepath.Join(dir, "linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if removed, _ := sweepDestinationPartials(dir, now); removed != 0 {
		t.Errorf("removed=%d through a symlinked folder", removed)
	}
	if _, err := os.Lstat(victim); err != nil {
		t.Error("a file outside the backup root was removed through a symlink")
	}
}

func TestSweepDestinationPartialsDoesNotRemoveASymlinkTarget(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	now := time.Now()
	target := sweepFixture(t, outside, "precious.tar", 5*time.Hour, now)
	if err := os.Symlink(target, filepath.Join(dir, ".mcharbor-backup-link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	sweepDestinationPartials(dir, now)
	if _, err := os.Lstat(target); err != nil {
		t.Error("the file a partial-named symlink points at was removed")
	}
}
