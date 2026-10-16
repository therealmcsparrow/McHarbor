// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package backup_log

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/xid"
	"github.com/therealmcsparrow/mcharbor/core/pluginruntime"
)

// validSeverities is what the table's CHECK constraint accepts.
var validSeverities = map[string]bool{"info": true, "success": true, "warning": true, "error": true}

// RecordProviderEntry writes one backup provider's lifecycle event.
//
// The provider's plan and run ids go in ref_plan_id / ref_run_id: plan_id and
// run_id are foreign keys into the container tables, and a virtual machine's
// id in either would fail the constraint and lose the row.
func (s *Service) RecordProviderEntry(ctx context.Context, source string, entry pluginruntime.BackupLogRecord) error {
	severity := entry.Severity
	if !validSeverities[severity] {
		// An imprecise severity is better than a dropped log line, and the
		// CHECK constraint would drop it.
		severity = "info"
	}
	message := entry.Message
	if message == "" {
		message = entry.Action
	}
	subjectKind := entry.SubjectKind
	if subjectKind == "" {
		subjectKind = source
	}

	var envP, subjectP, subjectNameP, planNameP, refPlanP, refRunP any
	if entry.EnvironmentID != "" {
		envP = entry.EnvironmentID
	}
	if entry.SubjectID != "" {
		subjectP = entry.SubjectID
	}
	if entry.SubjectName != "" {
		subjectNameP = entry.SubjectName
	}
	if entry.PlanName != "" {
		planNameP = entry.PlanName
	}
	if entry.PlanID != "" {
		refPlanP = entry.PlanID
	}
	if entry.RunID != "" {
		refRunP = entry.RunID
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO container_backup_log
			(id, environment_id, plan_id, plan_name, run_id,
			 container_id, container_name, action, phase, severity,
			 message, source, subject_kind, ref_plan_id, ref_run_id,
			 created_at, updated_at)
		VALUES (?, ?, NULL, ?, NULL, ?, ?, ?, ?, ?, ?, 'backup', ?, ?, ?, ?, ?)`,
		xid.New().String(), envP, planNameP, subjectP, subjectNameP,
		entry.Action, entry.Phase, severity, message,
		subjectKind, refPlanP, refRunP, now, now,
	)
	if err != nil {
		return fmt.Errorf("inserting provider backup log: %w", err)
	}
	return nil
}
