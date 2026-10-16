// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package containers manages Docker containers: lifecycle, exec, logs, files, moves between environments and health history.
package containers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

// ExecHistoryEntry is one row of container_exec_history.
type ExecHistoryEntry struct {
	ID            string   `json:"id"`
	UserID        string   `json:"userId"`
	Username      string   `json:"username,omitempty"`
	EnvironmentID string   `json:"environmentId"`
	ContainerID   string   `json:"containerId"`
	ContainerName string   `json:"containerName,omitempty"`
	Command       []string `json:"command"`
	WorkDir       string   `json:"workDir,omitempty"`
	User          string   `json:"user,omitempty"`
	ExitCode      int      `json:"exitCode"`
	Stdout        string   `json:"stdout"`
	Stderr        string   `json:"stderr"`
	DurationMs    int      `json:"durationMs"`
	CreatedAt     string   `json:"createdAt"`
}

// listExecHistory returns the most recent exec entries for the given
// environment + container (or all if containerID is empty). A non-empty onlyUser
// keeps just that user's rows, for a caller who may not read other people's output.
func (s *Service) listExecHistory(ctx context.Context, envID, containerID, userID, onlyUser string, limit int) ([]ExecHistoryEntry, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	var (
		rows *sql.Rows
		err  error
	)
	switch {
	case containerID != "":
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, user_id, environment_id, container_id, container_name, command,
			        COALESCE(work_dir, ''), COALESCE(user_name, ''),
			        exit_code, stdout, stderr, duration_ms,
			        created_at
			 FROM container_exec_history
			 WHERE environment_id = ? AND container_id = ? AND (? = '' OR user_id = ?)
			 ORDER BY created_at DESC LIMIT ?`,
			envID, containerID, onlyUser, onlyUser, limit,
		)
	case userID != "":
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, user_id, environment_id, container_id, container_name, command,
			        COALESCE(work_dir, ''), COALESCE(user_name, ''),
			        exit_code, stdout, stderr, duration_ms,
			        created_at
			 FROM container_exec_history
			 WHERE user_id = ?
			 ORDER BY created_at DESC LIMIT ?`,
			userID, limit,
		)
	default:
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, user_id, environment_id, container_id, container_name, command,
			        COALESCE(work_dir, ''), COALESCE(user_name, ''),
			        exit_code, stdout, stderr, duration_ms,
			        created_at
			 FROM container_exec_history
			 WHERE environment_id = ? AND (? = '' OR user_id = ?)
			 ORDER BY created_at DESC LIMIT ?`,
			envID, onlyUser, onlyUser, limit,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("querying exec history: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing rows failed", "error", err)
		}
	}()

	var out []ExecHistoryEntry
	for rows.Next() {
		var (
			e         ExecHistoryEntry
			cmdJSON   string
			createdAt string
		)
		if err := rows.Scan(&e.ID, &e.UserID, &e.EnvironmentID, &e.ContainerID, &e.ContainerName, &cmdJSON,
			&e.WorkDir, &e.User, &e.ExitCode, &e.Stdout, &e.Stderr, &e.DurationMs, &createdAt); err != nil {
			return nil, fmt.Errorf("scanning exec history row: %w", err)
		}
		if err := json.Unmarshal([]byte(cmdJSON), &e.Command); err != nil {
			slog.Warn("decoding exec history command failed", "error", err, "id", e.ID)
			e.Command = nil
		}
		// created_at is stored as RFC3339 text; pass through as-is and
		// fall back to an empty string when the column is missing.
		if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			e.CreatedAt = t.UTC().Format(time.RFC3339)
		} else {
			e.CreatedAt = createdAt
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading exec history rows: %w", err)
	}
	return out, nil
}

// recordExecHistory persists a single exec invocation for the audit trail.
func (s *Service) recordExecHistory(ctx context.Context, entry ExecHistoryEntry) error {
	if s.db == nil {
		return nil
	}
	cmdJSON, err := json.Marshal(entry.Command)
	if err != nil {
		return fmt.Errorf("encoding command: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO container_exec_history
		 (id, user_id, environment_id, container_id, container_name, command,
		  work_dir, user_name, exit_code, stdout, stderr, duration_ms, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.UserID, entry.EnvironmentID, entry.ContainerID, entry.ContainerName,
		string(cmdJSON), entry.WorkDir, entry.User, entry.ExitCode,
		entry.Stdout, entry.Stderr, entry.DurationMs,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("recording exec history: %w", err)
	}
	return nil
}
