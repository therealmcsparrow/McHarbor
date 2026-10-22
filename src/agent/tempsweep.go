// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"log/slog"
	"os"
	"strings"
	"time"
)

// orphanTempMaxAge keeps the sweep away from files a second agent instance may still be writing.
const orphanTempMaxAge = 10 * time.Minute

// orphanTempPrefixes is the closed list of names this agent gives its own temp archives; anything else in the temp dir is not ours.
var orphanTempPrefixes = []string{
	"mcharbor-agent-backup-",
	"mcharbor-agent-restore-",
	"mcharbor-agent-frozen-",
	"mcharbor-agent-image-load-",
	"mcharbor-agent-direct-image-",
	"mcharbor-agent-direct-archive-",
	".mcharbor-agent-image-",
}

// tempSweepResult reports what a sweep removed.
type tempSweepResult struct {
	Removed int
	Bytes   int64
}

// sweepOrphanTemps deletes stale archives a crashed agent left in dir, because on a full disk they are what keeps it full.
// Only regular files (never symlinks or directories) older than orphanTempMaxAge with a listed prefix are touched.
func sweepOrphanTemps(dir string, now time.Time, logger *slog.Logger) tempSweepResult {
	var res tempSweepResult
	entries, err := os.ReadDir(dir)
	if err != nil {
		logger.Warn("temp sweep skipped", "dir", dir, "error", err)
		return res
	}
	for _, e := range entries {
		name := e.Name()
		if !hasOrphanPrefix(name) {
			continue
		}
		path := dir + string(os.PathSeparator) + name
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if now.Sub(info.ModTime()) <= orphanTempMaxAge {
			continue
		}
		if err := os.Remove(path); err != nil {
			logger.Warn("orphaned temp file not removed", "path", path, "error", err)
			continue
		}
		res.Removed++
		res.Bytes += info.Size()
	}
	if res.Removed > 0 {
		logger.Info("removed orphaned temp files", "count", res.Removed, "bytes", res.Bytes)
	}
	return res
}

func hasOrphanPrefix(name string) bool {
	for _, p := range orphanTempPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}
