// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/alerting"
	"github.com/therealmcsparrow/mcharbor/core/plugin/types"
)

// Platform condition keys.
const (
	conditionBackupRunFailed         = "backup_run_failed"
	conditionBackupRunSucceeded      = "backup_run_succeeded"
	conditionRestoreRunFailed        = "restore_run_failed"
	conditionRestoreRunSucceeded     = "restore_run_succeeded"
	conditionBackupNoSuccessWithin   = "backup_no_success_within"
	conditionBackupDestinationFailed = "backup_destination_failed"
	conditionScanFindings            = "scan_findings"
	conditionWorkflowRunFailed       = "workflow_run_failed"
	conditionUpdateFailed            = "update_failed"
	conditionFailedLogins            = "failed_logins"
	conditionDesiredStateDrift       = "desired_state_drift"
	conditionInstanceBackupOverdue   = "instance_backup_no_success_within"
	conditionInstanceBackupFailed    = "instance_backup_failed"
	conditionReconcilerBreakerOpen   = "reconciler_breaker_open"
	conditionAPIKeyRotationDue       = "api_key_rotation_due" //nolint:gosec // G101: an alert condition key, not a credential
	conditionAPIKeyExpiring          = "api_key_expiring"     //nolint:gosec // G101: an alert condition key, not a credential
)

// platformPollInterval is the shared cadence for the platform source.
// These are database reads about things that change on the order of
// minutes — a backup run, a scan, a workflow — so a 30-second tick
// would be pure noise.
const platformPollInterval = 5 * time.Minute

// platformSourceSpec declares what the built-in platform source can
// alert on.
//
// Every condition here is host-wide (TargetNone): the subject is a
// backup plan, a workflow, an image — not a container in one
// environment — so the engine evaluates it once per tick rather than
// once per environment.
func platformSourceSpec() alerting.AlertSourceAdapterSpec {
	return alerting.AlertSourceAdapterSpec{
		SourceType:  sourcePlatform,
		DisplayName: "Platform",
		Description: "Backups, scans, workflows, updates and sign-in activity.",
		Conditions: []alerting.AlertConditionDecl{
			{
				Key:          conditionBackupRunFailed,
				DisplayName:  "Backup failed",
				Description:  "The most recent backup run for a plan failed. Resolves when a later run succeeds.",
				Kind:         alerting.KindEvent,
				TargetKind:   alerting.TargetNone,
				PollInterval: platformPollInterval,
			},
			{
				Key:          conditionBackupRunSucceeded,
				DisplayName:  "Backup succeeded",
				Description:  "The most recent backup run for a plan succeeded. Resolves when a later run does not.",
				Kind:         alerting.KindEvent,
				TargetKind:   alerting.TargetNone,
				PollInterval: platformPollInterval,
			},
			{
				Key:          conditionRestoreRunFailed,
				DisplayName:  "Restore failed",
				Description:  "The most recent restore for a container failed. Resolves when a later restore succeeds.",
				Kind:         alerting.KindEvent,
				TargetKind:   alerting.TargetNone,
				PollInterval: platformPollInterval,
			},
			{
				Key:          conditionRestoreRunSucceeded,
				DisplayName:  "Restore succeeded",
				Description:  "The most recent restore for a container succeeded. Resolves when a later restore does not.",
				Kind:         alerting.KindEvent,
				TargetKind:   alerting.TargetNone,
				PollInterval: platformPollInterval,
			},
			{
				Key:          conditionBackupNoSuccessWithin,
				DisplayName:  "Backup overdue",
				Description:  "No successful backup for a plan within the given period.",
				Kind:         alerting.KindDuration,
				TargetKind:   alerting.TargetNone,
				PollInterval: platformPollInterval,
			},
			{
				Key:          conditionBackupDestinationFailed,
				DisplayName:  "Backup destination failed",
				Description:  "An upload to a storage destination failed on its most recent attempt.",
				Kind:         alerting.KindEvent,
				TargetKind:   alerting.TargetNone,
				PollInterval: platformPollInterval,
			},
			{
				Key:              conditionScanFindings,
				DisplayName:      "Vulnerabilities found",
				Description:      "A completed image scan reported findings at or above a severity.",
				Kind:             alerting.KindThreshold,
				Unit:             "findings",
				Operators:        []string{">", ">="},
				DefaultOperator:  ">=",
				DefaultThreshold: 1,
				TargetKind:       alerting.TargetNone,
				PollInterval:     platformPollInterval,
				Fields: []types.ConfigFieldDecl{
					{
						Key:         "severity",
						Type:        "string",
						Label:       "Minimum severity",
						Default:     "high",
						Help:        "One of critical, high, medium or low. Findings below it are not counted.",
						Placeholder: "high",
					},
				},
			},
			{
				Key:          conditionWorkflowRunFailed,
				DisplayName:  "Workflow failed",
				Description:  "The most recent run of a workflow failed. Resolves when a later run succeeds.",
				Kind:         alerting.KindEvent,
				TargetKind:   alerting.TargetNone,
				PollInterval: platformPollInterval,
			},
			{
				Key:          conditionUpdateFailed,
				DisplayName:  "Auto-update failed",
				Description:  "An update policy's most recent run failed or was rolled back.",
				Kind:         alerting.KindEvent,
				TargetKind:   alerting.TargetNone,
				PollInterval: platformPollInterval,
			},
			{
				Key:          conditionDesiredStateDrift,
				DisplayName:  "Desired state drifted",
				Description:  "The reconciler has confirmed drift on a desired state: a container, a stack or an undeclared container in a watched environment. Resolves when the drift clears.",
				Kind:         alerting.KindEvent,
				TargetKind:   alerting.TargetNone,
				PollInterval: time.Minute,
			},
			{
				Key:          conditionInstanceBackupOverdue,
				DisplayName:  "Instance export overdue",
				Description:  "A scheduled instance export has not completed within the given period. Losing the host without a recent export loses the governance.",
				Kind:         alerting.KindDuration,
				TargetKind:   alerting.TargetNone,
				PollInterval: platformPollInterval,
			},
			{
				Key:          conditionInstanceBackupFailed,
				DisplayName:  "Instance export failed",
				Description:  "The last run of an instance export schedule failed: the export, the copy to its storage location, or its restore rehearsal. Resolves when a later run succeeds.",
				Kind:         alerting.KindEvent,
				TargetKind:   alerting.TargetNone,
				PollInterval: platformPollInterval,
			},
			{
				Key:          conditionReconcilerBreakerOpen,
				DisplayName:  "Drift enforcement halted",
				Description:  "Enforcement stopped after repeated failures and waits for someone to reset it. Resolves when it is reset.",
				Kind:         alerting.KindEvent,
				TargetKind:   alerting.TargetNone,
				PollInterval: platformPollInterval,
			},
			{
				Key:              conditionAPIKeyRotationDue,
				DisplayName:      "API key due for rotation",
				Description:      "An active API key is older than the threshold, in days. Resolves when it is rotated or revoked.",
				Kind:             alerting.KindThreshold,
				Unit:             "days",
				Operators:        []string{">", ">="},
				DefaultOperator:  ">=",
				DefaultThreshold: 90,
				TargetKind:       alerting.TargetNone,
				PollInterval:     platformPollInterval,
			},
			{
				Key:              conditionAPIKeyExpiring,
				DisplayName:      "API key expiring",
				Description:      "An active API key expires within the threshold, in days. Resolves when it is rotated, extended or revoked.",
				Kind:             alerting.KindThreshold,
				Unit:             "days",
				Operators:        []string{"<", "<="},
				DefaultOperator:  "<=",
				DefaultThreshold: 14,
				TargetKind:       alerting.TargetNone,
				PollInterval:     platformPollInterval,
			},
			{
				Key:              conditionFailedLogins,
				DisplayName:      "Repeated failed sign-ins",
				Description:      "Failed sign-in attempts for one account within the window.",
				Kind:             alerting.KindCountInWindow,
				Unit:             "attempts",
				Operators:        []string{">", ">="},
				DefaultOperator:  ">=",
				DefaultThreshold: 5,
				TargetKind:       alerting.TargetNone,
				PollInterval:     platformPollInterval,
			},
		},
	}
}

// platformSource evaluates the built-in platform conditions.
//
// Every query here reads a table another module owns. The source is
// strictly read-only against them: it answers "what does the record
// say?" and never writes back.
type platformSource struct {
	db *sql.DB
}

func newPlatformSource(deps EngineDeps) *platformSource {
	return &platformSource{db: deps.DB}
}

// Evaluate implements alerting.AlertSourceAdapterClient.
func (s *platformSource) Evaluate(ctx context.Context, req alerting.EvaluateRequest) ([]alerting.AlertObservation, error) {
	if s.db == nil {
		return nil, fmt.Errorf("platform source: database unavailable")
	}

	switch req.ConditionKey {
	case conditionBackupRunFailed:
		return s.backupRunStatus(ctx, "failure")
	case conditionBackupRunSucceeded:
		return s.backupRunStatus(ctx, "success")
	case conditionRestoreRunFailed:
		return s.restoreRunStatus(ctx, "failure")
	case conditionRestoreRunSucceeded:
		return s.restoreRunStatus(ctx, "success")
	case conditionBackupNoSuccessWithin:
		return s.backupOverdue(ctx)
	case conditionBackupDestinationFailed:
		return s.backupDestinationFailed(ctx)
	case conditionScanFindings:
		return s.scanFindings(ctx, req.Params["severity"])
	case conditionWorkflowRunFailed:
		return s.workflowRunFailed(ctx)
	case conditionUpdateFailed:
		return s.updateFailed(ctx)
	case conditionFailedLogins:
		return s.failedLogins(ctx, req.WindowSeconds)
	case conditionDesiredStateDrift:
		return s.desiredStateDrift(ctx)
	case conditionInstanceBackupOverdue:
		return s.instanceBackupOverdue(ctx)
	case conditionInstanceBackupFailed:
		return s.instanceBackupFailed(ctx)
	case conditionReconcilerBreakerOpen:
		return s.reconcilerBreakerOpen(ctx)
	case conditionAPIKeyRotationDue:
		return s.apiKeyAges(ctx)
	case conditionAPIKeyExpiring:
		return s.apiKeysExpiring(ctx)
	default:
		return nil, fmt.Errorf("platform source: unknown condition %q", req.ConditionKey)
	}
}

// backupRunFailed reports the latest run per plan.
//
// The subject is the plan, not the run: keying on the run id would
// make every failure a brand-new subject that never recovers, and the
// operator would get a fresh alert for a plan that has been broken for
// a week. Keyed on the plan, a failure fires once and the next
// successful run resolves it.
// backupRunStatus reports the latest backup run per plan, active when
// that run ended in activeStatus. One query serves both the "failed"
// and the "succeeded" condition: they differ only in which outcome is
// worth waking someone for, and a plan that alternates between them
// resolves one rule as it fires the other.
func (s *platformSource) backupRunStatus(ctx context.Context, activeStatus string) ([]alerting.AlertObservation, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.name, r.status, COALESCE(r.container_name, '')
		  FROM container_backup_plans p
		  JOIN container_backup_runs r
		    ON r.id = (SELECT id FROM container_backup_runs
		                WHERE plan_id = p.id AND operation = 'backup'
		                ORDER BY started_at DESC LIMIT 1)
		 LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing latest backup runs: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()
	out := make([]alerting.AlertObservation, 0, 32)
	for rows.Next() {
		var planID, planName, status, containerName string
		if err := rows.Scan(&planID, &planName, &status, &containerName); err != nil {
			return nil, fmt.Errorf("scanning backup run: %w", err)
		}

		// The run's own error text is deliberately not put in
		// Context: it reaches an operator's chat channel verbatim,
		// and a failed upload's error can carry a signed URL.
		out = append(out, alerting.AlertObservation{
			SubjectID:   planID,
			SubjectName: planName,
			Active:      status == activeStatus,
			ValueLabel:  status,
			Context:     map[string]string{"container": containerName, "status": status},
		})
	}

	return out, rows.Err()
}

// restoreRunStatus is the restore-side twin of backupRunStatus. It keys
// on the container rather than the plan: a restore is usually run ad hoc
// from the container's Backups tab and carries no plan at all, so a
// plan join would silently drop most of them.
func (s *platformSource) restoreRunStatus(ctx context.Context, activeStatus string) ([]alerting.AlertObservation, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.container_id, COALESCE(r.container_name, ''), r.status
		  FROM container_backup_runs r
		 WHERE r.operation = 'restore'
		   AND r.id = (SELECT id FROM container_backup_runs
		                WHERE container_id = r.container_id AND operation = 'restore'
		                ORDER BY started_at DESC LIMIT 1)
		 LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing latest restore runs: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()
	out := make([]alerting.AlertObservation, 0, 32)
	for rows.Next() {
		var containerID, containerName, status string
		if err := rows.Scan(&containerID, &containerName, &status); err != nil {
			return nil, fmt.Errorf("scanning restore run: %w", err)
		}

		name := containerName
		if name == "" {
			name = containerID
		}
		out = append(out, alerting.AlertObservation{
			SubjectID:   containerID,
			SubjectName: name,
			Active:      status == activeStatus,
			ValueLabel:  status,
			Context:     map[string]string{"container": name, "status": status},
		})
	}

	return out, rows.Err()
}

// backupOverdue reports how long it has been since each enabled plan
// last succeeded.
//
// Active is always true and Since carries the last success, because
// the host times the duration: "overdue by more than 24h" is then the
// operator's threshold rather than something baked into the query, and
// a fresh successful run moves Since forward and resolves the alert on
// the next tick.
func (s *platformSource) backupOverdue(ctx context.Context) ([]alerting.AlertObservation, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id,
		       p.name,
		       COALESCE((SELECT MAX(completed_at)
		                   FROM container_backup_runs
		                  WHERE plan_id = p.id
		                    AND operation = 'backup'
		                    AND status = 'success'), ''),
		       p.created_at
		  FROM container_backup_plans p
		 WHERE p.enabled = 1
		 LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing backup plans: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()
	out := make([]alerting.AlertObservation, 0, 32)
	for rows.Next() {
		var planID, planName, lastSuccess, createdAt string
		if err := rows.Scan(&planID, &planName, &lastSuccess, &createdAt); err != nil {
			return nil, fmt.Errorf("scanning backup plan: %w", err)
		}

		// A plan that has never succeeded counts from its creation.
		// Counting from zero time would fire the instant the plan is
		// created, before its first scheduled run ever had a chance.
		reference := lastSuccess
		if reference == "" {
			reference = createdAt
		}

		observation := alerting.AlertObservation{
			SubjectID:   planID,
			SubjectName: planName,
			Active:      true,
			Context:     map[string]string{"last_success": lastSuccess},
		}
		if since, parseErr := parseDBTime(reference); parseErr == nil {
			observation.Since = since
			observation.ValueLabel = formatDuration(time.Since(since))
		}

		out = append(out, observation)
	}

	return out, rows.Err()
}

// backupDestinationFailed reports the latest upload per storage
// destination, so one broken NAS shows up once rather than once per
// backup run that touched it.
func (s *platformSource) backupDestinationFailed(ctx context.Context) ([]alerting.AlertObservation, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT d.storage_location_id, d.storage_location_name, d.status
		  FROM container_backup_run_destinations d
		  JOIN (SELECT storage_location_id, MAX(created_at) AS latest
		          FROM container_backup_run_destinations
		         WHERE storage_location_id IS NOT NULL
		         GROUP BY storage_location_id) newest
		    ON newest.storage_location_id = d.storage_location_id
		   AND newest.latest = d.created_at
		 LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing backup destinations: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()
	out := make([]alerting.AlertObservation, 0, 16)
	for rows.Next() {
		var locationID, locationName, status string
		if err := rows.Scan(&locationID, &locationName, &status); err != nil {
			return nil, fmt.Errorf("scanning backup destination: %w", err)
		}

		if strings.TrimSpace(locationName) == "" {
			locationName = locationID
		}

		out = append(out, alerting.AlertObservation{
			SubjectID:   locationID,
			SubjectName: locationName,
			Active:      status == "failure",
			ValueLabel:  status,
		})
	}

	return out, rows.Err()
}

// scanFindings counts vulnerabilities at or above a severity in each
// image's most recent completed scan.
func (s *platformSource) scanFindings(ctx context.Context, severity string) ([]alerting.AlertObservation, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.image_ref, s.critical_count, s.high_count, s.medium_count, s.low_count
		  FROM scans s
		  JOIN (SELECT image_ref, MAX(created_at) AS latest
		          FROM scans
		         WHERE status = 'completed'
		         GROUP BY image_ref) newest
		    ON newest.image_ref = s.image_ref
		   AND newest.latest = s.created_at
		 WHERE s.status = 'completed'
		 LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing scans: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()
	minimum := strings.ToLower(strings.TrimSpace(severity))
	if minimum == "" {
		minimum = "high"
	}

	out := make([]alerting.AlertObservation, 0, 32)
	for rows.Next() {
		var imageRef string
		var critical, high, medium, low int
		if err := rows.Scan(&imageRef, &critical, &high, &medium, &low); err != nil {
			return nil, fmt.Errorf("scanning scan row: %w", err)
		}

		count := critical
		switch minimum {
		case "low":
			count += high + medium + low
		case "medium":
			count += high + medium
		case "high":
			count += high
		}

		out = append(out, alerting.AlertObservation{
			SubjectID:   imageRef,
			SubjectName: imageRef,
			Value:       float64(count),
			ValueLabel:  strconv.Itoa(count) + " " + minimum + "+ findings",
			Context: map[string]string{
				"critical": strconv.Itoa(critical),
				"high":     strconv.Itoa(high),
				"medium":   strconv.Itoa(medium),
				"low":      strconv.Itoa(low),
			},
		})
	}

	return out, rows.Err()
}

// workflowRunFailed reports the latest run per workflow.
func (s *platformSource) workflowRunFailed(ctx context.Context) ([]alerting.AlertObservation, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.workflow_id, COALESCE(w.name, r.workflow_id), r.status
		  FROM workflow_runs r
		  JOIN (SELECT workflow_id, MAX(started_at) AS latest
		          FROM workflow_runs
		         GROUP BY workflow_id) newest
		    ON newest.workflow_id = r.workflow_id
		   AND newest.latest = r.started_at
		  LEFT JOIN workflows w ON w.id = r.workflow_id
		 LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing workflow runs: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()
	out := make([]alerting.AlertObservation, 0, 32)
	for rows.Next() {
		var workflowID, workflowName, status string
		if err := rows.Scan(&workflowID, &workflowName, &status); err != nil {
			return nil, fmt.Errorf("scanning workflow run: %w", err)
		}

		out = append(out, alerting.AlertObservation{
			SubjectID:   workflowID,
			SubjectName: workflowName,
			Active:      strings.EqualFold(status, "failed") || strings.EqualFold(status, "failure") || strings.EqualFold(status, "error"),
			ValueLabel:  status,
		})
	}

	return out, rows.Err()
}

// desiredStateDrift reports every desired state, active while the reconciler
// has confirmed drift on it. It reads what the loop recorded rather than asking
// Docker again, so an alert can never disagree with the Reconciler page; the
// loop has already debounced it and treated an unreachable host as unknown.
func (s *platformSource) desiredStateDrift(ctx context.Context) ([]alerting.AlertObservation, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, COALESCE(kind, 'container'), COALESCE(env_id, ''), drift_detected, COALESCE(last_report, '')
		  FROM desired_states
		 LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing desired states: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()
	out := make([]alerting.AlertObservation, 0, 16)
	for rows.Next() {
		var id, name, kind, envID, report string
		var drifted bool
		if err := rows.Scan(&id, &name, &kind, &envID, &drifted, &report); err != nil {
			return nil, fmt.Errorf("scanning desired state: %w", err)
		}
		classes, fields := driftSummary(report)
		label := "in sync"
		if drifted {
			label = classes
		}
		out = append(out, alerting.AlertObservation{
			SubjectID:   id,
			SubjectName: name,
			Active:      drifted,
			ValueLabel:  label,
			Context:     map[string]string{"kind": kind, "environment": envID, "classes": classes, "fields": fields},
		})
	}
	return out, rows.Err()
}

// instanceBackupOverdue reports every enabled export schedule with how long
// since its last completed export, or since it was created if it never has.
func (s *platformSource) instanceBackupOverdue(ctx context.Context) ([]alerting.AlertObservation, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT sc.id, sc.name,
		       COALESCE((SELECT MAX(completed_at) FROM instance_backup_exports
		                  WHERE schedule_id = sc.id AND status = 'completed'), ''),
		       sc.created_at
		  FROM instance_backup_schedules sc
		 WHERE sc.enabled = 1
		 LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing instance export schedules: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()
	out := make([]alerting.AlertObservation, 0, 4)
	for rows.Next() {
		var id, name, last, created string
		if err := rows.Scan(&id, &name, &last, &created); err != nil {
			return nil, fmt.Errorf("scanning instance export schedule: %w", err)
		}
		reference := last
		if reference == "" {
			reference = created
		}
		o := alerting.AlertObservation{SubjectID: id, SubjectName: name, Active: true, Context: map[string]string{"last_success": last}}
		if since, parseErr := parseDBTime(reference); parseErr == nil {
			o.Since = since
			o.ValueLabel = formatDuration(time.Since(since))
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// instanceBackupFailed reports every export schedule, active while its last
// run failed. The schedule is the subject, so a broken destination alerts
// once and resolves on the next good run instead of once per export.
func (s *platformSource) instanceBackupFailed(ctx context.Context) ([]alerting.AlertObservation, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, last_status, last_error
		  FROM instance_backup_schedules
		 LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing instance export schedules: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()
	out := make([]alerting.AlertObservation, 0, 4)
	for rows.Next() {
		var id, name, status, lastErr string
		if err := rows.Scan(&id, &name, &status, &lastErr); err != nil {
			return nil, fmt.Errorf("scanning instance export schedule: %w", err)
		}
		label := status
		if label == "" {
			label = "not run yet"
		}
		out = append(out, alerting.AlertObservation{
			SubjectID: id, SubjectName: name, Active: status == "failed", ValueLabel: label,
			Context: map[string]string{"error": lastErr},
		})
	}
	return out, rows.Err()
}

// reconcilerBreakerOpen reports the enforcement circuit breaker. It is one
// subject for the whole instance, active from the trip until a person resets it.
func (s *platformSource) reconcilerBreakerOpen(ctx context.Context) ([]alerting.AlertObservation, error) {
	var open int
	var reason string
	err := s.db.QueryRowContext(ctx,
		`SELECT breaker_open, breaker_reason FROM reconciler_settings WHERE id = 'global'`).Scan(&open, &reason)
	if errors.Is(err, sql.ErrNoRows) {
		return []alerting.AlertObservation{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading the enforcement breaker: %w", err)
	}
	label := "closed"
	if open != 0 {
		label = "open"
	}
	return []alerting.AlertObservation{{
		SubjectID: "reconciler-enforcement", SubjectName: "Drift enforcement", Active: open != 0, ValueLabel: label,
		Context: map[string]string{"error": reason},
	}}, nil
}

// driftSummary names the drift classes and the first few fields of a stored
// report, which is what a chat message has room for.
func driftSummary(raw string) (classes, fields string) {
	var report struct {
		Diffs []struct {
			Class string `json:"class"`
			Field string `json:"field"`
		} `json:"diffs"`
	}
	if raw == "" || json.Unmarshal([]byte(raw), &report) != nil {
		return "", ""
	}
	seen := map[string]bool{}
	var cls, fs []string
	for _, d := range report.Diffs {
		if !seen[d.Class] {
			seen[d.Class] = true
			cls = append(cls, d.Class)
		}
		if len(fs) < 5 {
			fs = append(fs, d.Field)
		}
	}
	if len(report.Diffs) > len(fs) {
		fs = append(fs, fmt.Sprintf("+%d more", len(report.Diffs)-len(fs)))
	}
	return strings.Join(cls, ", "), strings.Join(fs, ", ")
}

// updateFailed reports the latest auto-update outcome per policy.
func (s *platformSource) updateFailed(ctx context.Context) ([]alerting.AlertObservation, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT h.policy_id, COALESCE(p.name, h.policy_id), h.status, h.container
		  FROM update_history h
		  JOIN (SELECT policy_id, MAX(executed_at) AS latest
		          FROM update_history
		         GROUP BY policy_id) newest
		    ON newest.policy_id = h.policy_id
		   AND newest.latest = h.executed_at
		  LEFT JOIN update_policies p ON p.id = h.policy_id
		 LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing update history: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()
	out := make([]alerting.AlertObservation, 0, 16)
	for rows.Next() {
		var policyID, policyName, status, container string
		if err := rows.Scan(&policyID, &policyName, &status, &container); err != nil {
			return nil, fmt.Errorf("scanning update history: %w", err)
		}

		out = append(out, alerting.AlertObservation{
			SubjectID:   policyID,
			SubjectName: policyName,
			Active:      status == "failure" || status == "rolled_back",
			ValueLabel:  status,
			Context:     map[string]string{"container": container, "status": status},
		})
	}

	return out, rows.Err()
}

// failedLogins counts failed sign-ins per account inside the window.
//
// Grouping by account rather than counting globally is the difference
// between "someone is being targeted" and "it is Monday morning".
func (s *platformSource) failedLogins(ctx context.Context, windowSeconds int) ([]alerting.AlertObservation, error) {
	if windowSeconds <= 0 {
		windowSeconds = 900
	}

	cutoff := time.Now().UTC().Add(-time.Duration(windowSeconds) * time.Second).Format(time.RFC3339)

	// entity_name first, not username. `username` is the *authenticated
	// actor*, which is correctly NULL on a failed login — nobody
	// authenticated. The account someone tried to sign in as is written
	// to entity_name by modules/auth, and to entity_id when a failed
	// MFA challenge already knows which user it belongs to. Grouping on
	// username collapsed every attempt into one subject called
	// "unknown", which is precisely the distinction this condition
	// exists to make.
	rows, err := s.db.QueryContext(ctx, `
		SELECT COALESCE(NULLIF(TRIM(entity_name), ''),
		                NULLIF(TRIM(username), ''),
		                NULLIF(TRIM(entity_id), ''),
		                'unknown') AS account,
		       COUNT(*) AS attempts,
		       COALESCE(MAX(ip_address), '') AS last_ip
		  FROM audit_logs
		 WHERE action = 'login_failed'
		   AND timestamp >= ?
		 GROUP BY account
		 ORDER BY attempts DESC
		 LIMIT 1000`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("counting failed logins: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()
	out := make([]alerting.AlertObservation, 0, 16)
	for rows.Next() {
		var account, lastIP string
		var attempts int
		if err := rows.Scan(&account, &attempts, &lastIP); err != nil {
			return nil, fmt.Errorf("scanning failed login count: %w", err)
		}

		out = append(out, alerting.AlertObservation{
			SubjectID:   account,
			SubjectName: account,
			Value:       float64(attempts),
			ValueLabel:  strconv.Itoa(attempts) + " attempts",
			Context:     map[string]string{"ip": lastIP},
		})
	}

	return out, rows.Err()
}

// parseDBTimeOrZero returns the zero time for a stored timestamp that is empty or unrecognised.
func parseDBTimeOrZero(value string) time.Time {
	parsed, err := parseDBTime(value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

// parseDBTime reads the two timestamp shapes this database contains:
// Go's RFC3339 (what every module writes) and SQLite's CURRENT_TIMESTAMP
// (what a column default writes).
func parseDBTime(value string) (time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("empty timestamp")
	}

	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			return parsed.UTC(), nil
		}
	}

	return time.Time{}, fmt.Errorf("unrecognised timestamp %q", trimmed)
}
