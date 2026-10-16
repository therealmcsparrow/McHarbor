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
		SELECT id, vm_id, operation, kind FROM plugin_addon_kvm_backup_runs
		 WHERE status = 'running' LIMIT 1000`)
	if err != nil {
		return fmt.Errorf("finding interrupted backup runs: %w", err)
	}
	type orphan struct{ id, vmID, operation, kind string }
	var orphans []orphan
	for rows.Next() {
		var o orphan
		if err := rows.Scan(&o.id, &o.vmID, &o.operation, &o.kind); err != nil {
			_ = rows.Close()
			return fmt.Errorf("finding interrupted backup runs: %w", err)
		}
		orphans = append(orphans, o)
	}
	_ = rows.Close()
	if len(orphans) == 0 {
		return nil
	}

	const reason = "Interrupted: McHarbor restarted while this was running, so how far it got is unknown."
	for _, o := range orphans {
		if _, err := s.db.Exec(ctx, `
			UPDATE plugin_addon_kvm_backup_runs
			   SET status = 'failure', error = ?, progress_stage = 'failed',
			       completed_at = ?, updated_at = ?
			 WHERE id = ? AND status = 'running'`, reason, now, now, o.id); err != nil {
			s.logger.Error("kvm: reaping an interrupted backup run", "run", o.id, "error", err)
			continue
		}
		// An incremental killed mid-copy may have consumed its bitmap without
		// producing a usable artifact, so the chain it belonged to can no
		// longer be built on. The next run starts a new full copy.
		if o.operation == BackupOpBackup && o.kind == BackupKindIncremental {
			if err := s.InvalidateChainsForVM(ctx, o.vmID,
				"a backup was interrupted by a restart, so what the change log covers is unknown"); err != nil {
				s.logger.Error("kvm: invalidating chains after an interrupted backup",
					"vm", o.vmID, "error", err)
			}
		}
		s.logBackupEvent(ctx, o.id, "interrupted", "failed", "error", reason)
	}
	s.logger.Warn("kvm: marked interrupted backup runs as failed", "count", len(orphans))
	return nil
}
