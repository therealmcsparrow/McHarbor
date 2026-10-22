// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func writeAged(t *testing.T, dir, name string, age time.Duration, now time.Time) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	mt := now.Add(-age)
	if err := os.Chtimes(p, mt, mt); err != nil {
		t.Fatal(err)
	}
	return p
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func TestSweepOrphanTempsRemovesStaleMatchingFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	var stale []string
	for _, n := range []string{
		"mcharbor-agent-backup-1.tar", "mcharbor-agent-backup-entry-2.tar",
		"mcharbor-agent-restore-3.tar", "mcharbor-agent-frozen-4.tar",
		"mcharbor-agent-image-load-5.tar", "mcharbor-agent-image-load-upload-6.tar",
		"mcharbor-agent-direct-image-7.tar", "mcharbor-agent-direct-archive-8.tar",
		".mcharbor-agent-image-9.tar",
	} {
		stale = append(stale, writeAged(t, dir, n, time.Hour, now))
	}
	res := sweepOrphanTemps(dir, now, slog.Default())
	for _, p := range stale {
		if exists(p) {
			t.Errorf("stale file kept: %s", p)
		}
	}
	if res.Removed != len(stale) || res.Bytes != int64(5*len(stale)) {
		t.Errorf("result = %+v, want %d removed", res, len(stale))
	}
}

func TestSweepOrphanTempsKeepsEverythingElse(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	fresh := writeAged(t, dir, "mcharbor-agent-backup-fresh.tar", 2*time.Minute, now)
	unmatched := writeAged(t, dir, "other-old.tar", time.Hour, now)
	bare := writeAged(t, dir, "mcharbor-agent-offline.db", time.Hour, now)
	subdir := filepath.Join(dir, "mcharbor-agent-backup-dir")
	if err := os.Mkdir(subdir, 0o750); err != nil {
		t.Fatal(err)
	}
	inner := writeAged(t, subdir, "mcharbor-agent-backup-inner.tar", time.Hour, now)
	oldDir := time.Hour
	if err := os.Chtimes(subdir, now.Add(-oldDir), now.Add(-oldDir)); err != nil {
		t.Fatal(err)
	}
	keep := []string{fresh, unmatched, bare, subdir, inner}
	if runtime.GOOS != "windows" {
		target := writeAged(t, dir, "target.dat", time.Hour, now)
		link := filepath.Join(dir, "mcharbor-agent-backup-link.tar")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		keep = append(keep, link, target)
	}
	res := sweepOrphanTemps(dir, now, slog.Default())
	for _, p := range keep {
		if !exists(p) {
			t.Errorf("file wrongly removed: %s", p)
		}
	}
	if res.Removed != 0 {
		t.Errorf("removed = %d, want 0", res.Removed)
	}
}

func TestSweepOrphanTempsMissingDir(t *testing.T) {
	res := sweepOrphanTemps(filepath.Join(t.TempDir(), "nope"), time.Now(), slog.Default())
	if res.Removed != 0 {
		t.Errorf("removed = %d", res.Removed)
	}
}
