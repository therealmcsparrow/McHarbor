// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/rs/xid"
)

// alertEventRetention is how long history is kept. Long enough to
// answer "what fired last night" and "was this flapping all week",
// short enough that a noisy rule cannot grow the database without
// bound.
const alertEventRetention = 30 * 24 * time.Hour

// historyStore writes and reads alert_events.
type historyStore struct {
	db *sql.DB
}

func newHistoryStore(db *sql.DB) *historyStore { return &historyStore{db: db} }

// record persists one firing or recovery.
//
// alert_name is denormalised and there is no foreign key to alerts, so
// history survives deleting the rule that produced it — which is
// exactly when you want to look at it.
func (h *historyStore) record(ctx context.Context, event AlertEvent) error {
	deliveries, err := json.Marshal(event.Deliveries)
	if err != nil {
		return fmt.Errorf("encoding alert deliveries: %w", err)
	}

	_, err = h.db.ExecContext(ctx, `
		INSERT INTO alert_events (
			id, alert_id, alert_name, source_type, condition_key,
			environment_id, environment_name, subject_id, subject_name,
			state, severity, value, value_label, message, deliveries, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		xid.New().String(), event.AlertID, event.AlertName, event.SourceType, event.ConditionKey,
		event.EnvironmentID, event.EnvironmentName, event.SubjectID, event.SubjectName,
		event.State, event.Severity, event.Value, event.ValueLabel, event.Message,
		string(deliveries), time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("recording alert event: %w", err)
	}

	return nil
}

// list returns a page of history, newest first.
func (h *historyStore) list(ctx context.Context, filter EventFilter, page, perPage int) ([]AlertEvent, int64, error) {
	where, args := filter.clause()

	var total int64
	if err := h.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM alert_events"+where, args...).Scan(&total); err != nil { // SQL identifier: where is built from fixed ? predicates in EventFilter.clause
		return nil, 0, fmt.Errorf("counting alert events: %w", err)
	}

	offset := (page - 1) * perPage
	const listColumns = `SELECT id, alert_id, alert_name, source_type, condition_key,
	                 COALESCE(environment_id, ''), COALESCE(environment_name, ''),
	                 COALESCE(subject_id, ''), COALESCE(subject_name, ''),
	                 state, severity, COALESCE(value, 0), COALESCE(value_label, ''),
	                 message, deliveries, created_at
	            FROM alert_events`
	query := listColumns + where + " ORDER BY created_at DESC LIMIT ? OFFSET ?" // SQL identifier: where is built from fixed ? predicates in EventFilter.clause

	rows, err := h.db.QueryContext(ctx, query, append(args, perPage, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing alert events: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()
	items := make([]AlertEvent, 0, perPage)
	for rows.Next() {
		var event AlertEvent
		var deliveries string
		if err := rows.Scan(&event.ID, &event.AlertID, &event.AlertName, &event.SourceType, &event.ConditionKey,
			&event.EnvironmentID, &event.EnvironmentName, &event.SubjectID, &event.SubjectName,
			&event.State, &event.Severity, &event.Value, &event.ValueLabel,
			&event.Message, &deliveries, &event.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning alert event: %w", err)
		}

		if err := json.Unmarshal([]byte(deliveries), &event.Deliveries); err != nil {
			// A malformed blob costs the delivery breakdown for one
			// row; it must not cost the whole page.
			event.Deliveries = nil
		}
		if event.Deliveries == nil {
			event.Deliveries = []EventDelivery{}
		}

		items = append(items, event)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating alert events: %w", err)
	}

	return items, total, nil
}

// prune drops events past the retention window.
func (h *historyStore) prune(ctx context.Context) error {
	cutoff := time.Now().UTC().Add(-alertEventRetention).Format(time.RFC3339)
	if _, err := h.db.ExecContext(ctx, "DELETE FROM alert_events WHERE created_at < ?", cutoff); err != nil {
		return fmt.Errorf("pruning alert events: %w", err)
	}
	return nil
}

// EventFilter narrows a history query.
type EventFilter struct {
	AlertID  string
	State    string
	Severity string
}

func (f EventFilter) clause() (string, []any) {
	conditions := make([]string, 0, 3)
	args := make([]any, 0, 3)

	if id := strings.TrimSpace(f.AlertID); id != "" {
		conditions = append(conditions, "alert_id = ?")
		args = append(args, id)
	}
	if state := strings.TrimSpace(f.State); state != "" {
		conditions = append(conditions, "state = ?")
		args = append(args, state)
	}
	if severity := strings.TrimSpace(f.Severity); severity != "" {
		conditions = append(conditions, "severity = ?")
		args = append(args, severity)
	}

	if len(conditions) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}
