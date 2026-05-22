// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package activity

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/rs/xid"
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
}

// NewService creates a new activity service.
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// List returns paginated container events.
func (s *Service) List(page, perPage int, envID string) ([]ContainerEvent, int64, error) {
	var conditions []string
	var args []any

	if envID != "" {
		conditions = append(conditions, "environment_id = ?")
		args = append(args, envID)
	}

	whereClause := "1=1"
	if len(conditions) > 0 {
		whereClause = strings.Join(conditions, " AND ")
	}

	// Count total
	var total int64
	countQuery := "SELECT COUNT(*) FROM container_events WHERE " + whereClause
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting events: %w", err)
	}

	// Fetch page
	offset := (page - 1) * perPage
	query := fmt.Sprintf(`
		SELECT id, environment_id, container_id, container_name, event_type, action,
		       metadata, timestamp, created_at, updated_at
		FROM container_events
		WHERE %s
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?
	`, whereClause)

	args = append(args, perPage, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying events: %w", err)
	}
	defer rows.Close()

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
func (s *Service) Create(req CreateRequest) (*ContainerEvent, error) {
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

	_, err := s.db.Exec(`
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
	var envID, containerName, metadata sql.NullString

	err := rows.Scan(
		&evt.ID, &envID, &evt.ContainerID, &containerName,
		&evt.EventType, &evt.Action, &metadata,
		&evt.Timestamp, &evt.CreatedAt, &evt.UpdatedAt,
	)
	if err != nil {
		return evt, fmt.Errorf("scanning event: %w", err)
	}

	evt.EnvironmentID = nullStringPtr(envID)
	evt.ContainerName = nullStringPtr(containerName)
	evt.Metadata = nullStringPtr(metadata)

	return evt, nil
}

func nullStringPtr(ns sql.NullString) *string {
	if ns.Valid {
		return &ns.String
	}
	return nil
}
