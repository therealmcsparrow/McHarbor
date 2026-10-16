// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// pruneBackupRuns applies a plan's retention.
//
// Two rules, either of which may be off: keep the last N successful runs, and
// keep runs for D days. A run that survives either rule survives.
//
// Two rules outrank both. A run that a later incremental still builds on is
// never pruned, however old — deleting it would leave a chain whose base is
// gone, and nothing would report that until someone tried to restore. And the
// single most recent surviving backup of a container that still exists is never
// pruned either, because a retention policy that can leave a live container with
// no backup at all has stopped being a retention policy.
func (s *Service) pruneBackupRuns(ctx context.Context, plan BackupPlan) error {
	if plan.RetentionCount <= 0 && plan.RetentionDays <= 0 {
		return nil
	}

	runs, err := s.successfulRunsForPlan(ctx, plan.ID)
	if err != nil {
		return err
	}
	keep := s.baselineRuns(runs)

	// The newest run is never a candidate. Ordered newest first, so index 0 is
	// it — and it is the one an operator would reach for first.
	if len(runs) > 0 && s.containerExists(ctx, runs[0].ContainerID) {
		keep[runs[0].ID] = true
	}

	for _, run := range retentionCandidates(plan, runs, keep, time.Now().UTC()) {
		if err := s.deleteBackupRun(ctx, run); err != nil {
			// One stubborn destination must not stop the rest of the prune.
			s.logger.Warn("system containers: pruning a backup", "run", run.ID, "error", err)
		}
	}
	return nil
}

// retentionCandidates is the pure half of the prune: which runs the two rules
// give up on, given the set that is protected regardless.
//
// Extracted from pruneBackupRuns so the rules can be read against a fixed clock
// without a database behind them. runs arrive newest first, which is what makes
// the count rule an index comparison.
func retentionCandidates(
	plan BackupPlan,
	runs []BackupRun,
	keep map[string]bool,
	now time.Time,
) []BackupRun {
	var doomed []BackupRun
	cutoff := now.AddDate(0, 0, -plan.RetentionDays)
	for index, run := range runs {
		if keep[run.ID] {
			continue
		}
		if plan.RetentionCount > 0 && index >= plan.RetentionCount {
			doomed = append(doomed, run)
			continue
		}
		if plan.RetentionDays > 0 && run.StartedAt.Before(cutoff) {
			doomed = append(doomed, run)
		}
	}
	return doomed
}

// baselineRuns is the set of runs a live chain still depends on.
//
// For every chain, the most recent full copy and everything after it. Those are
// the links a restore of the newest backup needs, and losing any of them
// silently breaks every incremental above it.
func (s *Service) baselineRuns(runs []BackupRun) map[string]bool {
	keep := map[string]bool{}
	newestFull := map[string]time.Time{}

	for _, run := range runs {
		if run.Kind != BackupKindFull || run.ChainID == "" {
			continue
		}
		if existing, ok := newestFull[run.ChainID]; !ok || run.StartedAt.After(existing) {
			newestFull[run.ChainID] = run.StartedAt
		}
	}
	for _, run := range runs {
		if run.ChainID == "" {
			continue
		}
		full, ok := newestFull[run.ChainID]
		if ok && !run.StartedAt.Before(full) {
			keep[run.ID] = true
		}
	}
	return keep
}

// containerExists reports whether the subject of a backup is still here.
//
// A container that has been destroyed has nothing left to protect, so its last
// backup is an ordinary retention candidate. One that still exists must never be
// left without any backup at all.
func (s *Service) containerExists(ctx context.Context, containerID string) bool {
	if containerID == "" {
		return false
	}
	rows, err := s.db.Query(ctx,
		`SELECT 1 FROM plugin_addon_lxc_containers WHERE id = ? LIMIT 1`, containerID)
	if err != nil {
		// Unknown reads as "still here": the conservative direction is keeping
		// a backup that could have gone, not deleting one that should not have.
		return true
	}
	defer func() { _ = rows.Close() }()
	return rows.Next()
}

// successfulRunsForPlan lists a plan's successful backups, newest first.
func (s *Service) successfulRunsForPlan(ctx context.Context, planID string) ([]BackupRun, error) {
	rows, err := s.db.Query(ctx, `SELECT `+backupRunColumns+`
		  FROM plugin_addon_lxc_backup_runs
		 WHERE plan_id = ? AND operation = 'backup' AND status = 'success'
		 ORDER BY started_at DESC LIMIT ?`, planID, maxRows)
	if err != nil {
		return nil, fmt.Errorf("listing backups to prune: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []BackupRun
	for rows.Next() {
		run, err := scanBackupRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

// deleteBackupRun removes a run and the objects it wrote.
//
// The remote objects go first. A retention policy whose only effect is deleting
// rows has stopped being one — the whole point is that the bytes go away.
func (s *Service) deleteBackupRun(ctx context.Context, run BackupRun) error {
	destinations, err := s.runDestinations(ctx, run.ID)
	if err != nil {
		return err
	}
	streamer, err := s.streamHandle()
	if err != nil {
		// Said once, loudly. The row would go and the objects would stay, and an
		// operator watching their destination fill up deserves to know why
		// rather than concluding retention is broken — so the record is kept too
		// and the prune is refused rather than half-done.
		s.logger.Warn("system containers: this host cannot remove objects from a storage "+
			"destination; the backup is kept rather than losing track of its copies",
			"run", run.ID, "error", err)
		return err
	}

	for _, dest := range destinations {
		if dest.Status != DestinationSuccess || dest.RemotePath == "" {
			continue
		}
		if err := streamer.DeleteObject(ctx, dest.LocationID, dest.RemotePath); err != nil {
			// One stubborn destination does not keep the whole run: the others
			// are gone, and a row that outlived its objects is a smaller problem
			// than one that blocks every later prune.
			s.logger.Warn("system containers: removing a backup object",
				"run", run.ID, "path", dest.RemotePath, "error", err)
		}
	}
	s.removeEmptyBackupFolders(ctx, streamer, destinations)

	if _, err := s.db.Exec(ctx,
		`DELETE FROM plugin_addon_lxc_backup_runs WHERE id = ?`, run.ID); err != nil {
		return fmt.Errorf("removing the backup record: %w", err)
	}
	return nil
}

// rmdirLocationTypes are the storage types whose delete of a directory key is an
// rmdir that refuses a non-empty directory. Every other adapter is left alone: a
// WebDAV or cloud-drive DELETE of a folder removes everything inside it.
var rmdirLocationTypes = map[string]bool{"local": true, "nfs": true, "samba": true, "sftp": true}

// backupFolderDepth is how many folders backupObjectKey puts above an artifact,
// <env>/ct/<name>/. Nothing above them is this plugin's to remove.
const backupFolderDepth = 3

// emptyFolderCandidates returns the folders a deleted artifact at remotePath may
// have left empty, deepest first, or nil when the path is not the shape
// backupObjectKey writes, so a stored path from elsewhere never climbs the tree.
func emptyFolderCandidates(remotePath string) []string {
	for _, part := range strings.Split(remotePath, "/") {
		if part == ".." {
			return nil
		}
	}
	clean := path.Clean(remotePath)
	parts := strings.Split(clean, "/")
	if len(parts) < backupFolderDepth+1 || parts[len(parts)-3] != "ct" {
		return nil
	}
	for _, part := range parts[len(parts)-4:] {
		if part == "" || part == "." || part == ".." {
			return nil
		}
	}
	out := make([]string, 0, backupFolderDepth)
	for dir := path.Dir(clean); len(out) < backupFolderDepth; dir = path.Dir(dir) {
		out = append(out, dir)
	}
	return out
}

// removeEmptyBackupFolders removes the <env>/ct/<name>/ folders a deletion left
// empty, through the storage adapter and only where its delete is an rmdir. It
// is best-effort: a folder that still holds another run is the common case.
func (s *Service) removeEmptyBackupFolders(ctx context.Context, streamer plugin.StorageStreamHandle, destinations []BackupRunDestination) {
	types := map[string]string{}
	for _, location := range s.storage.Locations() {
		types[location.ID] = location.LocationType
	}
	tried := map[string]bool{}
	for _, dest := range destinations {
		if dest.Status != DestinationSuccess || dest.RemotePath == "" {
			continue
		}
		if !rmdirLocationTypes[types[dest.LocationID]] {
			s.logger.Debug("system containers: leaving backup folders on a destination whose delete is recursive",
				"location", dest.LocationID, "type", types[dest.LocationID])
			continue
		}
		for _, dir := range emptyFolderCandidates(dest.RemotePath) {
			key := dest.LocationID + "|" + dir
			if tried[key] {
				break
			}
			tried[key] = true
			if err := streamer.DeleteObject(ctx, dest.LocationID, dir); err != nil {
				// A folder that still holds something keeps every folder above it.
				s.logger.Debug("system containers: backup folder kept",
					"location", dest.LocationID, "path", dir, "error", err)
				break
			}
			s.logger.Info("system containers: empty backup folder removed, or already gone",
				"location", dest.LocationID, "path", dir)
		}
	}
}
