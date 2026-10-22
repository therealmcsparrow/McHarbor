// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

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

func TestSweepOrphanAgentBackupsRemovesOnlyStaleOwnedFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	stale := sweepFixture(t, dir, ".mcharbor-agent-backup-123", time.Hour, now)
	fresh := sweepFixture(t, dir, ".mcharbor-agent-backup-456", time.Minute, now)
	other := sweepFixture(t, dir, "backup-2026.tar", time.Hour, now)
	subdir := filepath.Join(dir, ".mcharbor-agent-backup-dir")
	if err := os.Mkdir(subdir, 0o750); err != nil {
		t.Fatal(err)
	}

	removed, bytes := SweepOrphanAgentBackups(dir, now)

	if removed != 1 || bytes != 3 {
		t.Errorf("removed=%d bytes=%d, want 1 and 3", removed, bytes)
	}
	if _, err := os.Lstat(stale); err == nil {
		t.Error("stale temp file was kept")
	}
	for _, p := range []string{fresh, other, subdir} {
		if _, err := os.Lstat(p); err != nil {
			t.Errorf("wrongly removed %s", p)
		}
	}
}

func TestSweepOrphanAgentBackupsMissingDir(t *testing.T) {
	if removed, _ := SweepOrphanAgentBackups(filepath.Join(t.TempDir(), "nope"), time.Now()); removed != 0 {
		t.Errorf("removed=%d", removed)
	}
}

func TestSweepOrphanAgentBackupsDescendsOneLevelIntoRunFolders(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	run := filepath.Join(dir, "run1")
	deep := filepath.Join(run, "nested")
	if err := os.MkdirAll(deep, 0o750); err != nil {
		t.Fatal(err)
	}
	stale := sweepFixture(t, run, ".mcharbor-agent-backup-777", time.Hour, now)
	fresh := sweepFixture(t, run, ".mcharbor-agent-backup-888", time.Minute, now)
	archive := sweepFixture(t, run, "mcharbor.tar", time.Hour, now)
	tooDeep := sweepFixture(t, deep, ".mcharbor-agent-backup-999", time.Hour, now)

	removed, bytes := SweepOrphanAgentBackups(dir, now)

	if removed != 1 || bytes != 3 {
		t.Errorf("removed=%d bytes=%d, want 1 and 3", removed, bytes)
	}
	if _, err := os.Lstat(stale); err == nil {
		t.Error("the orphan inside the run folder was kept")
	}
	for _, p := range []string{fresh, archive, tooDeep} {
		if _, err := os.Lstat(p); err != nil {
			t.Errorf("wrongly removed %s", p)
		}
	}
}

func TestSweepOrphanAgentBackupsDoesNotFollowSymlinkedFolders(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	now := time.Now()
	victim := sweepFixture(t, outside, ".mcharbor-agent-backup-555", time.Hour, now)
	if err := os.Symlink(outside, filepath.Join(dir, "linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if removed, _ := SweepOrphanAgentBackups(dir, now); removed != 0 {
		t.Errorf("removed=%d through a symlinked folder", removed)
	}
	if _, err := os.Lstat(victim); err != nil {
		t.Error("a file outside the backup root was removed through a symlink")
	}
}
