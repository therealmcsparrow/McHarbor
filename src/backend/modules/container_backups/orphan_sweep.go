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
	orphanBackupPrefix      = ".mcharbor-backup-"
	// tempSweepMaxAge is the one age limit every sweep applies; a longer-running upload keeps its file owned instead.
	tempSweepMaxAge = time.Hour
)

// isDestinationPartial reports whether name is a partial file McHarbor itself writes into a backup destination.
func isDestinationPartial(name string) bool {
	return strings.HasPrefix(name, orphanBackupPrefix) || strings.HasPrefix(name, orphanAgentBackupPrefix)
}

// sweepDestinationPartials removes stale McHarbor partial files from dir and from its run subfolders, one level down.
// Only regular files older than an hour that no upload owns go, and a symlinked folder is never entered.
func sweepDestinationPartials(dir string, now time.Time) (int, int64) {
	removed, freed, _ := sweepPartials(dir, now)
	return removed, freed
}

// sweepPartials is sweepDestinationPartials that also reports how many old files were kept because an upload owns them.
func sweepPartials(dir string, now time.Time) (removed int, freed int64, keptOwned int) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("backup destination not swept", "dir", dir, "error", err)
		}
		return 0, 0, 0
	}
	defer closeRoot(root, dir)
	removed, freed, keptOwned = sweepPartialsIn(root, dir, now)
	entries, err := os.ReadDir(dir)
	if err != nil {
		slog.Warn("backup destination run folders not listed", "dir", dir, "error", err)
		return removed, freed, keptOwned
	}
	for _, e := range entries {
		// DirEntry.Type comes from lstat, so a symlink to a folder is not IsDir and is skipped.
		if !e.Type().IsDir() {
			continue
		}
		sub, err := root.OpenRoot(e.Name())
		if err != nil {
			slog.Warn("backup run folder not swept", "dir", filepath.Join(dir, e.Name()), "error", err)
			continue
		}
		n, b, k := sweepPartialsIn(sub, filepath.Join(dir, e.Name()), now)
		closeRoot(sub, dir)
		removed += n
		freed += b
		keptOwned += k
	}
	return removed, freed, keptOwned
}

// sweepPartialsIn removes the stale partial regular files directly inside root, whose path on disk is dir.
func sweepPartialsIn(root *os.Root, dir string, now time.Time) (removed int, freed int64, keptOwned int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		slog.Warn("backup destination not listed", "dir", dir, "error", err)
		return 0, 0, 0
	}
	for _, e := range entries {
		name := e.Name()
		if !isDestinationPartial(name) {
			continue
		}
		info, err := root.Lstat(name)
		if err != nil || !info.Mode().IsRegular() || now.Sub(info.ModTime()) <= tempSweepMaxAge {
			continue
		}
		path := filepath.Join(dir, name)
		if tempPathOwned(path) {
			keptOwned++
			continue
		}
		if err := root.Remove(name); err != nil {
			slog.Warn("backup partial file not removed", "path", path, "error", err)
			continue
		}
		removed++
		freed += info.Size()
	}
	return removed, freed, keptOwned
}

func closeRoot(root *os.Root, dir string) {
	if err := root.Close(); err != nil {
		slog.Debug("closing backup folder handle", "dir", dir, "error", err)
	}
}
