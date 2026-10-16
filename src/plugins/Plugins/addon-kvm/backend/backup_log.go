// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// logBackupEvent writes one lifecycle event to the host's backup log.
//
// Best effort throughout: a backup that ran must not fail because its log
// line did not land, and a host that wired no backup log leaves the handle
// nil rather than erroring on every write.
func (s *Service) logBackupEvent(ctx context.Context, runID, action, phase, severity, message string) {
	if s == nil || s.backupLog == nil || runID == "" {
		return
	}

	// One query for the context the feed needs: a run alone carries ids, and
	// a log line reading "dapu5ud… failed" helps nobody.
	var envID, vmID, vmName, planID, planName, operation string
	rows, err := s.db.Query(ctx, `
		SELECT r.environment_id, r.vm_id, COALESCE(v.name, ''),
		       COALESCE(r.plan_id, ''), COALESCE(p.name, ''), r.operation
		  FROM plugin_addon_kvm_backup_runs r
		  LEFT JOIN plugin_addon_kvm_vms v ON v.id = r.vm_id
		  LEFT JOIN plugin_addon_kvm_backup_plans p ON p.id = r.plan_id
		 WHERE r.id = ? LIMIT 1`, runID)
	if err != nil {
		s.logger.Warn("kvm: backup log context lookup failed", "run", runID, "error", err)
		return
	}
	found := rows.Next()
	if found {
		if err := rows.Scan(&envID, &vmID, &vmName, &planID, &planName, &operation); err != nil {
			_ = rows.Close()
			s.logger.Warn("kvm: backup log context lookup failed", "run", runID, "error", err)
			return
		}
	}
	_ = rows.Close()
	if !found {
		return
	}

	if err := s.backupLog.Record(ctx, plugin.BackupLogEntry{
		EnvironmentID: envID,
		PlanID:        planID,
		PlanName:      planName,
		RunID:         runID,
		SubjectID:     vmID,
		SubjectName:   nameOrID(vmName, vmID),
		SubjectKind:   "vm",
		Action:        operation + "." + action,
		Phase:         phase,
		Severity:      severity,
		Message:       messageNoun(operation) + " " + message,
	}); err != nil {
		s.logger.Warn("kvm: backup log write failed", "run", runID, "error", err)
	}
}

// messageNoun names the operation a log line is about.
//
// finishBackupRun and failBackupRun serve both backups and restores, so the
// noun cannot be baked into the call site without one of the two lying.
func messageNoun(operation string) string {
	if operation == BackupOpRestore {
		return "Restore"
	}
	return "Backup"
}
