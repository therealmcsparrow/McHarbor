// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lifecycle

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/rs/xid"

	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
)

// LifecycleEvent is the row in the lifecycle_events table. The
// Docker menu "Logging" tab in the UI subscribes to this stream.
type LifecycleEvent struct {
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

// Service handles lifecycle event queries against the database.
type Service struct {
	db *sql.DB
}

// NewService creates a new lifecycle service.
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
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
	conds := []string{"1=1"}
	args := []any{}
	if f.EnvironmentID != "" {
		conds = append(conds, "environment_id = ?")
		args = append(args, f.EnvironmentID)
	}
	if f.SubjectType != "" {
		conds = append(conds, "subject_type = ?")
		args = append(args, f.SubjectType)
	}
	if f.Severity != "" {
		conds = append(conds, "severity = ?")
		args = append(args, f.Severity)
	}
	if !f.Since.IsZero() {
		conds = append(conds, "timestamp >= ?")
		args = append(args, f.Since.UTC().Format(time.RFC3339))
	}
	if !f.Until.IsZero() {
		conds = append(conds, "timestamp <= ?")
		args = append(args, f.Until.UTC().Format(time.RFC3339))
	}
	if s := strings.TrimSpace(f.Search); s != "" {
		conds = append(conds, "(subject_id LIKE ? OR subject_name LIKE ?)")
		like := "%" + s + "%"
		args = append(args, like, like)
	}
	where := strings.Join(conds, " AND ")

	var total int64
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM lifecycle_events WHERE "+where, args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting lifecycle events: %w", err)
	}

	offset := (page - 1) * perPage
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, environment_id, subject_type, subject_id, subject_name,
		       event_type, action, state, severity, metadata, source, timestamp
		FROM lifecycle_events
		WHERE `+where+`
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?`,
		append(args, perPage, offset)...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("querying lifecycle events: %w", err)
	}
	defer rows.Close()

	out := make([]LifecycleEvent, 0, perPage)
	for rows.Next() {
		var ev LifecycleEvent
		var envID, subjName, state, metadata sql.NullString
		if err := rows.Scan(&ev.ID, &envID, &ev.SubjectType, &ev.SubjectID, &subjName,
			&ev.EventType, &ev.Action, &state, &ev.Severity, &metadata, &ev.Source, &ev.Timestamp,
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
	days := coreSettings.ReadRetentionSettings(s.db).LifecycleRetentionDays
	if days <= 0 {
		return 0, 0, nil
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days).Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx,
		"DELETE FROM lifecycle_events WHERE timestamp < ?",
		cutoff,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("purging lifecycle events: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, 0, fmt.Errorf("reading rows affected: %w", err)
	}
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
	defer rows.Close()
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
