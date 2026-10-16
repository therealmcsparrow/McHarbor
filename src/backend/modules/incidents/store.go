// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package incidents

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// Store persists incidents, settings and findings.
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// NewStore returns a store over database.
func NewStore(database *sql.DB) *Store {
	return &Store{db: database, now: time.Now}
}

func (s *Store) stamp() string { return s.now().UTC().Format(time.RFC3339) }

type scanner interface {
	Scan(dest ...any) error
}

const incidentColumns = `id, env_id, subject_type, subject_name, stack_name, signal_kind, status, signal_count, first_signal_at,
	last_signal_at, alert_event_id, evidence, evidence_at, narrative, narrative_prompt, narrative_meta, narrative_status,
	narrative_error, narrated_at, timeline, summary, resolved_at, created_at, subject_id, source`

func nullable(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func decodeInto[T any](raw string, out **T) {
	if raw == "" {
		return
	}
	var v T
	if json.Unmarshal([]byte(raw), &v) == nil {
		*out = &v
	}
}

func scanIncident(row scanner, full bool) (Incident, error) {
	var in Incident
	var env, alert, evidenceAt, narratedAt, resolvedAt sql.NullString
	var evidence, narrative, prompt, meta, timeline string
	if err := row.Scan(&in.ID, &env, &in.SubjectType, &in.SubjectName, &in.StackName, &in.Signal, &in.Status,
		&in.SignalCount, &in.FirstSignalAt, &in.LastSignalAt, &alert, &evidence, &evidenceAt, &narrative, &prompt,
		&meta, &in.NarrativeStatus, &in.NarrativeError, &narratedAt, &timeline, &in.Summary, &resolvedAt, &in.CreatedAt,
		&in.SubjectID, &in.Source); err != nil {
		return in, err
	}
	in.EnvID, in.AlertEventID, in.EvidenceAt = env.String, alert.String, evidenceAt.String
	in.NarratedAt, in.ResolvedAt = narratedAt.String, resolvedAt.String
	decodeInto(narrative, &in.Narrative)
	decodeInto(meta, &in.NarrativeMeta)
	if full {
		decodeInto(evidence, &in.Evidence)
		in.NarrativePrompt = prompt
		if timeline != "" {
			if err := json.Unmarshal([]byte(timeline), &in.Timeline); err != nil {
				return in, fmt.Errorf("decoding timeline: %w", err)
			}
		}
	}
	return in, nil
}

// OpenFor returns the open incident for a container, or nil. Plugin incidents are
// excluded: a VM and a container may share a name, and each must keep its own incident.
func (s *Store) OpenFor(ctx context.Context, envID, subject string) (*Incident, error) {
	in, err := scanIncident(s.db.QueryRowContext(ctx,
		`SELECT `+incidentColumns+` FROM incidents WHERE COALESCE(env_id, '') = ? AND subject_name = ? AND source = '' AND status = ?
		 ORDER BY first_signal_at DESC LIMIT 1`, envID, subject, StatusOpen), true)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading open incident: %w", err)
	}
	return &in, nil
}

// OpenForSubject returns the open incident a plugin raised about one subject, keyed on
// the subject's stable id so a rename does not split one failure into two incidents.
func (s *Store) OpenForSubject(ctx context.Context, source, subjectType, envID, subjectID string) (*Incident, error) {
	in, err := scanIncident(s.db.QueryRowContext(ctx,
		`SELECT `+incidentColumns+` FROM incidents WHERE source = ? AND subject_type = ? AND COALESCE(env_id, '') = ?
		 AND subject_id = ? AND status = ? ORDER BY first_signal_at DESC LIMIT 1`,
		source, subjectType, envID, subjectID, StatusOpen), true)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading open plugin incident: %w", err)
	}
	return &in, nil
}

// Get returns one incident with its evidence and prompt, or nil.
func (s *Store) Get(ctx context.Context, id string) (*Incident, error) {
	in, err := scanIncident(s.db.QueryRowContext(ctx, `SELECT `+incidentColumns+` FROM incidents WHERE id = ?`, id), true)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading incident: %w", err)
	}
	return &in, nil
}

// ByAlertEvent returns the incident an alert event opened, or nil.
func (s *Store) ByAlertEvent(ctx context.Context, eventID string) (*Incident, error) {
	in, err := scanIncident(s.db.QueryRowContext(ctx,
		`SELECT `+incidentColumns+` FROM incidents WHERE alert_event_id = ? ORDER BY created_at DESC LIMIT 1`, eventID), false)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading incident by alert: %w", err)
	}
	return &in, nil
}

// List returns incidents newest first, without their evidence.
func (s *Store) List(ctx context.Context, envID, status string, page, perPage int) ([]Incident, int64, error) {
	where, args := " WHERE 1 = 1", []any{}
	if envID != "" {
		where += " AND env_id = ?"
		args = append(args, envID)
	}
	if status != "" {
		where += " AND status = ?"
		args = append(args, status)
	}
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM incidents`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting incidents: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+incidentColumns+` FROM incidents`+where+ // SQL identifier: incidentColumns is a constant and where holds fixed AND clauses, every value is bound
		` ORDER BY last_signal_at DESC, id DESC LIMIT ? OFFSET ?`, append(args, perPage, (page-1)*perPage)...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing incidents: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("closing rows failed", "error", cerr)
		}
	}()
	out := []Incident{}
	for rows.Next() {
		in, err := scanIncident(rows, false)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning incident: %w", err)
		}
		out = append(out, in)
	}
	return out, total, rows.Err()
}

// Open inserts a new incident.
func (s *Store) Open(ctx context.Context, in Incident) (*Incident, error) {
	in.ID = xid.New().String()
	now := s.stamp()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO incidents (id, env_id, subject_type, subject_name, stack_name, signal_kind, status, signal_count,
		 first_signal_at, last_signal_at, alert_event_id, created_at, updated_at, subject_id, source)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.ID, nullable(in.EnvID), in.SubjectType, in.SubjectName, in.StackName, in.Signal, StatusOpen, in.SignalCount,
		in.FirstSignalAt, in.LastSignalAt, nullable(in.AlertEventID), now, now, in.SubjectID, in.Source); err != nil {
		return nil, fmt.Errorf("opening incident: %w", err)
	}
	return s.Get(ctx, in.ID)
}

// Extend records more signals on an open incident. A stronger signal replaces
// a weaker one, so a crash loop is not described as a single exit.
func (s *Store) Extend(ctx context.Context, id, signal string, count int, at, alertEventID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE incidents SET signal_kind = ?, signal_count = signal_count + ?, last_signal_at = ?,
		 alert_event_id = COALESCE(alert_event_id, ?), updated_at = ? WHERE id = ?`,
		signal, count, at, nullable(alertEventID), s.stamp(), id)
	if err != nil {
		return fmt.Errorf("extending incident: %w", err)
	}
	return nil
}

// SaveEvidence stores the bundle an incident will be judged on.
func (s *Store) SaveEvidence(ctx context.Context, id string, ev *Evidence) error {
	raw, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("encoding evidence: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `UPDATE incidents SET evidence = ?, evidence_at = ?, updated_at = ? WHERE id = ?`,
		string(raw), s.stamp(), s.stamp(), id)
	if err != nil {
		return fmt.Errorf("saving evidence: %w", err)
	}
	return nil
}

// ClaimNarration moves an incident to pending unless a narration is already
// running, so a double click or a racing proactive pass narrates it once.
func (s *Store) ClaimNarration(ctx context.Context, id string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE incidents SET narrative_status = ?, narrative_error = '', updated_at = ? WHERE id = ? AND narrative_status <> ?`,
		NarrativePending, s.stamp(), id, NarrativePending)
	if err != nil {
		return false, fmt.Errorf("claiming narration: %w", err)
	}
	return db.RowsAffected(res) == 1, nil
}

// SaveNarrative records a finished narration with its attribution and prompt.
func (s *Store) SaveNarrative(ctx context.Context, id string, n *Narrative, meta ModelMeta, prompt string) error {
	rawN, err := json.Marshal(n)
	if err != nil {
		return fmt.Errorf("encoding narrative: %w", err)
	}
	rawM, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("encoding narrative meta: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE incidents SET narrative = ?, narrative_meta = ?, narrative_prompt = ?, narrative_status = ?,
		 narrative_error = '', narrated_at = ?, narrated_count = narrated_count + 1, updated_at = ? WHERE id = ?`,
		string(rawN), string(rawM), prompt, NarrativeReady, meta.At, s.stamp(), id)
	if err != nil {
		return fmt.Errorf("saving narrative: %w", err)
	}
	return nil
}

// FailNarration records why a narration did not happen.
func (s *Store) FailNarration(ctx context.Context, id, status, reason string) error {
	if len(reason) > 2000 {
		reason = reason[:2000]
	}
	_, err := s.db.ExecContext(ctx, `UPDATE incidents SET narrative_status = ?, narrative_error = ?, updated_at = ? WHERE id = ?`,
		status, reason, s.stamp(), id)
	if err != nil {
		return fmt.Errorf("recording narration failure: %w", err)
	}
	return nil
}

// NarrationsSince counts narrations started for an environment since a moment,
// which is what the hourly cap is measured against.
func (s *Store) NarrationsSince(ctx context.Context, envID string, since time.Time) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM incidents WHERE COALESCE(env_id, '') = ? AND narrated_at >= ?`,
		envID, since.UTC().Format(time.RFC3339)).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("counting narrations: %w", err)
	}
	return n, nil
}

// Resolve closes an incident with its timeline.
func (s *Store) Resolve(ctx context.Context, id string, timeline []TimelineItem) error {
	raw, err := json.Marshal(timeline)
	if err != nil {
		return fmt.Errorf("encoding timeline: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE incidents SET status = ?, timeline = ?, resolved_at = ?, updated_at = ? WHERE id = ? AND status = ?`,
		StatusResolved, string(raw), s.stamp(), s.stamp(), id, StatusOpen)
	if err != nil {
		return fmt.Errorf("resolving incident: %w", err)
	}
	return nil
}

// SaveSummary stores a model-written post-incident summary.
func (s *Store) SaveSummary(ctx context.Context, id, summary string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE incidents SET summary = ?, updated_at = ? WHERE id = ?`, summary, s.stamp(), id)
	if err != nil {
		return fmt.Errorf("saving summary: %w", err)
	}
	return nil
}

// Quiet returns open incidents whose last signal is older than cutoff.
func (s *Store) Quiet(ctx context.Context, cutoff time.Time) ([]Incident, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+incidentColumns+` FROM incidents WHERE status = ? AND last_signal_at < ? LIMIT 200`,
		StatusOpen, cutoff.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("listing quiet incidents: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("closing rows failed", "error", cerr)
		}
	}()
	out := []Incident{}
	for rows.Next() {
		in, err := scanIncident(rows, true)
		if err != nil {
			return nil, fmt.Errorf("scanning incident: %w", err)
		}
		out = append(out, in)
	}
	return out, rows.Err()
}

// Settings returns an environment's narration settings; the zero value means
// nothing is ever sent.
func (s *Store) Settings(ctx context.Context, envID string) (Settings, error) {
	st := Settings{EnvID: envID, HourlyCap: 10}
	var optIn, proactive int
	var at sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT narrate_opt_in, proactive, hourly_cap, opted_in_by, opted_in_at FROM incident_settings WHERE id = ?`, envID,
	).Scan(&optIn, &proactive, &st.HourlyCap, &st.OptedInBy, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return st, nil
	}
	if err != nil {
		return st, fmt.Errorf("reading incident settings: %w", err)
	}
	st.NarrateOptIn, st.Proactive, st.OptedInAt = optIn != 0, proactive != 0, at.String
	return st, nil
}

// SaveSettings writes an environment's settings, recording who opted it in.
func (s *Store) SaveSettings(ctx context.Context, st Settings, by string) (Settings, error) {
	prev, err := s.Settings(ctx, st.EnvID)
	if err != nil {
		return st, err
	}
	optedBy, optedAt := prev.OptedInBy, nullable(prev.OptedInAt)
	if st.NarrateOptIn && !prev.NarrateOptIn {
		optedBy, optedAt = by, s.stamp()
	}
	if !st.NarrateOptIn {
		optedBy, optedAt, st.Proactive = "", nil, false
	}
	b := func(v bool) int {
		if v {
			return 1
		}
		return 0
	}
	now := s.stamp()
	res, err := s.db.ExecContext(ctx,
		`UPDATE incident_settings SET narrate_opt_in = ?, proactive = ?, hourly_cap = ?, opted_in_by = ?, opted_in_at = ?,
		 updated_at = ? WHERE id = ?`, b(st.NarrateOptIn), b(st.Proactive), st.HourlyCap, optedBy, optedAt, now, st.EnvID)
	if err != nil {
		return st, fmt.Errorf("saving incident settings: %w", err)
	}
	if db.RowsAffected(res) == 0 {
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO incident_settings (id, env_id, narrate_opt_in, proactive, hourly_cap, opted_in_by, opted_in_at,
			 created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			st.EnvID, st.EnvID, b(st.NarrateOptIn), b(st.Proactive), st.HourlyCap, optedBy, optedAt, now, now); err != nil {
			return st, fmt.Errorf("creating incident settings: %w", err)
		}
	}
	return s.Settings(ctx, st.EnvID)
}

// The environment lookups behind rbac.PinEnvToResource: an incident, the incident
// raised for an alert, and a behaviour finding are authorized on their own env.
const (
	incidentEnvQuery      = `SELECT COALESCE(env_id, '') FROM incidents WHERE id = ?`
	incidentAlertEnvQuery = `SELECT COALESCE(env_id, '') FROM incidents WHERE alert_event_id = ? LIMIT 1`
	findingEnvQuery       = `SELECT COALESCE(env_id, '') FROM behaviour_findings WHERE id = ?`
)

// IncidentEnvironment reports the environment an incident belongs to.
func (s *Store) IncidentEnvironment(ctx context.Context, id string) (string, bool, error) {
	return rbac.EnvOfRow(ctx, s.db, incidentEnvQuery, id)
}

// AlertIncidentEnvironment reports the environment of the incident an alert raised.
func (s *Store) AlertIncidentEnvironment(ctx context.Context, eventID string) (string, bool, error) {
	return rbac.EnvOfRow(ctx, s.db, incidentAlertEnvQuery, eventID)
}

// FindingEnvironment reports the environment a behaviour finding belongs to.
func (s *Store) FindingEnvironment(ctx context.Context, id string) (string, bool, error) {
	return rbac.EnvOfRow(ctx, s.db, findingEnvQuery, id)
}

// memoryPoint is one rollup bucket of memory use, for a container (name set) or
// a host (name empty).
type memoryPoint struct {
	env, name, at string
	value         float64
}

// containerMemoryRollup reads container memory buckets newest-first, so a large
// fleet hitting limit drops its oldest buckets rather than the recent ones.
func (s *Store) containerMemoryRollup(ctx context.Context, since string, limit int) ([]memoryPoint, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT COALESCE(environment_id, ''), COALESCE(container_name, ''), bucket_start, mem_used_avg
		 FROM container_metrics_rollup WHERE bucket_start >= ? ORDER BY bucket_start DESC LIMIT ?`,
		since, limit)
	if err != nil {
		return nil, fmt.Errorf("reading container memory history: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("closing rows failed", "error", cerr)
		}
	}()
	var out []memoryPoint
	for rows.Next() {
		var p memoryPoint
		if err := rows.Scan(&p.env, &p.name, &p.at, &p.value); err != nil {
			return nil, fmt.Errorf("scanning container memory: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// hostMemoryRollup reads host memory-percent buckets newest-first, capped at limit.
func (s *Store) hostMemoryRollup(ctx context.Context, since string, limit int) ([]memoryPoint, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT environment_id, bucket_start, mem_percent_avg FROM host_metrics_rollup
		 WHERE bucket_start >= ? ORDER BY bucket_start DESC LIMIT ?`, since, limit)
	if err != nil {
		return nil, fmt.Errorf("reading host memory history: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("closing rows failed", "error", cerr)
		}
	}()
	var out []memoryPoint
	for rows.Next() {
		var p memoryPoint
		if err := rows.Scan(&p.env, &p.at, &p.value); err != nil {
			return nil, fmt.Errorf("scanning host memory: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// restartEvent is one container restart or start from the lifecycle log.
type restartEvent struct{ env, name, at string }

// restartEvents lists container restart and start events since the given time.
func (s *Store) restartEvents(ctx context.Context, since string) ([]restartEvent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT COALESCE(environment_id, ''), COALESCE(subject_name, ''), timestamp FROM lifecycle_events
		 WHERE subject_type = 'container' AND action IN ('restart', 'start') AND timestamp >= ?
		 ORDER BY timestamp LIMIT 100000`, since)
	if err != nil {
		return nil, fmt.Errorf("reading restarts: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("closing rows failed", "error", cerr)
		}
	}()
	var out []restartEvent
	for rows.Next() {
		var e restartEvent
		if err := rows.Scan(&e.env, &e.name, &e.at); err != nil {
			return nil, fmt.Errorf("scanning restart: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// promotionRow is a succeeded pipeline promotion and the environment it targeted.
type promotionRow struct{ env, pipeline, at string }

// succeededPromotions lists promotions that succeeded within [from, to].
func (s *Store) succeededPromotions(ctx context.Context, from, to string) ([]promotionRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT COALESCE(s.target_environment_id, ''), pl.name, COALESCE(p.finished_at, p.started_at)
		 FROM gitops_promotions p JOIN gitops_pipeline_stages s ON s.id = p.stage_id JOIN gitops_pipelines pl ON pl.id = p.pipeline_id
		 WHERE p.status = 'succeeded' AND p.started_at >= ? AND p.started_at <= ? LIMIT 500`, from, to)
	if err != nil {
		return nil, fmt.Errorf("reading promotions: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("closing rows failed", "error", cerr)
		}
	}()
	var out []promotionRow
	for rows.Next() {
		var p promotionRow
		if err := rows.Scan(&p.env, &p.pipeline, &p.at); err != nil {
			return nil, fmt.Errorf("scanning promotion: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// dieEventMetadata returns the metadata of container die events for env in [from, to).
func (s *Store) dieEventMetadata(ctx context.Context, env, from, to string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT COALESCE(metadata, '') FROM lifecycle_events WHERE subject_type = 'container' AND action = 'die'
		 AND COALESCE(environment_id, '') = ? AND timestamp >= ? AND timestamp < ? LIMIT 10000`, env, from, to)
	if err != nil {
		return nil, fmt.Errorf("counting failing exits: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("closing rows failed", "error", cerr)
		}
	}()
	var out []string
	for rows.Next() {
		var meta string
		if err := rows.Scan(&meta); err != nil {
			return nil, fmt.Errorf("scanning exit: %w", err)
		}
		out = append(out, meta)
	}
	return out, rows.Err()
}

// upsertFinding refreshes the open finding for a subject and kind, or opens a new
// one when there is none or the last was cleared, and returns its id.
func (s *Store) upsertFinding(ctx context.Context, env, subjectType, subject, kind, metrics, stamp string) (string, error) {
	var id, status string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, status FROM behaviour_findings WHERE COALESCE(env_id, '') = ? AND subject_name = ? AND kind = ?
		 ORDER BY last_seen_at DESC LIMIT 1`, env, subject, kind).Scan(&id, &status)
	switch {
	case errors.Is(err, sql.ErrNoRows), err == nil && status == "cleared":
		id = xid.New().String()
		_, err = s.db.ExecContext(ctx,
			`INSERT INTO behaviour_findings (id, env_id, subject_type, subject_name, kind, status, metrics, first_seen_at,
			 last_seen_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 'open', ?, ?, ?, ?, ?)`,
			id, nullable(env), subjectType, subject, kind, metrics, stamp, stamp, stamp, stamp)
	case err == nil:
		_, err = s.db.ExecContext(ctx,
			`UPDATE behaviour_findings SET metrics = ?, last_seen_at = ?, updated_at = ? WHERE id = ?`, metrics, stamp, stamp, id)
	}
	if err != nil {
		return "", fmt.Errorf("recording finding: %w", err)
	}
	return id, nil
}

// clearStaleFindings marks open findings not seen since cutoff as cleared.
func (s *Store) clearStaleFindings(ctx context.Context, now, cutoff string) error {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE behaviour_findings SET status = 'cleared', updated_at = ? WHERE status = 'open' AND last_seen_at < ?`,
		now, cutoff); err != nil {
		return fmt.Errorf("clearing stale findings: %w", err)
	}
	return nil
}

// Findings lists findings for an environment, open first, newest first.
func (s *Store) Findings(ctx context.Context, envID, status string) ([]Finding, error) {
	where, args := " WHERE 1 = 1", []any{}
	if envID != "" {
		where += " AND env_id = ?"
		args = append(args, envID)
	}
	if status != "" {
		where += " AND status = ?"
		args = append(args, status)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, COALESCE(env_id, ''), subject_type, subject_name, kind, status, metrics, first_seen_at, last_seen_at,`+ // SQL identifier: where is built only from fixed AND clauses, every value is bound
			` explanation, explanation_meta FROM behaviour_findings`+where+` ORDER BY last_seen_at DESC LIMIT 500`, args...)
	if err != nil {
		return nil, fmt.Errorf("listing findings: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("closing rows failed", "error", cerr)
		}
	}()
	out := []Finding{}
	for rows.Next() {
		f, err := scanFinding(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Status == "open" && out[j].Status != "open" })
	return out, rows.Err()
}

func scanFinding(row scanner) (Finding, error) {
	var f Finding
	var metrics, meta string
	if err := row.Scan(&f.ID, &f.EnvID, &f.SubjectType, &f.SubjectName, &f.Kind, &f.Status, &metrics, &f.FirstSeenAt,
		&f.LastSeenAt, &f.Explanation, &meta); err != nil {
		return f, fmt.Errorf("scanning finding: %w", err)
	}
	f.Metrics = map[string]float64{}
	if metrics != "" {
		if err := json.Unmarshal([]byte(metrics), &f.Metrics); err != nil {
			return f, fmt.Errorf("decoding finding metrics: %w", err)
		}
	}
	decodeInto(meta, &f.ExplanationMeta)
	return f, nil
}

// Finding returns one finding, or nil.
func (s *Store) Finding(ctx context.Context, id string) (*Finding, error) {
	f, err := scanFinding(s.db.QueryRowContext(ctx,
		`SELECT id, COALESCE(env_id, ''), subject_type, subject_name, kind, status, metrics, first_seen_at, last_seen_at,
		 explanation, explanation_meta FROM behaviour_findings WHERE id = ?`, id))
	if err != nil {
		if errors.Is(errors.Unwrap(err), sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &f, nil
}

// DismissFinding hides a finding until it is detected again after clearing.
func (s *Store) DismissFinding(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE behaviour_findings SET status = 'dismissed', updated_at = ? WHERE id = ?`, s.stamp(), id)
	if err != nil {
		return fmt.Errorf("dismissing finding: %w", err)
	}
	return nil
}

func (s *Store) saveExplanation(ctx context.Context, id, text string, meta ModelMeta) error {
	raw, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("encoding explanation meta: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `UPDATE behaviour_findings SET explanation = ?, explanation_meta = ?, updated_at = ? WHERE id = ?`,
		text, string(raw), s.stamp(), id)
	if err != nil {
		return fmt.Errorf("saving explanation: %w", err)
	}
	return nil
}

// failureEvent is one failure-shaped container event with the time it was stored.
type failureEvent struct{ env, name, action, at, meta, created string }

// failureEvents reads die, oom and unhealthy container events stored since createdSince
// and happening since eventSince.
func (s *Store) failureEvents(ctx context.Context, createdSince, eventSince string) ([]failureEvent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT COALESCE(environment_id, ''), COALESCE(subject_name, ''), action, timestamp, COALESCE(metadata, ''), created_at
		 FROM lifecycle_events
		 WHERE subject_type = 'container' AND created_at >= ? AND timestamp >= ?
		   AND (action IN ('die', 'oom') OR action LIKE 'health_status%unhealthy')
		 ORDER BY timestamp LIMIT 5000`, createdSince, eventSince)
	if err != nil {
		return nil, fmt.Errorf("reading failure events: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("closing rows failed", "error", cerr)
		}
	}()
	var out []failureEvent
	for rows.Next() {
		var e failureEvent
		if err := rows.Scan(&e.env, &e.name, &e.action, &e.at, &e.meta, &e.created); err != nil {
			return nil, fmt.Errorf("scanning failure event: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// firingContainerAlerts turns container alert events that started firing since the given time into detections.
func (s *Store) firingContainerAlerts(ctx context.Context, since string) ([]Detection, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, COALESCE(environment_id, ''), subject_name, created_at FROM alert_events
		 WHERE state = 'firing' AND source_type = 'container' AND COALESCE(subject_name, '') <> '' AND created_at >= ?
		 ORDER BY created_at LIMIT 200`, since)
	if err != nil {
		return nil, fmt.Errorf("reading firing alerts: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("closing rows failed", "error", cerr)
		}
	}()
	var out []Detection
	for rows.Next() {
		var det Detection
		if err := rows.Scan(&det.AlertEventID, &det.EnvID, &det.Subject, &det.At); err != nil {
			return nil, fmt.Errorf("scanning firing alert: %w", err)
		}
		det.Signal, det.Count = SignalAlert, 1
		out = append(out, det)
	}
	return out, rows.Err()
}

// lastContainerEvent returns the newest non-exec event for a container since the
// given time, and false when there is none.
func (s *Store) lastContainerEvent(ctx context.Context, subject, env, since string) (string, string, bool, error) {
	var action, meta string
	err := s.db.QueryRowContext(ctx,
		`SELECT action, COALESCE(metadata, '') FROM lifecycle_events
		 WHERE subject_type = 'container' AND subject_name = ? AND COALESCE(environment_id, '') = ?
		   AND action NOT LIKE 'exec_%' AND timestamp >= ?
		 ORDER BY timestamp DESC LIMIT 1`, subject, env, since).Scan(&action, &meta)
	if err == sql.ErrNoRows {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("reading last event: %w", err)
	}
	return action, meta, true, nil
}

// containerEvents reads the container's lifecycle events in [start, end], oldest first.
func (s *Store) containerEvents(ctx context.Context, subject, env, start, end string, limit int) ([]EventItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT timestamp, action, COALESCE(metadata, '') FROM lifecycle_events
		 WHERE subject_type = 'container' AND subject_name = ? AND COALESCE(environment_id, '') = ?
		   AND timestamp >= ? AND timestamp <= ? AND action NOT LIKE 'exec_%'
		 ORDER BY timestamp DESC LIMIT ?`, subject, env, start, end, limit)
	if err != nil {
		return nil, fmt.Errorf("reading container events: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("closing rows failed", "error", cerr)
		}
	}()
	var out []EventItem
	for rows.Next() {
		var at, action, meta string
		if err := rows.Scan(&at, &action, &meta); err != nil {
			return nil, fmt.Errorf("scanning container event: %w", err)
		}
		attrs := eventAttrs(meta)
		out = append(out, EventItem{At: at, Action: action, ExitCode: attrs["exitCode"], Image: attrs["image"]})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At < out[j].At })
	return out, rows.Err()
}

// containerCreation is one container create event and the image it used.
type containerCreation struct{ at, image string }

// containerCreations reads the container's create events in [start, end], oldest first.
func (s *Store) containerCreations(ctx context.Context, subject, env, start, end string) ([]containerCreation, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT timestamp, COALESCE(metadata, '') FROM lifecycle_events
		 WHERE subject_type = 'container' AND subject_name = ? AND COALESCE(environment_id, '') = ?
		   AND action = 'create' AND timestamp >= ? AND timestamp <= ? ORDER BY timestamp ASC LIMIT 50`,
		subject, env, start, end)
	if err != nil {
		return nil, fmt.Errorf("reading container creations: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("closing rows failed", "error", cerr)
		}
	}()
	var out []containerCreation
	for rows.Next() {
		var at, meta string
		if err := rows.Scan(&at, &meta); err != nil {
			return nil, fmt.Errorf("scanning container creation: %w", err)
		}
		out = append(out, containerCreation{at: at, image: eventAttrs(meta)["image"]})
	}
	return out, rows.Err()
}

// auditRows reads audit entries in [start, end] that name any of names, or that
// touched the environment, stack or desired state of env, oldest first.
func (s *Store) auditRows(ctx context.Context, names []any, env, start, end string, limit int) ([]AuditItem, error) {
	q := `SELECT timestamp, COALESCE(username, ''), action, COALESCE(entity_type, ''), COALESCE(entity_name, '')` + // SQL identifier: the repeated segment is only "?" markers, every value is bound
		` FROM audit_logs WHERE timestamp >= ? AND timestamp <= ?
		   AND (entity_name IN (?` + strings.Repeat(", ?", len(names)-1) + `)
		        OR (COALESCE(environment_id, '') = ? AND entity_type IN ('environment', 'stack', 'desired_state')))
		 ORDER BY timestamp DESC LIMIT ?`
	args := append([]any{start, end}, names...)
	args = append(args, env, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("reading audit rows: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("closing rows failed", "error", cerr)
		}
	}()
	var out []AuditItem
	for rows.Next() {
		var item AuditItem
		var entityType, entityName string
		if err := rows.Scan(&item.At, &item.Who, &item.Action, &entityType, &entityName); err != nil {
			return nil, fmt.Errorf("scanning audit row: %w", err)
		}
		item.Entity = entityType + ":" + entityName
		if item.Who == "" {
			item.Who = "system"
		}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At < out[j].At })
	return out, rows.Err()
}

// promotionsInto lists promotions that targeted env and started in [start, end], newest first.
func (s *Store) promotionsInto(ctx context.Context, env, start, end string, limit int) ([]DeployItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT COALESCE(p.finished_at, p.started_at), pl.name, s.stage_name, p.commit_sha, p.status
		 FROM gitops_promotions p
		 JOIN gitops_pipeline_stages s ON s.id = p.stage_id
		 JOIN gitops_pipelines pl ON pl.id = p.pipeline_id
		 WHERE s.target_environment_id = ? AND p.started_at >= ? AND p.started_at <= ?
		 ORDER BY p.started_at DESC LIMIT ?`, env, start, end, limit)
	if err != nil {
		return nil, fmt.Errorf("reading promotions: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("closing rows failed", "error", cerr)
		}
	}()
	var out []DeployItem
	for rows.Next() {
		var d DeployItem
		if err := rows.Scan(&d.At, &d.Pipeline, &d.Stage, &d.Commit, &d.Status); err != nil {
			return nil, fmt.Errorf("scanning promotion: %w", err)
		}
		if len(d.Commit) > 12 {
			d.Commit = d.Commit[:12]
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// alertEventItems reads alert events for a subject in [start, end], oldest first.
func (s *Store) alertEventItems(ctx context.Context, subject, env, start, end string, limit int) ([]AlertItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT created_at, alert_name, state, message FROM alert_events
		 WHERE subject_name = ? AND COALESCE(environment_id, '') = ? AND created_at >= ? AND created_at <= ?
		 ORDER BY created_at DESC LIMIT ?`, subject, env, start, end, limit)
	if err != nil {
		return nil, fmt.Errorf("reading alert events: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("closing rows failed", "error", cerr)
		}
	}()
	var out []AlertItem
	for rows.Next() {
		var item AlertItem
		if err := rows.Scan(&item.At, &item.Alert, &item.State, &item.Message); err != nil {
			return nil, fmt.Errorf("scanning alert event: %w", err)
		}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At < out[j].At })
	return out, rows.Err()
}

// seriesKind names one of the four metric series an evidence bundle carries.
type seriesKind int

const (
	seriesHostCPU seriesKind = iota
	seriesHostMem
	seriesContainerCPU
	seriesContainerMem
)

var seriesQueries = map[seriesKind]string{
	seriesHostCPU: `SELECT timestamp, cpu_percent FROM host_metrics WHERE environment_id = ?
			AND timestamp >= ? AND timestamp <= ? ORDER BY timestamp LIMIT 5000`,
	seriesHostMem: `SELECT timestamp, memory_percent FROM host_metrics WHERE environment_id = ?
			AND timestamp >= ? AND timestamp <= ? ORDER BY timestamp LIMIT 5000`,
	seriesContainerCPU: `SELECT sampled_at, cpu_percent FROM container_metrics WHERE container_name = ?
		AND COALESCE(environment_id, '') = ? AND sampled_at >= ? AND sampled_at <= ? ORDER BY sampled_at LIMIT 5000`,
	seriesContainerMem: `SELECT sampled_at, mem_used FROM container_metrics WHERE container_name = ?
		AND COALESCE(environment_id, '') = ? AND sampled_at >= ? AND sampled_at <= ? ORDER BY sampled_at LIMIT 5000`,
}

// metricSeries reads one metric series in [start, end] and downsamples it. Host
// series are keyed on env alone; container series on the container name and env.
func (s *Store) metricSeries(ctx context.Context, kind seriesKind, name, env, start, end string) ([]MetricPoint, error) {
	args := []any{name, env, start, end}
	if kind == seriesHostCPU || kind == seriesHostMem {
		args = []any{env, start, end}
	}
	rows, err := s.db.QueryContext(ctx, seriesQueries[kind], args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("closing rows failed", "error", cerr)
		}
	}()
	var pts []MetricPoint
	for rows.Next() {
		var p MetricPoint
		var v sql.NullFloat64
		if err := rows.Scan(&p.At, &v); err != nil {
			return nil, err
		}
		if v.Valid {
			p.Value = v.Float64
			if t, err := time.Parse(metricsLayout, p.At); err == nil {
				p.At = t.UTC().Format(time.RFC3339)
			}
			pts = append(pts, p)
		}
	}
	return downsample(pts, maxPoints), rows.Err()
}

// driftItems lists the configuration diffs of drifted desired states that watch
// the container or its stack.
func (s *Store) driftItems(ctx context.Context, env, container, stack string) ([]DriftItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT name, COALESCE(last_report, '') FROM desired_states
		 WHERE drift_detected = 1 AND COALESCE(env_id, '') = ? AND (container_name = ? OR (stack_name <> '' AND stack_name = ?))
		 LIMIT 20`, env, container, stack)
	if err != nil {
		return nil, fmt.Errorf("reading drift: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("closing rows failed", "error", cerr)
		}
	}()
	var out []DriftItem
	for rows.Next() {
		var name, raw string
		if err := rows.Scan(&name, &raw); err != nil {
			return nil, fmt.Errorf("scanning drift: %w", err)
		}
		var report struct {
			Diffs []struct{ Field, Expected, Actual string } `json:"diffs"`
		}
		if raw == "" || json.Unmarshal([]byte(raw), &report) != nil {
			continue
		}
		for _, d := range report.Diffs {
			out = append(out, DriftItem{Watch: name, Field: d.Field, Expected: d.Expected, Actual: d.Actual})
		}
	}
	return out, rows.Err()
}

// runbookRuns reads the runbook runs recorded against an incident or its alert
// event, oldest first, each with the last step the operator reached.
func (s *Store) runbookRuns(ctx context.Context, incidentID, alertEventID string, limit int) ([]RunbookTrace, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, COALESCE(runbook_name, ''), COALESCE(started_by_name, ''), started_at,
		        COALESCE(outcome, ''), COALESCE(note, '')
		 FROM runbook_runs
		 WHERE (? <> '' AND incident_id = ?) OR (? <> '' AND alert_event_id = ?)
		 ORDER BY started_at ASC LIMIT ?`,
		incidentID, incidentID, alertEventID, alertEventID, limit)
	if err != nil {
		return nil, fmt.Errorf("reading runbook runs: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("closing rows failed", "error", cerr)
		}
	}()
	var out []RunbookTrace
	ids := []string{}
	for rows.Next() {
		var id string
		var tr RunbookTrace
		if err := rows.Scan(&id, &tr.Runbook, &tr.Who, &tr.At, &tr.Outcome, &tr.Note); err != nil {
			return nil, fmt.Errorf("scanning runbook run: %w", err)
		}
		if tr.Who == "" {
			tr.Who = "unknown"
		}
		out = append(out, tr)
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i, id := range ids {
		if err := s.fillStopPoint(ctx, id, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// fillStopPoint reads the last step the operator reached, which is what makes
// the timeline line say something a backwards audit scan never could.
func (s *Store) fillStopPoint(ctx context.Context, runID string, tr *RunbookTrace) error {
	err := s.db.QueryRowContext(ctx,
		`SELECT step_position, COALESCE(step_title, ''), COALESCE(state, ''), COALESCE(finding, '')
		 FROM runbook_step_results WHERE run_id = ? ORDER BY step_position DESC, at DESC LIMIT 1`,
		runID).Scan(&tr.StoppedAt, &tr.StoppedTitle, &tr.StoppedState, &tr.Finding)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading the runbook stop point: %w", err)
	}
	return nil
}
