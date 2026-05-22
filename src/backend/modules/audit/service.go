// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/rs/xid"
)

// AuditLog represents a single audit log entry.
type AuditLog struct {
	ID            string  `json:"id"`
	UserID        *string `json:"userId,omitempty"`
	Username      *string `json:"username,omitempty"`
	Action        string  `json:"action"`
	EntityType    *string `json:"entityType,omitempty"`
	EntityID      *string `json:"entityId,omitempty"`
	EntityName    *string `json:"entityName,omitempty"`
	Details       *string `json:"details,omitempty"`
	IPAddress     *string `json:"ipAddress,omitempty"`
	EnvironmentID *string `json:"environmentId,omitempty"`
	Timestamp     string  `json:"timestamp"`
	CreatedAt     string  `json:"createdAt"`
	UpdatedAt     string  `json:"updatedAt"`
}

// CreateRequest is the JSON body for POST /audit.
type CreateRequest struct {
	UserID        *string `json:"userId,omitempty"`
	Username      *string `json:"username,omitempty"`
	Action        string  `json:"action"`
	EntityType    *string `json:"entityType,omitempty"`
	EntityID      *string `json:"entityId,omitempty"`
	EntityName    *string `json:"entityName,omitempty"`
	Details       *string `json:"details,omitempty"`
	IPAddress     *string `json:"ipAddress,omitempty"`
	EnvironmentID *string `json:"environmentId,omitempty"`
}

// Service handles audit log operations against the database.
type Service struct {
	db *sql.DB
}

// NewService creates a new audit service.
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// List returns paginated audit logs with optional filtering.
func (s *Service) List(page, perPage int, action, entityType string) ([]AuditLog, int64, error) {
	// Build WHERE clause with slice-based builder
	var conditions []string
	var args []any

	if action != "" {
		conditions = append(conditions, "action = ?")
		args = append(args, action)
	}
	if entityType != "" {
		conditions = append(conditions, "entity_type = ?")
		args = append(args, entityType)
	}

	whereClause := "1=1"
	if len(conditions) > 0 {
		whereClause = strings.Join(conditions, " AND ")
	}

	// Count total
	var total int64
	countQuery := "SELECT COUNT(*) FROM audit_logs WHERE " + whereClause
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting audit logs: %w", err)
	}

	// Fetch page
	offset := (page - 1) * perPage
	query := fmt.Sprintf(`
		SELECT id, user_id, username, action, entity_type, entity_id, entity_name,
		       details, ip_address, environment_id, timestamp, created_at, updated_at
		FROM audit_logs
		WHERE %s
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?
	`, whereClause)

	args = append(args, perPage, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying audit logs: %w", err)
	}
	defer rows.Close()

	var logs []AuditLog
	for rows.Next() {
		log, err := scanAuditLog(rows)
		if err != nil {
			return nil, 0, err
		}
		logs = append(logs, log)
	}
	if logs == nil {
		logs = []AuditLog{}
	}
	return logs, total, rows.Err()
}

// Create inserts a new audit log entry.
func (s *Service) Create(req CreateRequest) (*AuditLog, error) {
	if req.Action == "" {
		return nil, fmt.Errorf("action is required")
	}

	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.Exec(`
		INSERT INTO audit_logs (id, user_id, username, action, entity_type, entity_id,
		                        entity_name, details, ip_address, environment_id,
		                        timestamp, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, req.UserID, req.Username, req.Action, req.EntityType, req.EntityID,
		req.EntityName, req.Details, req.IPAddress, req.EnvironmentID,
		now, now, now)
	if err != nil {
		return nil, fmt.Errorf("inserting audit log: %w", err)
	}

	return &AuditLog{
		ID:            id,
		UserID:        req.UserID,
		Username:      req.Username,
		Action:        req.Action,
		EntityType:    req.EntityType,
		EntityID:      req.EntityID,
		EntityName:    req.EntityName,
		Details:       req.Details,
		IPAddress:     req.IPAddress,
		EnvironmentID: req.EnvironmentID,
		Timestamp:     now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

func scanAuditLog(rows *sql.Rows) (AuditLog, error) {
	var log AuditLog
	var userID, username, entityType, entityID, entityName sql.NullString
	var details, ipAddr, envID sql.NullString

	err := rows.Scan(
		&log.ID, &userID, &username, &log.Action,
		&entityType, &entityID, &entityName,
		&details, &ipAddr, &envID,
		&log.Timestamp, &log.CreatedAt, &log.UpdatedAt,
	)
	if err != nil {
		return log, fmt.Errorf("scanning audit log: %w", err)
	}

	log.UserID = nullStringPtr(userID)
	log.Username = nullStringPtr(username)
	log.EntityType = nullStringPtr(entityType)
	log.EntityID = nullStringPtr(entityID)
	log.EntityName = nullStringPtr(entityName)
	log.Details = nullStringPtr(details)
	log.IPAddress = nullStringPtr(ipAddr)
	log.EnvironmentID = nullStringPtr(envID)

	return log, nil
}

func nullStringPtr(ns sql.NullString) *string {
	if ns.Valid {
		return &ns.String
	}
	return nil
}
