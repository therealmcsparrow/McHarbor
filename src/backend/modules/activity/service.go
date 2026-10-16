// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package activity

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
)

// ContainerEvent represents a container event stored in the DB.
type ContainerEvent struct {
	ID            string  `json:"id"`
	EnvironmentID *string `json:"environmentId,omitempty"`
	ContainerID   string  `json:"containerId"`
	ContainerName *string `json:"containerName,omitempty"`
	EventType     string  `json:"eventType"`
	Action        string  `json:"action"`
	Metadata      *string `json:"metadata,omitempty"`
	Timestamp     string  `json:"timestamp"`
	CreatedAt     string  `json:"createdAt"`
	UpdatedAt     string  `json:"updatedAt"`
	// EnvironmentName is joined from the environments table in List only.
	EnvironmentName *string `json:"environmentName,omitempty"`
}

// CreateRequest is the JSON body for POST /activity.
type CreateRequest struct {
	EnvironmentID *string `json:"environmentId,omitempty"`
	ContainerID   string  `json:"containerId"`
	ContainerName *string `json:"containerName,omitempty"`
	EventType     string  `json:"eventType"`
	Action        string  `json:"action"`
	Metadata      *string `json:"metadata,omitempty"`
}

// Service handles container event operations against the database.
type Service struct {
	db *sql.DB

	mu      sync.Mutex
	total   int64
	totalAt time.Time
	now     func() time.Time
}

// unfilteredTotalTTL is how long the unfiltered event count is reused. It is
// a full-table COUNT on the largest table and runs on every page load, while a
// few seconds' lag in "N events" is invisible.
const unfilteredTotalTTL = 30 * time.Second

// NewService creates a new activity service.
func NewService(db *sql.DB) *Service {
	return &Service{db: db, now: time.Now}
}

// count returns the rows matching where; the unfiltered count is cached.
func (s *Service) count(ctx context.Context, from, where string, args []any, unfiltered bool) (int64, error) {
	if unfiltered {
		s.mu.Lock()
		if !s.totalAt.IsZero() && s.now().Sub(s.totalAt) < unfilteredTotalTTL {
			total := s.total
			s.mu.Unlock()
			return total, nil
		}
		s.mu.Unlock()
	}
	var total int64
	query := "SELECT COUNT(*) " + from + " WHERE " + where
	if unfiltered {
		query = "SELECT COUNT(*) FROM container_events"
	}
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&total); err != nil {
		return 0, err
	}
	if unfiltered {
		s.mu.Lock()
		s.total, s.totalAt = total, s.now()
		s.mu.Unlock()
	}
	return total, nil
}

// ListFilter narrows the activity list query by the UI filters.
//
// From and To are inclusive lower/upper bounds on the timestamp column;
// a zero time.Time disables the corresponding bound. Search is the free-text
// query and SearchMode is `contains` (default) or `exact` — see
// db.SearchClause.
type ListFilter struct {
	EnvironmentID string
	From          time.Time
	To            time.Time
	Search        string
	SearchMode    string
}

// searchColumns are the fields the free-text query matches. The environment
// name comes from the join so a search for an environment reads the same as
// the name rendered in the table, not the opaque id.
var searchColumns = []string{
	"ce.container_id",
	"ce.container_name",
	"ce.action",
	"ce.event_type",
	"ce.metadata",
	"env.name",
}

// List returns paginated container events with optional filtering.
func (s *Service) List(ctx context.Context, page, perPage int, f ListFilter) ([]ContainerEvent, int64, error) {
	var conditions []string
	var args []any

	if f.EnvironmentID != "" {
		conditions = append(conditions, "ce.environment_id = ?")
		args = append(args, f.EnvironmentID)
	}
	if !f.From.IsZero() {
		conditions = append(conditions, "ce.timestamp >= ?")
		args = append(args, f.From.UTC().Format(time.RFC3339))
	}
	if !f.To.IsZero() {
		conditions = append(conditions, "ce.timestamp <= ?")
		args = append(args, f.To.UTC().Format(time.RFC3339))
	}
	if clause, clauseArgs := db.SearchClause(f.Search, f.SearchMode, searchColumns); clause != "" {
		conditions = append(conditions, clause)
		args = append(args, clauseArgs...)
	}

	whereClause := "1=1"
	if len(conditions) > 0 {
		whereClause = strings.Join(conditions, " AND ")
	}

	const fromClause = `
		FROM container_events ce
		LEFT JOIN environments env ON env.id = ce.environment_id
	`

	total, err := s.count(ctx, fromClause, whereClause, args, len(conditions) == 0)
	if err != nil {
		return nil, 0, fmt.Errorf("counting events: %w", err)
	}

	offset := (page - 1) * perPage
	query := fmt.Sprintf(`
		SELECT ce.id, ce.environment_id, ce.container_id, ce.container_name,
		       ce.event_type, ce.action, ce.metadata, ce.timestamp,
		       ce.created_at, ce.updated_at, env.name
		%s
		WHERE %s
		ORDER BY ce.timestamp DESC
		LIMIT ? OFFSET ?
	`, fromClause, whereClause)

	args = append(args, perPage, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying events: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing rows failed", "error", err)
		}
	}()

	var events []ContainerEvent
	for rows.Next() {
		evt, err := scanEvent(rows)
		if err != nil {
			return nil, 0, err
		}
		events = append(events, evt)
	}
	if events == nil {
		events = []ContainerEvent{}
	}
	return events, total, rows.Err()
}

// Create inserts a new container event.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*ContainerEvent, error) {
	if req.ContainerID == "" {
		return nil, fmt.Errorf("containerId is required")
	}
	if req.EventType == "" {
		return nil, fmt.Errorf("eventType is required")
	}
	if req.Action == "" {
		return nil, fmt.Errorf("action is required")
	}

	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO container_events (id, environment_id, container_id, container_name,
		                              event_type, action, metadata, timestamp,
		                              created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, req.EnvironmentID, req.ContainerID, req.ContainerName,
		req.EventType, req.Action, req.Metadata, now, now, now)
	if err != nil {
		return nil, fmt.Errorf("inserting event: %w", err)
	}

	return &ContainerEvent{
		ID:            id,
		EnvironmentID: req.EnvironmentID,
		ContainerID:   req.ContainerID,
		ContainerName: req.ContainerName,
		EventType:     req.EventType,
		Action:        req.Action,
		Metadata:      req.Metadata,
		Timestamp:     now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

func scanEvent(rows *sql.Rows) (ContainerEvent, error) {
	var evt ContainerEvent
	var envID, containerName, metadata, envName sql.NullString

	err := rows.Scan(
		&evt.ID, &envID, &evt.ContainerID, &containerName,
		&evt.EventType, &evt.Action, &metadata,
		&evt.Timestamp, &evt.CreatedAt, &evt.UpdatedAt, &envName,
	)
	if err != nil {
		return evt, fmt.Errorf("scanning event: %w", err)
	}

	evt.EnvironmentID = nullStringPtr(envID)
	evt.ContainerName = nullStringPtr(containerName)
	evt.Metadata = nullStringPtr(metadata)
	evt.EnvironmentName = nullStringPtr(envName)

	return evt, nil
}

func nullStringPtr(ns sql.NullString) *string {
	if ns.Valid {
		return &ns.String
	}
	return nil
}

// PurgeByRetention deletes every container event older than the
// configured retention cutoff.
//
// The cutoff is computed in Go as an RFC3339 string and passed as a
// bound parameter so SQLite can use the timestamp index for the
// range scan. Earlier revisions used julianday(timestamp) on both
// sides of the comparison — that fix for the format-mismatch bug
// was correct but forced a full table scan because julianday() is
// a function expression and therefore not indexable. Comparing the
// stored RFC3339 value against an RFC3339 bound uses the index
// directly and is the same chronological order.
//
// Returns the number of rows actually deleted.
func (s *Service) PurgeByRetention(ctx context.Context) (int64, int, error) {
	days := coreSettings.ReadRetentionSettingsContext(ctx, s.db).ActivityRetentionDays
	if days <= 0 {
		return 0, 0, nil
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days).Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx,
		"DELETE FROM container_events WHERE timestamp < ?",
		cutoff,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("purging activity events: %w", err)
	}
	n := db.RowsAffected(result)
	slog.Info("container events purged", "retention_days", days, "rows_deleted", n)
	return n, days, nil
}
