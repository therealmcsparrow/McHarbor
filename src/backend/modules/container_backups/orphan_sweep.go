// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	orphanAgentBackupPrefix = ".mcharbor-agent-backup-"
	orphanAgentBackupMaxAge = 10 * time.Minute
)

// SweepOrphanAgentBackups removes stale .mcharbor-agent-backup-* temp files that an interrupted agent transfer left in dir
// or in its run subfolders (uploads land in <dir>/<runID>/). Only regular files older than ten minutes go, and symlinked folders are never entered; it returns the count and bytes freed.
func SweepOrphanAgentBackups(dir string, now time.Time) (int, int64) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("agent backup temp sweep skipped", "dir", dir, "error", err)
		}
		return 0, 0
	}
	removed, freed := sweepOrphanFiles(dir, entries, now)
	for _, e := range entries {
		// DirEntry.Type comes from lstat, so a symlink to a folder is not IsDir and is skipped.
		if !e.Type().IsDir() {
			continue
		}
		sub := filepath.Join(dir, e.Name())
		subEntries, err := os.ReadDir(sub)
		if err != nil {
			slog.Warn("agent backup run folder not swept", "dir", sub, "error", err)
			continue
		}
		n, b := sweepOrphanFiles(sub, subEntries, now)
		removed += n
		freed += b
	}
	if removed > 0 {
		slog.Info("removed orphaned agent backup temp files", "dir", dir, "count", removed, "bytes", freed)
	}
	return removed, freed
}

// sweepOrphanFiles removes the stale prefixed regular files among entries, all of which live in dir.
func sweepOrphanFiles(dir string, entries []os.DirEntry, now time.Time) (int, int64) {
	var removed int
	var freed int64
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), orphanAgentBackupPrefix) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || now.Sub(info.ModTime()) <= orphanAgentBackupMaxAge {
			continue
		}
		if err := os.Remove(path); err != nil {
			slog.Warn("orphaned agent backup temp file not removed", "path", path, "error", err)
			continue
		}
		removed++
		freed += info.Size()
	}
	return removed, freed
}
