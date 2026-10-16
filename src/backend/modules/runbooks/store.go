// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package runbooks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/rs/xid"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// ErrNotFound is returned when a runbook, run or step does not exist, so the
// handler can answer 404 without reading sql.ErrNoRows itself.
var ErrNotFound = errors.New("runbooks: not found")

// Store is every SQL statement the runbook engine issues. Handlers never
// query; the service never writes SQL of its own.
type Store struct {
	DB  *sql.DB
	Now func() time.Time

	// Requires names the permissions a step is checked against, which for a plugin
	// step depends on the plugin's live descriptors; unset, only host steps resolve.
	Requires func(Step) []string
}

func (s *Store) requiredFor(step Step) []string {
	if s.Requires != nil {
		return s.Requires(step)
	}
	return hostRequiredFor(step)
}

// NewStore returns a Store over db with the real clock.
func NewStore(db *sql.DB) *Store {
	return &Store{DB: db, Now: time.Now}
}

func (s *Store) now() string {
	clock := time.Now
	if s.Now != nil {
		clock = s.Now
	}
	return clock().UTC().Format(time.RFC3339)
}

const runbookColumns = `id, name, description, attach_kind, attach_source_type, attach_condition_key,
	attach_alert_id, attach_subject, environment_id, enabled, created_by, created_at, updated_at`

// runbookSelect is runbookColumns plus the automation columns and the
// principal's username, for reads. Writes still name runbookColumns alone.
const runbookSelect = runbookColumns + `, automation_principal_id, automation_on_alert, automation_cron,
	automation_since, automation_next_run_at, automation_last_run_at, automation_last_error,
	automation_updated_by,
	COALESCE((SELECT u.username FROM users u WHERE u.id = runbooks.automation_principal_id), '')`

func scanRunbook(row interface{ Scan(...any) error }) (Runbook, error) {
	var rb Runbook
	var enabled, onAlert int
	a := &rb.Automation
	err := row.Scan(&rb.ID, &rb.Name, &rb.Description, &rb.AttachKind, &rb.AttachSourceType,
		&rb.AttachConditionKey, &rb.AttachAlertID, &rb.AttachSubject, &rb.EnvironmentID,
		&enabled, &rb.CreatedBy, &rb.CreatedAt, &rb.UpdatedAt,
		&a.PrincipalID, &onAlert, &a.Cron, &a.Since, &a.NextRunAt, &a.LastRunAt, &a.LastError,
		&a.UpdatedBy, &a.PrincipalName)
	rb.Enabled = enabled != 0
	a.OnAlert = onAlert != 0
	rb.Steps = []Step{}
	return rb, err
}

// List returns one page of runbooks, newest first, without their steps.
func (s *Store) List(ctx context.Context, envID string, page, perPage int) ([]Runbook, int64, error) {
	var total int64
	if err := s.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM runbooks WHERE ? = '' OR environment_id = '' OR environment_id = ?`,
		envID, envID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting runbooks: %w", err)
	}
	rows, err := s.DB.QueryContext(ctx,
		`SELECT `+runbookSelect+` FROM runbooks
		 WHERE ? = '' OR environment_id = '' OR environment_id = ?
		 ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		envID, envID, perPage, (page-1)*perPage)
	if err != nil {
		return nil, 0, fmt.Errorf("listing runbooks: %w", err)
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
			return nil, 0, fmt.Errorf("scanning runbook: %w", err)
		}
		out = append(out, rb)
	}
	return out, total, rows.Err()
}

// Get returns one runbook with its steps in position order.
func (s *Store) Get(ctx context.Context, id string) (*Runbook, error) {
	rb, err := scanRunbook(s.DB.QueryRowContext(ctx, `SELECT `+runbookSelect+` FROM runbooks WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("reading runbook: %w", err)
	}
	steps, err := s.Steps(ctx, id)
	if err != nil {
		return nil, err
	}
	rb.Steps = steps
	return &rb, nil
}

// Steps returns a runbook's steps in position order.
func (s *Store) Steps(ctx context.Context, runbookID string) ([]Step, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, runbook_id, position, kind, title, body, diagnostic_key, action_key, action_params, required
		 FROM runbook_steps WHERE runbook_id = ? ORDER BY position ASC LIMIT 1000`, runbookID)
	if err != nil {
		return nil, fmt.Errorf("reading runbook steps: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("closing rows failed", "error", cerr)
		}
	}()
	out := []Step{}
	for rows.Next() {
		var st Step
		var required int
		var params string
		if err := rows.Scan(&st.ID, &st.RunbookID, &st.Position, &st.Kind, &st.Title, &st.Body,
			&st.DiagnosticKey, &st.ActionKey, &params, &required); err != nil {
			return nil, fmt.Errorf("scanning runbook step: %w", err)
		}
		st.Required = required != 0
		st.ActionParams = decodeParams(params)
		st.RequiredPermissions = s.requiredFor(st)
		out = append(out, st)
	}
	return out, rows.Err()
}

func decodeParams(raw string) map[string]string {
	out := map[string]string{}
	if raw == "" {
		return out
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]string{}
	}
	return out
}

func encodeParams(params map[string]string) string {
	if len(params) == 0 {
		return "{}"
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

// Save writes a runbook and replaces its steps in one transaction, assigning
// positions from the order the input gives. An empty id creates.
func (s *Store) Save(ctx context.Context, id string, in RunbookInput, createdBy string) (string, error) {
	now := s.now()
	err := inTx(ctx, s.DB, func(tx *sql.Tx) error {
		if id == "" {
			id = xid.New().String()
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO runbooks (`+runbookColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				id, in.Name, in.Description, in.AttachKind, in.AttachSourceType, in.AttachConditionKey,
				in.AttachAlertID, in.AttachSubject, in.EnvironmentID, boolInt(in.Enabled), createdBy, now, now); err != nil {
				return fmt.Errorf("inserting runbook: %w", err)
			}
		} else {
			res, err := tx.ExecContext(ctx,
				`UPDATE runbooks SET name = ?, description = ?, attach_kind = ?, attach_source_type = ?,
				 attach_condition_key = ?, attach_alert_id = ?, attach_subject = ?, environment_id = ?,
				 enabled = ?, updated_at = ? WHERE id = ?`,
				in.Name, in.Description, in.AttachKind, in.AttachSourceType, in.AttachConditionKey,
				in.AttachAlertID, in.AttachSubject, in.EnvironmentID, boolInt(in.Enabled), now, id)
			if err != nil {
				return fmt.Errorf("updating runbook: %w", err)
			}
			if coredb.RowsAffected(res) == 0 {
				return ErrNotFound
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM runbook_steps WHERE runbook_id = ?`, id); err != nil {
				return fmt.Errorf("clearing runbook steps: %w", err)
			}
		}
		for i, st := range in.Steps {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO runbook_steps (id, runbook_id, position, kind, title, body, diagnostic_key,
				 action_key, action_params, required, created_at, updated_at)
				 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
				xid.New().String(), id, i+1, st.Kind, st.Title, st.Body, st.DiagnosticKey,
				st.ActionKey, encodeParams(st.ActionParams), boolInt(st.Required), now, now); err != nil {
				return fmt.Errorf("inserting runbook step: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

// Delete removes a runbook. Its runs go with it, which is deliberate: a run
// whose steps no longer exist cannot be read, and the reporting source reads
// finished runs from a live runbook.
func (s *Store) Delete(ctx context.Context, id string) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM runbooks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting runbook: %w", err)
	}
	if coredb.RowsAffected(res) == 0 {
		return ErrNotFound
	}
	return nil
}

// Resolve returns the enabled runbooks that apply to one alert event or
// subject, condition-attached first because that is the durable attachment.
func (s *Store) Resolve(ctx context.Context, q ResolveQuery) ([]Runbook, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT `+runbookSelect+` FROM runbooks WHERE enabled = 1
		   AND (environment_id = '' OR ? = '' OR environment_id = ?)
		   AND ((attach_kind = ? AND attach_source_type = ? AND attach_condition_key = ? AND ? <> '')
		     OR (attach_kind = ? AND attach_alert_id = ? AND ? <> '')
		     OR (attach_kind = ? AND attach_subject = ? AND ? <> '')
		     OR (attach_kind = ? AND attach_subject = ? AND ? <> '')
		     OR (attach_kind = ? AND environment_id = ? AND ? <> '')
		     OR (attach_kind = ? AND attach_subject = ? AND ? <> '' AND ? <> ''))
		 ORDER BY CASE attach_kind WHEN ? THEN 0 WHEN ? THEN 1 WHEN ? THEN 2 WHEN ? THEN 3 ELSE 4 END,
		          created_at ASC LIMIT 1000`,
		q.EnvironmentID, q.EnvironmentID,
		AttachAlertCondition, q.SourceType, q.ConditionKey, q.ConditionKey,
		AttachAlertCondition, q.AlertID, q.AlertID,
		AttachContainer, q.ContainerName, q.ContainerName,
		AttachStack, q.StackName, q.StackName,
		AttachEnvironment, q.EnvironmentID, q.EnvironmentID,
		q.Kind, q.Subject, q.Kind, q.Subject,
		AttachAlertCondition, AttachContainer, AttachStack, AttachEnvironment)
	if err != nil {
		return nil, fmt.Errorf("resolving runbooks: %w", err)
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
			return nil, fmt.Errorf("scanning resolved runbook: %w", err)
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

// ResolveQuery is what an alert row or a container page knows about itself.
type ResolveQuery struct {
	EnvironmentID string
	SourceType    string
	ConditionKey  string
	AlertID       string
	ContainerName string
	StackName     string

	// Kind and Subject answer for any attachable object — a VM, a system
	// container, a swarm service, a Kubernetes workload — by its attach kind
	// and the id it was attached by.
	Kind    string
	Subject string
}

const runColumns = `id, runbook_id, runbook_name, environment_id, subject_kind, subject_id, subject_name,
	incident_id, alert_event_id, started_by, started_by_name, started_at, finished_at, outcome, note,
	trigger_kind`

func scanRun(row interface{ Scan(...any) error }) (Run, error) {
	var r Run
	err := row.Scan(&r.ID, &r.RunbookID, &r.RunbookName, &r.EnvironmentID, &r.SubjectKind, &r.SubjectID,
		&r.SubjectName, &r.IncidentID, &r.AlertEventID, &r.StartedBy, &r.StartedByName, &r.StartedAt,
		&r.FinishedAt, &r.Outcome, &r.Note, &r.Trigger)
	r.Results = []StepResult{}
	return r, err
}

// StartRun records a new run. startedBy is the operator the handler resolved
// from the request, or the service principal the engine acts as; nothing about
// it comes from a body.
func (s *Store) StartRun(ctx context.Context, in StartInput, runbookName, startedBy, startedByName, trigger string) (*Run, error) {
	id := xid.New().String()
	now := s.now()
	if _, err := s.DB.ExecContext(ctx,
		`INSERT INTO runbook_runs (`+runColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, in.RunbookID, runbookName, in.EnvironmentID, in.SubjectKind, in.SubjectID, in.SubjectName,
		in.IncidentID, in.AlertEventID, startedBy, startedByName, now, "", OutcomeInProgress, "", trigger); err != nil {
		return nil, fmt.Errorf("inserting runbook run: %w", err)
	}
	return s.Run(ctx, id)
}

// Run returns one run with its step results in the order they were recorded.
func (s *Store) Run(ctx context.Context, id string) (*Run, error) {
	run, err := scanRun(s.DB.QueryRowContext(ctx, `SELECT `+runColumns+` FROM runbook_runs WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("reading runbook run: %w", err)
	}
	results, err := s.Results(ctx, id)
	if err != nil {
		return nil, err
	}
	run.Results = results
	return &run, nil
}

// Results returns a run's recorded step results.
func (s *Store) Results(ctx context.Context, runID string) ([]StepResult, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, run_id, step_id, step_position, step_title, step_kind, state, skip_reason,
		 output, finding, acted_by, acted_by_name, at, last_error
		 FROM runbook_step_results WHERE run_id = ? ORDER BY step_position ASC, at ASC LIMIT 1000`, runID)
	if err != nil {
		return nil, fmt.Errorf("reading runbook step results: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("closing rows failed", "error", cerr)
		}
	}()
	out := []StepResult{}
	for rows.Next() {
		var sr StepResult
		if err := rows.Scan(&sr.ID, &sr.RunID, &sr.StepID, &sr.StepPosition, &sr.StepTitle, &sr.StepKind,
			&sr.State, &sr.SkipReason, &sr.Output, &sr.Finding, &sr.ActedBy, &sr.ActedByName,
			&sr.At, &sr.LastError); err != nil {
			return nil, fmt.Errorf("scanning runbook step result: %w", err)
		}
		out = append(out, sr)
	}
	return out, rows.Err()
}

// RecordResult appends one step result. A result is never updated in place:
// "I tried this, it failed, I skipped it" is three facts, not one row rewritten.
func (s *Store) RecordResult(ctx context.Context, sr StepResult) (*StepResult, error) {
	sr.ID = xid.New().String()
	sr.At = s.now()
	if _, err := s.DB.ExecContext(ctx,
		`INSERT INTO runbook_step_results (id, run_id, step_id, step_position, step_title, step_kind,
		 state, skip_reason, output, finding, acted_by, acted_by_name, at, last_error)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		sr.ID, sr.RunID, sr.StepID, sr.StepPosition, sr.StepTitle, sr.StepKind, sr.State,
		sr.SkipReason, sr.Output, sr.Finding, sr.ActedBy, sr.ActedByName, sr.At, sr.LastError); err != nil {
		return nil, fmt.Errorf("inserting runbook step result: %w", err)
	}
	return &sr, nil
}

// FinishRun closes a run with an outcome. A run already finished is refused, so
// a stale tab cannot relabel someone else's abandoned run as completed.
func (s *Store) FinishRun(ctx context.Context, id, outcome, note string) error {
	res, err := s.DB.ExecContext(ctx,
		`UPDATE runbook_runs SET outcome = ?, note = ?, finished_at = ? WHERE id = ? AND outcome = ?`,
		outcome, note, s.now(), id, OutcomeInProgress)
	if err != nil {
		return fmt.Errorf("finishing runbook run: %w", err)
	}
	if coredb.RowsAffected(res) == 0 {
		return ErrNotFound
	}
	return nil
}

// OpenRuns returns the in-progress runs for a subject, so whoever opens the
// alert next is offered the run already under way instead of a second one.
func (s *Store) OpenRuns(ctx context.Context, q ResolveQuery, alertEventID string) ([]Run, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT `+runColumns+` FROM runbook_runs
		 WHERE outcome = ? AND ((? <> '' AND alert_event_id = ?) OR (? <> '' AND subject_name = ?)
		   OR (? <> '' AND ? <> '' AND subject_kind = ? AND (subject_id = ? OR subject_name = ?)))
		 ORDER BY started_at DESC LIMIT 1000`,
		OutcomeInProgress, alertEventID, alertEventID, q.ContainerName, q.ContainerName,
		q.Kind, q.Subject, q.Kind, q.Subject, q.Subject)
	if err != nil {
		return nil, fmt.Errorf("reading open runbook runs: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("closing rows failed", "error", cerr)
		}
	}()
	return collectRuns(rows)
}

// RunFilter narrows a run listing. Every field is optional; StartedBy and
// Trigger are what "what did this service principal do overnight" reads.
type RunFilter struct {
	EnvironmentID string
	IncidentID    string
	StartedBy     string
	Trigger       string
}

// ListRuns returns one page of runs, newest first, narrowed by f.
func (s *Store) ListRuns(ctx context.Context, f RunFilter, page, perPage int) ([]Run, int64, error) {
	where := `WHERE (? = '' OR environment_id = ?) AND (? = '' OR incident_id = ?)
		AND (? = '' OR started_by = ?) AND (? = '' OR trigger_kind = ?)`
	args := []any{f.EnvironmentID, f.EnvironmentID, f.IncidentID, f.IncidentID,
		f.StartedBy, f.StartedBy, f.Trigger, f.Trigger}
	var total int64
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM runbook_runs `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting runbook runs: %w", err)
	}
	rows, err := s.DB.QueryContext(ctx,
		`SELECT `+runColumns+` FROM runbook_runs `+where+` ORDER BY started_at DESC LIMIT ? OFFSET ?`,
		append(args, perPage, (page-1)*perPage)...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing runbook runs: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("closing rows failed", "error", cerr)
		}
	}()
	runs, err := collectRuns(rows)
	if err != nil {
		return nil, 0, err
	}
	return runs, total, nil
}

func collectRuns(rows *sql.Rows) ([]Run, error) {
	out := []Run{}
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning runbook run: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// The environment lookups behind rbac.PinEnvToResource. A runbook whose
// environment_id is ” is shared by every environment, so it is reported as not
// pinned and the caller's own environment decides; a run always has one.
const (
	runbookEnvQuery    = `SELECT COALESCE(environment_id, '') FROM runbooks WHERE id = ?`
	runbookRunEnvQuery = `SELECT COALESCE(environment_id, '') FROM runbook_runs WHERE id = ?`
)

// RunbookEnvironment reports the environment a runbook is pinned to; a shared
// runbook reports found false so the request's own scope applies.
func (s *Store) RunbookEnvironment(ctx context.Context, id string) (string, bool, error) {
	envID, found, err := rbac.EnvOfRow(ctx, s.DB, runbookEnvQuery, id)
	if err != nil || envID == "" {
		return "", false, err
	}
	return envID, found, nil
}

// RunEnvironment reports the environment a runbook run executed in.
func (s *Store) RunEnvironment(ctx context.Context, id string) (string, bool, error) {
	return rbac.EnvOfRow(ctx, s.DB, runbookRunEnvQuery, id)
}

// inTx is core/db.Tx carrying the caller's ctx, so a cancelled request abandons
// the transaction instead of letting it run on a background context.
func inTx(ctx context.Context, database *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("rollback failed: %w (original: %w)", rbErr, err)
		}
		return err
	}
	return tx.Commit()
}
