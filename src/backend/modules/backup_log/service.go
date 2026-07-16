// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package backup_log is the read API for the container_backup_log
// table. Writes happen inside the container_backups module
// (progress collector, lifecycle hooks). The Logging tab in the
// Backup page reads through this service.
package backup_log

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

// BackupLog is one row of the container_backup_log table.
type BackupLog struct {
	ID             string  `json:"id"`
	EnvironmentID  *string `json:"environmentId,omitempty"`
	PlanID         *string `json:"planId,omitempty"`
	PlanName       *string `json:"planName,omitempty"`
	RunID          *string `json:"runId,omitempty"`
	ContainerID    *string `json:"containerId,omitempty"`
	ContainerName  *string `json:"containerName,omitempty"`
	Action         string  `json:"action"`
	Phase          string  `json:"phase"`
	Severity       string  `json:"severity"`
	Message        string  `json:"message"`
	Source         string  `json:"source"`
	CreatedAt      string  `json:"createdAt"`
}

// ListFilter narrows the read query.
type ListFilter struct {
	EnvironmentID string
	PlanID        string
	RunID         string
	Severity      string // empty = all
	Action        string // empty = all
	Search        string // matches message or plan_name
	From          time.Time
	Until         time.Time
	Page          int
	PerPage       int
}

// Service is the read-side of the backup log.
type Service struct {
	db *sql.DB
}

// NewService creates a new backup_log service.
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// List returns the filtered log events sorted by created_at desc.
func (s *Service) List(ctx context.Context, f ListFilter) ([]BackupLog, int64, error) {
	conds := []string{"1=1"}
	args := []any{}
	if f.EnvironmentID != "" {
		conds = append(conds, "environment_id = ?")
		args = append(args, f.EnvironmentID)
	}
	if f.PlanID != "" {
		conds = append(conds, "plan_id = ?")
		args = append(args, f.PlanID)
	}
	if f.RunID != "" {
		conds = append(conds, "run_id = ?")
		args = append(args, f.RunID)
	}
	if f.Severity != "" {
		conds = append(conds, "severity = ?")
		args = append(args, f.Severity)
	}
	if f.Action != "" {
		conds = append(conds, "action = ?")
		args = append(args, f.Action)
	}
	if !f.From.IsZero() {
		conds = append(conds, "created_at >= ?")
		args = append(args, f.From.UTC().Format(time.RFC3339))
	}
	if !f.Until.IsZero() {
		conds = append(conds, "created_at <= ?")
		args = append(args, f.Until.UTC().Format(time.RFC3339))
	}
	if s := strings.TrimSpace(f.Search); s != "" {
		conds = append(conds, "(message LIKE ? OR plan_name LIKE ? OR container_name LIKE ?)")
		like := "%" + s + "%"
		args = append(args, like, like, like)
	}
	where := strings.Join(conds, " AND ")

	page := f.Page
	if page < 1 {
		page = 1
	}
	perPage := f.PerPage
	if perPage < 1 {
		perPage = 50
	}
	if perPage > 200 {
		perPage = 200
	}

	var total int64
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM container_backup_log WHERE "+where, args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting backup log: %w", err)
	}

	offset := (page - 1) * perPage
	args = append(args, perPage, offset)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, environment_id, plan_id, plan_name, run_id,
		       container_id, container_name, action, phase, severity,
		       message, source, created_at
		FROM container_backup_log
		WHERE `+where+`
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?`,
		args...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("querying backup log: %w", err)
	}
	defer rows.Close()
	out := make([]BackupLog, 0, perPage)
	for rows.Next() {
		var log BackupLog
		var envID, planID, planName, runID, containerID, containerName sql.NullString
		if err := rows.Scan(&log.ID, &envID, &planID, &planName, &runID,
			&containerID, &containerName, &log.Action, &log.Phase, &log.Severity,
			&log.Message, &log.Source, &log.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning backup log: %w", err)
		}
		if envID.Valid {
			v := envID.String
			log.EnvironmentID = &v
		}
		if planID.Valid {
			v := planID.String
			log.PlanID = &v
		}
		if planName.Valid {
			v := planName.String
			log.PlanName = &v
		}
		if runID.Valid {
			v := runID.String
			log.RunID = &v
		}
		if containerID.Valid {
			v := containerID.String
			log.ContainerID = &v
		}
		if containerName.Valid {
			v := containerName.String
			log.ContainerName = &v
		}
		out = append(out, log)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating backup log: %w", err)
	}
	return out, total, nil
}

// PurgeByRetention deletes every container_backup_log row older than the configured retention cutoff.
func (s *Service) PurgeByRetention(ctx context.Context) (int64, int, error) {
	days := coreSettings.ReadRetentionSettings(s.db).BackupLogRetentionDays
	if days <= 0 {
		return 0, 0, nil
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days).Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx,
		"DELETE FROM container_backup_log WHERE created_at < ?",
		cutoff,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("purging backup log rows: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, 0, fmt.Errorf("reading rows affected: %w", err)
	}
	slog.Info("backup log rows purged", "retention_days", days, "rows_deleted", n)
	if n == 0 {
		return 0, days, nil
	}
	return n, days, nil
}

// Record inserts a log row. Other modules (e.g. container_backups)
// call this when a backup lifecycle event happens. Returns the new
// row id. The id is xid.New() for uniqueness and URL-safety.
func (s *Service) Record(
	ctx context.Context,
	environmentID, planID, planName, runID, containerID, containerName,
	action, phase, severity, message string,
) (string, error) {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	var envP, planP, runP, contP interface{}
	if environmentID != "" {
		envP = environmentID
	}
	if planID != "" {
		planP = planID
	}
	if runID != "" {
		runP = runID
	}
	if containerID != "" {
		contP = containerID
	}
	if severity == "" {
		severity = "info"
	}
	if message == "" {
		message = action
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO container_backup_log
			(id, environment_id, plan_id, plan_name, run_id,
			 container_id, container_name, action, phase, severity,
			 message, source, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'backup', ?, ?)`,
		id, envP, planP, planName, runP, contP, containerName,
		action, phase, severity, message, now, now,
	)
	if err != nil {
		return "", fmt.Errorf("inserting backup log: %w", err)
	}
	return id, nil
}
