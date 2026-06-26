// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

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
//
// from and to are inclusive lower/upper bounds on the timestamp column.
// A zero time.Time disables the corresponding bound.
func (s *Service) List(page, perPage int, action, entityType string, from, to time.Time) ([]AuditLog, int64, error) {
	var total int64
	offset := (page - 1) * perPage
	rows, total, err := s.listRows(page, perPage, offset, action, entityType, from, to)
	if err != nil {
		return nil, 0, err
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

func (s *Service) listRows(page, perPage, offset int, action, entityType string, from, to time.Time) (*sql.Rows, int64, error) {
	var (
		rows  *sql.Rows
		err   error
		total int64
	)

	whereParts := []string{}
	args := []any{}
	if action != "" {
		whereParts = append(whereParts, "action = ?")
		args = append(args, action)
	}
	if entityType != "" {
		whereParts = append(whereParts, "entity_type = ?")
		args = append(args, entityType)
	}
	if !from.IsZero() {
		whereParts = append(whereParts, "timestamp >= ?")
		args = append(args, from.UTC().Format(time.RFC3339))
	}
	if !to.IsZero() {
		whereParts = append(whereParts, "timestamp <= ?")
		args = append(args, to.UTC().Format(time.RFC3339))
	}

	whereClause := ""
	if len(whereParts) > 0 {
		whereClause = " WHERE " + strings.Join(whereParts, " AND ")
	}

	countQuery := "SELECT COUNT(*) FROM audit_logs" + whereClause
	if err = s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting audit logs: %w", err)
	}

	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, perPage, offset)
	rows, err = s.db.Query(`
		SELECT id, user_id, username, action, entity_type, entity_id, entity_name,
		       details, ip_address, environment_id, timestamp, created_at, updated_at
		FROM audit_logs`+whereClause+`
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?
	`, queryArgs...)

	if err != nil {
		return nil, 0, fmt.Errorf("querying audit logs: %w", err)
	}

	return rows, total, nil
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

// PurgeByRetention deletes every audit log row older than the
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
	days := coreSettings.ReadRetentionSettings(s.db).AuditRetentionDays
	if days <= 0 {
		return 0, 0, nil
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days).Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx,
		"DELETE FROM audit_logs WHERE timestamp < ?",
		cutoff,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("purging audit logs: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, 0, fmt.Errorf("reading rows affected: %w", err)
	}
	slog.Info("audit logs purged", "retention_days", days, "rows_deleted", n)
	return n, days, nil
}
