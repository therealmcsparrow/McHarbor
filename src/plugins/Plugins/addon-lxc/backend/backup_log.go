// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// logBackupEvent writes one lifecycle event to the host's backup log.
//
// Best effort throughout: a backup that ran must not fail because its log line
// did not land, and a host that wired no backup log leaves the handle nil rather
// than erroring on every write.
func (s *Service) logBackupEvent(ctx context.Context, runID, action, phase, severity, message string) {
	if s == nil || s.backupLog == nil || runID == "" {
		return
	}

	// One query for the context the feed needs: a run alone carries ids, and a
	// log line reading "dapu5ud… failed" helps nobody.
	var envID, ctID, ctName, planID, planName, operation string
	rows, err := s.db.Query(ctx, `
		SELECT r.environment_id, r.container_id, r.container_name,
		       COALESCE(r.plan_id, ''), COALESCE(p.name, ''), r.operation
		  FROM plugin_addon_lxc_backup_runs r
		  LEFT JOIN plugin_addon_lxc_backup_plans p ON p.id = r.plan_id
		 WHERE r.id = ? LIMIT 1`, runID)
	if err != nil {
		s.logger.Warn("system containers: backup log context lookup failed",
			"run", runID, "error", err)
		return
	}
	found := rows.Next()
	if found {
		if err := rows.Scan(&envID, &ctID, &ctName, &planID, &planName, &operation); err != nil {
			_ = rows.Close()
			s.logger.Warn("system containers: backup log context lookup failed",
				"run", runID, "error", err)
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
		SubjectID:     ctID,
		SubjectName:   nameOrID(ctName, ctID),
		SubjectKind:   "ct",
		Action:        operation + "." + action,
		Phase:         phase,
		Severity:      severity,
		Message:       backupMessageNoun(operation) + " " + message,
	}); err != nil {
		s.logger.Warn("system containers: backup log write failed", "run", runID, "error", err)
	}
}

// backupMessageNoun names the operation a log line is about.
//
// finishBackupRun and failBackupRun serve both backups and restores, so the noun
// cannot be baked into the call site without one of the two lying.
func backupMessageNoun(operation string) string {
	if operation == BackupOpRestore {
		return "Restore"
	}
	return "Backup"
}
