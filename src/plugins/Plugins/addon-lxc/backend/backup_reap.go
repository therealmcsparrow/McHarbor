// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"time"
)

// reapOrphanedBackupRuns fails every run left mid-flight by a restart.
//
// A run's progress lives in a goroutine, so a plugin reload, a container
// recreate or a crash leaves the row saying "running" with nothing behind it.
// Without this it says that forever, and the Backups page shows a backup that
// will never finish beside ones that did.
func (s *Service) reapOrphanedBackupRuns(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339)
	rows, err := s.db.Query(ctx, `
		SELECT id, container_id, operation, kind
		  FROM plugin_addon_lxc_backup_runs
		 WHERE status = 'running' LIMIT ?`, maxRows)
	if err != nil {
		return fmt.Errorf("finding interrupted backup runs: %w", err)
	}
	type orphan struct{ id, containerID, operation, kind string }
	var orphans []orphan
	for rows.Next() {
		var item orphan
		if err := rows.Scan(&item.id, &item.containerID, &item.operation, &item.kind); err != nil {
			_ = rows.Close()
			return fmt.Errorf("finding interrupted backup runs: %w", err)
		}
		orphans = append(orphans, item)
	}
	_ = rows.Close()
	if len(orphans) == 0 {
		return nil
	}

	const reason = "Interrupted: McHarbor restarted while this was running, so how far it got is unknown."
	for _, item := range orphans {
		if _, err := s.db.Exec(ctx, `
			UPDATE plugin_addon_lxc_backup_runs
			   SET status = 'failure', error = ?, progress_stage = 'failed',
			       completed_at = ?, updated_at = ?
			 WHERE id = ? AND status = 'running'`, reason, now, now, item.id); err != nil {
			s.logger.Error("system containers: reaping an interrupted backup run",
				"run", item.id, "error", err)
			continue
		}
		// An interrupted backup may have advanced the host's incremental listing
		// without producing a usable artifact, and a restore killed partway
		// leaves a rootfs whose contents no longer match the listing at all. In
		// both cases the chain can no longer be built on, so the next run starts
		// a fresh full copy.
		if err := s.InvalidateChain(ctx, item.containerID,
			"a backup or restore was interrupted by a restart, so what the incremental "+
				"listing covers is unknown"); err != nil {
			s.logger.Error("system containers: invalidating a chain after an interrupted backup",
				"container", item.containerID, "error", err)
		}
		s.logBackupEvent(ctx, item.id, "interrupted", "failed", "error", reason)
	}
	s.logger.Warn("system containers: marked interrupted backup runs as failed",
		"count", len(orphans))
	return nil
}
