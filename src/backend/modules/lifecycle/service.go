// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lifecycle

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/rs/xid"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/plugincaps"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
)

// LifecycleEvent is the row in the lifecycle_events table. The
// Docker menu "Logging" tab in the UI subscribes to this stream.
type LifecycleEvent struct { //nolint:revive // the name is the OpenAPI schema and generated-client name, so renaming it changes the wire contract
	ID            string  `json:"id"`
	EnvironmentID *string `json:"environmentId,omitempty"`
	SubjectType   string  `json:"subjectType"`
	SubjectID     string  `json:"subjectId"`
	SubjectName   *string `json:"subjectName,omitempty"`
	EventType     string  `json:"eventType"`
	Action        string  `json:"action"`
	State         *string `json:"state,omitempty"`
	Severity      string  `json:"severity"`
	Metadata      *string `json:"metadata,omitempty"`
	Source        string  `json:"source"`
	Timestamp     string  `json:"timestamp"`
	// EnvironmentName is joined from the environments table in List only.
	EnvironmentName *string `json:"environmentName,omitempty"`
}

// ListFilter narrows the list query by the standard UI filters.
type ListFilter struct {
	EnvironmentID string
	SubjectType   string // empty = all
	Severity      string // empty = all
	Since         time.Time
	Until         time.Time
	Search        string // free-text match on subject name or id
}

// ErrUnknownSubjectType refuses an event whose subject type is neither the host's own nor
// one a loaded plugin declares, the check migration 130 took out of the table's CHECK.
var ErrUnknownSubjectType = errors.New("lifecycle: unknown subject type")

// hostSubjectTypes are the subjects the host itself records; every other type is a
// plugin's, and is accepted only while that plugin declares it a lifecycle-subject.
var hostSubjectTypes = []string{"container", "image", "volume", "network", "stack"}

// Service handles lifecycle event queries against the database.
type Service struct {
	db *sql.DB
	// declared reports a plugin-declared subject type; tests swap it for a fixed set.
	declared func(subjectType string) bool
}

// NewService creates a lifecycle service that accepts the host's subject types plus
// whatever the loaded plugins declare.
func NewService(db *sql.DB) *Service {
	return &Service{db: db, declared: plugincaps.Default.IsLifecycleSubject}
}

// knownSubjectType reports whether subjectType may be written to the timeline.
func (s *Service) knownSubjectType(subjectType string) bool {
	for _, host := range hostSubjectTypes {
		if subjectType == host {
			return true
		}
	}
	return s.declared != nil && s.declared(subjectType)
}

// List returns lifecycle events ordered by timestamp desc, paginated
// by the (page, perPage) pair. perPage is clamped to [1, 200] so
// a misbehaving client can't drain the table.
func (s *Service) List(ctx context.Context, page, perPage int, f ListFilter) ([]LifecycleEvent, int64, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 25
	}
	if perPage > 200 {
		perPage = 200
	}
	// Every condition is qualified with the event table's alias because the
	// list query below joins environments, which also carries created_at /
	// updated_at — an unqualified column name there risks ambiguity, and
	// SQLite rejects the whole query rather than guessing. The count query
	// is aliased the same way purely so this one `where` string works
	// unmodified against both.
	conds := []string{"1=1"}
	args := []any{}
	if f.EnvironmentID != "" {
		conds = append(conds, "l.environment_id = ?")
		args = append(args, f.EnvironmentID)
	}
	if f.SubjectType != "" {
		conds = append(conds, "l.subject_type = ?")
		args = append(args, f.SubjectType)
	}
	if f.Severity != "" {
		conds = append(conds, "l.severity = ?")
		args = append(args, f.Severity)
	}
	if !f.Since.IsZero() {
		conds = append(conds, "l.timestamp >= ?")
		args = append(args, f.Since.UTC().Format(time.RFC3339))
	}
	if !f.Until.IsZero() {
		conds = append(conds, "l.timestamp <= ?")
		args = append(args, f.Until.UTC().Format(time.RFC3339))
	}
	if s := strings.TrimSpace(f.Search); s != "" {
		conds = append(conds, "(l.subject_id LIKE ? OR l.subject_name LIKE ?)")
		like := "%" + s + "%"
		args = append(args, like, like)
	}
	where := strings.Join(conds, " AND ")

	var total int64
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM lifecycle_events l WHERE "+where, args..., // SQL identifier: where is joined from fixed condition literals, values bind through ?
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting lifecycle events: %w", err)
	}

	offset := (page - 1) * perPage
	rows, err := s.db.QueryContext(ctx, listSelectSQL+where+listOrderSQL, // SQL identifier: where is joined from fixed condition literals, values bind through ?
		append(args, perPage, offset)...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("querying lifecycle events: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("lifecycle: closing event rows", "error", err)
		}
	}()

	out := make([]LifecycleEvent, 0, perPage)
	for rows.Next() {
		var ev LifecycleEvent
		var envID, subjName, state, metadata, envName sql.NullString
		if err := rows.Scan(&ev.ID, &envID, &ev.SubjectType, &ev.SubjectID, &subjName,
			&ev.EventType, &ev.Action, &state, &ev.Severity, &metadata, &ev.Source, &ev.Timestamp,
			&envName,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning lifecycle event: %w", err)
		}
		if envID.Valid {
			v := envID.String
			ev.EnvironmentID = &v
		}
		if subjName.Valid {
			v := subjName.String
			ev.SubjectName = &v
		}
		if state.Valid {
			v := state.String
			ev.State = &v
		}
		if metadata.Valid {
			v := metadata.String
			ev.Metadata = &v
		}
		if envName.Valid {
			v := envName.String
			ev.EnvironmentName = &v
		}
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating lifecycle events: %w", err)
	}
	return out, total, nil
}

// PurgeByRetention deletes every lifecycle event older than the
// configured retention cutoff. When the retention is 0 (or
// unset) the call is a no-op so callers can invoke it
// unconditionally.
func (s *Service) PurgeByRetention(ctx context.Context) (int64, int, error) {
	days := coreSettings.ReadRetentionSettingsContext(ctx, s.db).LifecycleRetentionDays
	if days <= 0 {
		return 0, 0, nil
	}
	// julianday() on BOTH sides, never a string comparison on this column.
	//
	// The previous form compared the TEXT timestamp against an RFC3339 cutoff
	// directly. That was wrong twice over. SQLite compares TEXT
	// lexicographically, so the cutoff "2026-07-29T…" sorted after every row
	// written before migration 130, which stored Docker's int64 nanoseconds
	// ("1787236463897873700") — '1' sorts before '2', so on a measured install
	// the predicate matched 1,450,023 of 1,450,023 rows and this call would
	// have deleted the entire table, today's events included. Even for correct
	// RFC3339 rows the comparison is fragile at a date boundary, because
	// datetime('now') renders with a space where RFC3339 uses 'T'.
	//
	// julianday() converts both sides to a numeric day count, making the
	// comparison chronological. It returns NULL for an unconvertible value, so
	// any row that somehow still holds the nanosecond form is skipped rather
	// than destroyed.
	result, err := s.db.ExecContext(ctx,
		"DELETE FROM lifecycle_events WHERE julianday(timestamp) < julianday('now', '-' || ? || ' days')",
		days,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("purging lifecycle events: %w", err)
	}
	n := coredb.RowsAffected(result)
	slog.Info("lifecycle events purged", "retention_days", days, "rows_deleted", n)
	if n == 0 {
		return 0, days, nil
	}
	return n, days, nil
}

// Recent returns the n most recent lifecycle events. The UI uses
// this to show "what just happened" before the user has applied
// any filter. n is clamped to [1, 50].
func (s *Service) Recent(ctx context.Context, n int) ([]LifecycleEvent, error) {
	if n < 1 {
		n = 10
	}
	if n > 50 {
		n = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, environment_id, subject_type, subject_id, subject_name,
		       event_type, action, state, severity, metadata, source, timestamp
		FROM lifecycle_events
		ORDER BY timestamp DESC
		LIMIT ?`, n)
	if err != nil {
		return nil, fmt.Errorf("querying recent lifecycle events: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("lifecycle: closing event rows", "error", err)
		}
	}()
	out := make([]LifecycleEvent, 0, n)
	for rows.Next() {
		var ev LifecycleEvent
		var envID, subjName, state, metadata sql.NullString
		if err := rows.Scan(&ev.ID, &envID, &ev.SubjectType, &ev.SubjectID, &subjName,
			&ev.EventType, &ev.Action, &state, &ev.Severity, &metadata, &ev.Source, &ev.Timestamp,
		); err != nil {
			return nil, fmt.Errorf("scanning recent lifecycle event: %w", err)
		}
		if envID.Valid {
			v := envID.String
			ev.EnvironmentID = &v
		}
		if subjName.Valid {
			v := subjName.String
			ev.SubjectName = &v
		}
		if state.Valid {
			v := state.String
			ev.State = &v
		}
		if metadata.Valid {
			v := metadata.String
			ev.Metadata = &v
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

// Insert allows other modules to write lifecycle events that
// didn't come from the Docker event stream — for example the
// McHarbor Compose stack lifecycle or the storage migration
// pipeline. The handler enforces a sane severity / state before
// the INSERT so the badge rendering in the UI is consistent.
func (s *Service) Insert(ctx context.Context, ev LifecycleEvent) error {
	if !s.knownSubjectType(ev.SubjectType) {
		return fmt.Errorf("%w: %q", ErrUnknownSubjectType, ev.SubjectType)
	}
	if ev.ID == "" {
		ev.ID = xid.New().String()
	}
	if ev.Source == "" {
		ev.Source = "mcharbor"
	}
	if ev.Severity == "" {
		ev.Severity = "info"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if ev.Timestamp == "" {
		ev.Timestamp = now
	}
	subjectName := ev.SubjectName
	state := ev.State
	metadata := ev.Metadata
	envID := ev.EnvironmentID
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO lifecycle_events
			(id, environment_id, subject_type, subject_id, subject_name,
			 event_type, action, state, severity, metadata, source, timestamp,
			 created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ev.ID, envID, ev.SubjectType, ev.SubjectID, subjectName,
		ev.EventType, ev.Action, state, ev.Severity, metadata, ev.Source, ev.Timestamp,
		now, now,
	)
	if err != nil {
		return fmt.Errorf("inserting lifecycle event: %w", err)
	}
	return nil
}

const listSelectSQL = `
	SELECT l.id, l.environment_id, l.subject_type, l.subject_id, l.subject_name,
	       l.event_type, l.action, l.state, l.severity, l.metadata, l.source, l.timestamp,
	       le.name
	FROM lifecycle_events l
	LEFT JOIN environments le ON le.id = l.environment_id
	WHERE `

const listOrderSQL = `
	ORDER BY l.timestamp DESC
	LIMIT ? OFFSET ?`
