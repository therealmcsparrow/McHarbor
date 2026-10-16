// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"time"
)

// pruneBackupRuns applies a plan's retention.
//
// Two rules, either of which may be off: keep the last N successful runs, and
// keep runs for D days. A run that survives either rule survives.
//
// One rule outranks both: a run that a later incremental still builds on is
// never pruned, however old. Deleting it would leave a chain whose base is
// gone — every backup after it becomes unrestorable, and nothing would report
// that until someone tried.
func (s *Service) pruneBackupRuns(ctx context.Context, plan BackupPlan) error {
	if plan.RetentionCount <= 0 && plan.RetentionDays <= 0 {
		return nil
	}

	runs, err := s.successfulRunsForPlan(ctx, plan.ID)
	if err != nil {
		return err
	}
	keep := s.baselineRuns(runs)

	var doomed []BackupRun
	for index, run := range runs {
		if keep[run.ID] {
			continue
		}
		if plan.RetentionCount > 0 && index >= plan.RetentionCount {
			doomed = append(doomed, run)
			continue
		}
		if plan.RetentionDays > 0 {
			cutoff := time.Now().UTC().AddDate(0, 0, -plan.RetentionDays)
			if run.StartedAt.Before(cutoff) {
				doomed = append(doomed, run)
			}
		}
	}

	for _, run := range doomed {
		if err := s.deleteBackupRun(ctx, run); err != nil {
			// One stubborn destination must not stop the rest of the prune.
			s.logger.Warn("kvm: pruning a backup", "run", run.ID, "error", err)
		}
	}
	return nil
}

// baselineRuns is the set of runs a live chain still depends on.
//
// For every chain, the most recent full copy and everything after it. Those
// are the links a restore of the newest backup needs, and losing any of them
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

// successfulRunsForPlan lists a plan's successful backups, newest first.
func (s *Service) successfulRunsForPlan(ctx context.Context, planID string) ([]BackupRun, error) {
	rows, err := s.db.Query(ctx, `SELECT `+backupRunColumns+`
		  FROM plugin_addon_kvm_backup_runs
		 WHERE plan_id = ? AND operation = 'backup' AND status = 'success'
		 ORDER BY started_at DESC LIMIT 1000`, planID)
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
// The remote objects go first. The container module deletes only its local
// staging directory and leaves the remote copies forever, which is how a
// retention policy quietly stops being one — the whole point of retention is
// that the bytes go away.
func (s *Service) deleteBackupRun(ctx context.Context, run BackupRun) error {
	destinations, err := s.runDestinations(ctx, run.ID)
	if err != nil {
		return err
	}
	streamer, err := s.streamHandle()
	if err != nil {
		// Said once, loudly. The row would go and the object would stay, and
		// an operator watching their destination fill up deserves to know why
		// rather than concluding retention is broken — so the record is kept
		// too, and the prune is refused rather than half-done.
		s.logger.Warn("kvm: this host cannot remove objects from a storage destination; "+
			"the backup is kept rather than losing track of its copies",
			"run", run.ID, "error", err)
		return err
	}

	for _, dest := range destinations {
		if dest.Status != DestinationSuccess || dest.RemotePath == "" {
			continue
		}
		if err := streamer.DeleteObject(ctx, dest.LocationID, dest.RemotePath); err != nil {
			// One stubborn destination does not keep the whole run: the
			// others are gone, and a row that outlived its objects is a
			// smaller problem than one that blocks every later prune.
			s.logger.Warn("kvm: removing a backup object",
				"run", run.ID, "path", dest.RemotePath, "error", err)
		}
	}

	if _, err := s.db.Exec(ctx,
		`DELETE FROM plugin_addon_kvm_backup_runs WHERE id = ?`, run.ID); err != nil {
		return fmt.Errorf("removing the backup record: %w", err)
	}
	return nil
}

// runDestinations lists where one run's artifacts went.
func (s *Service) runDestinations(ctx context.Context, runID string) ([]BackupRunDestination, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, run_id, storage_location_id, storage_location_name, disk_id,
		       status, remote_path, key_id, bytes_uploaded, error,
		       COALESCE(uploaded_at, '')
		  FROM plugin_addon_kvm_backup_run_destinations
		 WHERE run_id = ? ORDER BY created_at LIMIT 1000`, runID)
	if err != nil {
		return nil, fmt.Errorf("reading backup destinations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []BackupRunDestination{}
	for rows.Next() {
		dest, err := scanBackupRunDestination(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, dest)
	}
	return out, rows.Err()
}

// ListBackupRuns returns a machine's runs, newest first, with destinations.
func (s *Service) ListBackupRuns(ctx context.Context, vmID string, limit int) ([]BackupRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `SELECT `+backupRunColumns+`
		  FROM plugin_addon_kvm_backup_runs
		 WHERE vm_id = ? ORDER BY started_at DESC LIMIT ?`, vmID, limit)
	if err != nil {
		return nil, fmt.Errorf("listing backup runs: %w", err)
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing backup runs: %w", err)
	}
	for i := range out {
		if out[i].Destinations, err = s.runDestinations(ctx, out[i].ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// BackupRunByID reads one run with its destinations.
func (s *Service) BackupRunByID(ctx context.Context, id string) (BackupRun, error) {
	rows, err := s.db.Query(ctx, `SELECT `+backupRunColumns+`
		  FROM plugin_addon_kvm_backup_runs WHERE id = ?`, id)
	if err != nil {
		return BackupRun{}, fmt.Errorf("reading the backup run: %w", err)
	}
	found := rows.Next()
	var run BackupRun
	if found {
		run, err = scanBackupRun(rows)
	}
	_ = rows.Close()
	if err != nil {
		return BackupRun{}, err
	}
	if !found {
		return BackupRun{}, ErrNotFound
	}
	if run.Destinations, err = s.runDestinations(ctx, run.ID); err != nil {
		return BackupRun{}, err
	}
	return run, nil
}

// DeleteBackupRun removes one run on request.
func (s *Service) DeleteBackupRun(ctx context.Context, id string) error {
	run, err := s.BackupRunByID(ctx, id)
	if err != nil {
		return err
	}
	// Refused rather than cascading: deleting a full copy that later
	// incrementals build on makes every one of them unrestorable, and the
	// operator asking for this one deletion cannot see that from here.
	if dependents, err := s.dependentRuns(ctx, run); err != nil {
		return err
	} else if dependents > 0 {
		return fmt.Errorf("%w: %d later backup(s) build on this one",
			ErrChainIncomplete, dependents)
	}
	return s.deleteBackupRun(ctx, run)
}

// dependentRuns counts later incrementals that rest on this run.
func (s *Service) dependentRuns(ctx context.Context, run BackupRun) (int, error) {
	if run.Kind != BackupKindFull || run.ChainID == "" {
		return 0, nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT COUNT(*) FROM plugin_addon_kvm_backup_runs
		 WHERE chain_id = ? AND operation = 'backup' AND status = 'success'
		   AND kind = 'incremental' AND started_at > ?`,
		run.ChainID, run.StartedAt.Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("counting dependent backups: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		return 0, rows.Err()
	}
	var count int
	if err := rows.Scan(&count); err != nil {
		return 0, fmt.Errorf("counting dependent backups: %w", err)
	}
	return count, rows.Err()
}

// backupRunsBetween returns a machine's backups started inside [from, to),
// oldest first, with destinations, for the backup assurance report.
func (s *Service) backupRunsBetween(ctx context.Context, vmID string, from, to time.Time) ([]BackupRun, error) {
	rows, err := s.db.Query(ctx, `SELECT `+backupRunColumns+`
		  FROM plugin_addon_kvm_backup_runs
		 WHERE vm_id = ? AND operation = 'backup'
		   AND started_at >= ? AND started_at < ?
		 ORDER BY started_at LIMIT ?`,
		vmID, from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339), maxRows)
	if err != nil {
		return nil, fmt.Errorf("listing backup runs in a period: %w", err)
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing backup runs in a period: %w", err)
	}
	for i := range out {
		if out[i].Destinations, err = s.runDestinations(ctx, out[i].ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// lastGoodBackupAt is when a machine's newest successful backup started, or
// the zero time when it has none. Read without a period bound, because a
// report window that starts after the last good copy must still age it.
func (s *Service) lastGoodBackupAt(ctx context.Context, vmID string) (time.Time, error) {
	rows, err := s.db.Query(ctx, `
		SELECT started_at FROM plugin_addon_kvm_backup_runs
		 WHERE vm_id = ? AND operation = 'backup' AND status = 'success'
		 ORDER BY started_at DESC LIMIT 1`, vmID)
	if err != nil {
		return time.Time{}, fmt.Errorf("reading the last good backup: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		return time.Time{}, rows.Err()
	}
	var started string
	if err := rows.Scan(&started); err != nil {
		return time.Time{}, fmt.Errorf("reading the last good backup: %w", err)
	}
	return parseTime(started), rows.Err()
}
