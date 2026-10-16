// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package runbooks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

// principalKindService mirrors rbac.PrincipalKindService in SQL, so the store
// can only ever resolve a service principal and never a person.
const principalKindService = "service"

// SetAutomation stores a runbook's unattended configuration. since restarts the
// alert window, so turning automation on never replays alerts that fired before.
func (s *Store) SetAutomation(ctx context.Context, id string, in AutomationInput, nextRunAt, updatedBy string) error {
	since := ""
	if in.PrincipalID != "" {
		since = s.now()
	}
	res, err := s.DB.ExecContext(ctx,
		`UPDATE runbooks SET automation_principal_id = ?, automation_on_alert = ?, automation_cron = ?,
		 automation_since = ?, automation_next_run_at = ?, automation_last_error = '',
		 automation_updated_by = ?, updated_at = ? WHERE id = ?`,
		in.PrincipalID, boolInt(in.OnAlert), in.Cron, since, nextRunAt, updatedBy, s.now(), id)
	if err != nil {
		return fmt.Errorf("saving runbook automation: %w", err)
	}
	if coredb.RowsAffected(res) == 0 {
		return ErrNotFound
	}
	return nil
}

// ServicePrincipal returns one service principal. A person's id reads as not
// found, which is what keeps automation from ever acting as a human.
func (s *Store) ServicePrincipal(ctx context.Context, id string) (*ServicePrincipal, error) {
	var p ServicePrincipal
	var active bool
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, username, COALESCE(display_name, ''), is_active FROM users
		 WHERE id = ? AND principal_kind = ?`, id, principalKindService).Scan(&p.ID, &p.Username, &p.DisplayName, &active)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("reading service principal: %w", err)
	}
	p.Active = active
	return &p, nil
}

// ServicePrincipals lists every service principal for the automation picker.
func (s *Store) ServicePrincipals(ctx context.Context) ([]ServicePrincipal, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, username, COALESCE(display_name, ''), is_active FROM users
		 WHERE principal_kind = ? ORDER BY username ASC LIMIT 1000`, principalKindService)
	if err != nil {
		return nil, fmt.Errorf("listing service principals: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("closing rows failed", "error", cerr)
		}
	}()
	out := []ServicePrincipal{}
	for rows.Next() {
		var p ServicePrincipal
		var active bool
		if err := rows.Scan(&p.ID, &p.Username, &p.DisplayName, &active); err != nil {
			return nil, fmt.Errorf("scanning service principal: %w", err)
		}
		p.Active = active
		out = append(out, p)
	}
	return out, rows.Err()
}

// DueScheduled returns the automated runbooks whose schedule has come, with
// their steps. The engine advances next_run_at before running one.
func (s *Store) DueScheduled(ctx context.Context, now string) ([]Runbook, error) {
	return s.automated(ctx,
		`automation_cron <> '' AND automation_next_run_at <> '' AND automation_next_run_at <= ?`, now)
}

// AlertAutomated returns the automated runbooks that run on their alert. Only
// an alert-condition attachment can, because nothing else names an alert.
func (s *Store) AlertAutomated(ctx context.Context) ([]Runbook, error) {
	return s.automated(ctx, `automation_on_alert = 1 AND attach_kind = ?`, AttachAlertCondition)
}

func (s *Store) automated(ctx context.Context, cond string, args ...any) ([]Runbook, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT `+runbookSelect+` FROM runbooks`+ // SQL identifier: runbookSelect is a constant and cond is a fixed clause supplied by in-package callers
			` WHERE enabled = 1 AND automation_principal_id <> '' AND `+cond+`
		 ORDER BY created_at ASC LIMIT 100`, args...)
	if err != nil {
		return nil, fmt.Errorf("reading automated runbooks: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("closing rows failed", "error", cerr)
		}
	}()
	out := []Runbook{}
	for rows.Next() {
		rb, err := scanRunbook(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning automated runbook: %w", err)
		}
		out = append(out, rb)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		steps, err := s.Steps(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Steps = steps
	}
	return out, nil
}

// AdvanceSchedule moves a runbook's next run forward before the run starts, so
// a slow run can never be started twice by the next tick.
func (s *Store) AdvanceSchedule(ctx context.Context, id, next string) error {
	if _, err := s.DB.ExecContext(ctx,
		`UPDATE runbooks SET automation_next_run_at = ? WHERE id = ?`, next, id); err != nil {
		return fmt.Errorf("advancing runbook schedule: %w", err)
	}
	return nil
}

// RecordAutomationOutcome stamps the last unattended run and why it stopped, if
// it did not finish; the editor shows that message behind StatusError.
func (s *Store) RecordAutomationOutcome(ctx context.Context, id, lastError string) error {
	if _, err := s.DB.ExecContext(ctx,
		`UPDATE runbooks SET automation_last_run_at = ?, automation_last_error = ? WHERE id = ?`,
		s.now(), lastError, id); err != nil {
		return fmt.Errorf("recording runbook automation outcome: %w", err)
	}
	return nil
}

// firingAlert is the part of an alert event an unattended run is started from.
type firingAlert struct {
	ID            string
	SourceType    string
	EnvironmentID string
	SubjectID     string
	SubjectName   string
}

// FiringAlertsFor returns the firing alert events since since that rb applies
// to and has no run for yet. The NOT EXISTS is the dedupe: it survives a
// restart and a leader change, because it reads the runs themselves.
func (s *Store) FiringAlertsFor(ctx context.Context, rb Runbook, since string) ([]firingAlert, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, source_type, COALESCE(environment_id, ''), COALESCE(subject_id, ''), COALESCE(subject_name, '')
		 FROM alert_events
		 WHERE state = 'firing' AND created_at >= ?
		   AND ((? <> '' AND source_type = ? AND condition_key = ?) OR (? <> '' AND alert_id = ?))
		   AND (? = '' OR COALESCE(environment_id, '') = ?)
		   AND NOT EXISTS (SELECT 1 FROM runbook_runs rr WHERE rr.runbook_id = ? AND rr.alert_event_id = alert_events.id)
		 ORDER BY created_at ASC LIMIT 50`,
		since,
		rb.AttachConditionKey, rb.AttachSourceType, rb.AttachConditionKey, rb.AttachAlertID, rb.AttachAlertID,
		rb.EnvironmentID, rb.EnvironmentID, rb.ID)
	if err != nil {
		return nil, fmt.Errorf("reading firing alerts for a runbook: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("closing rows failed", "error", cerr)
		}
	}()
	var out []firingAlert
	for rows.Next() {
		var a firingAlert
		if err := rows.Scan(&a.ID, &a.SourceType, &a.EnvironmentID, &a.SubjectID, &a.SubjectName); err != nil {
			return nil, fmt.Errorf("scanning firing alert: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
