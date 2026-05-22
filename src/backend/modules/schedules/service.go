// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package schedules

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// Service handles schedule business logic and database operations.
type Service struct {
	db *sql.DB
}

// NewService creates a new schedules service.
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// List returns a paginated list of schedules and the total count.
func (s *Service) List(ctx context.Context, page, perPage int) ([]Schedule, int64, error) {
	var total int64
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schedules").Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting schedules: %w", err)
	}

	offset := (page - 1) * perPage
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, description, cron, action, target, env_id, enabled,
		        last_run_at, next_run_at, created_at, updated_at
		 FROM schedules ORDER BY name ASC LIMIT ? OFFSET ?`,
		perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("listing schedules: %w", err)
	}
	defer rows.Close()

	var items []Schedule
	for rows.Next() {
		sched, err := scanSchedule(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning schedule row: %w", err)
		}
		items = append(items, sched)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating schedule rows: %w", err)
	}

	if items == nil {
		items = []Schedule{}
	}

	return items, total, nil
}

// ByID returns a single schedule by ID, or nil if not found.
func (s *Service) ByID(ctx context.Context, id string) (*Schedule, error) {
	var sched Schedule
	var desc, cron, action, target, envID, lastRunAt, nextRunAt sql.NullString
	var enabled sql.NullBool

	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, description, cron, action, target, env_id, enabled,
		        last_run_at, next_run_at, created_at, updated_at
		 FROM schedules WHERE id = ?`, id,
	).Scan(&sched.ID, &sched.Name, &desc, &cron, &action, &target, &envID,
		&enabled, &lastRunAt, &nextRunAt, &sched.CreatedAt, &sched.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting schedule %s: %w", id, err)
	}

	applyScheduleNullables(&sched, desc, cron, action, target, envID, enabled, lastRunAt, nextRunAt)

	return &sched, nil
}

// Create inserts a new schedule and returns the created record.
func (s *Service) Create(ctx context.Context, input CreateScheduleInput) (*Schedule, error) {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	var envID interface{}
	if input.EnvID != "" {
		envID = input.EnvID
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO schedules (id, name, description, cron, action, target, env_id, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		id, input.Name, input.Description, input.Cron, input.Action, input.Target, envID, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting schedule: %w", err)
	}

	return &Schedule{
		ID: id, Name: input.Name, Description: input.Description,
		Cron: input.Cron, Action: input.Action, Target: input.Target,
		EnvID: input.EnvID, Enabled: true, CreatedAt: now, UpdatedAt: now,
	}, nil
}

// Update applies partial updates to an existing schedule. Returns the
// updated schedule or nil if the ID was not found.
func (s *Service) Update(ctx context.Context, id string, input UpdateScheduleInput) (*Schedule, error) {
	// Verify existence
	var existsID string
	if err := s.db.QueryRowContext(ctx, "SELECT id FROM schedules WHERE id = ?", id).Scan(&existsID); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("checking schedule existence: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	if input.Name != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE schedules SET name = ?, updated_at = ? WHERE id = ?", *input.Name, now, id); err != nil {
			return nil, fmt.Errorf("updating schedule name: %w", err)
		}
	}
	if input.Description != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE schedules SET description = ?, updated_at = ? WHERE id = ?", *input.Description, now, id); err != nil {
			return nil, fmt.Errorf("updating schedule description: %w", err)
		}
	}
	if input.Cron != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE schedules SET cron = ?, updated_at = ? WHERE id = ?", *input.Cron, now, id); err != nil {
			return nil, fmt.Errorf("updating schedule cron: %w", err)
		}
	}
	if input.Action != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE schedules SET action = ?, updated_at = ? WHERE id = ?", *input.Action, now, id); err != nil {
			return nil, fmt.Errorf("updating schedule action: %w", err)
		}
	}
	if input.Target != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE schedules SET target = ?, updated_at = ? WHERE id = ?", *input.Target, now, id); err != nil {
			return nil, fmt.Errorf("updating schedule target: %w", err)
		}
	}
	if input.EnvID != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE schedules SET env_id = ?, updated_at = ? WHERE id = ?", *input.EnvID, now, id); err != nil {
			return nil, fmt.Errorf("updating schedule env_id: %w", err)
		}
	}
	if input.Enabled != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE schedules SET enabled = ?, updated_at = ? WHERE id = ?", *input.Enabled, now, id); err != nil {
			return nil, fmt.Errorf("updating schedule enabled: %w", err)
		}
	}

	return s.ByID(ctx, id)
}

// Delete removes a schedule by ID. Returns true if a row was deleted.
func (s *Service) Delete(ctx context.Context, id string) (bool, error) {
	result, err := s.db.ExecContext(ctx, "DELETE FROM schedules WHERE id = ?", id)
	if err != nil {
		return false, fmt.Errorf("deleting schedule %s: %w", id, err)
	}

	return db.RowsAffected(result) > 0, nil
}

// ListExecutions returns a paginated list of executions for a schedule
// and the total count.
func (s *Service) ListExecutions(ctx context.Context, scheduleID string, page, perPage int) ([]Execution, int64, error) {
	var total int64
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM schedule_executions WHERE schedule_id = ?", scheduleID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting executions for schedule %s: %w", scheduleID, err)
	}

	offset := (page - 1) * perPage
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, schedule_id, status, output, duration, executed_at
		 FROM schedule_executions WHERE schedule_id = ? ORDER BY executed_at DESC LIMIT ? OFFSET ?`,
		scheduleID, perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("listing executions for schedule %s: %w", scheduleID, err)
	}
	defer rows.Close()

	var items []Execution
	for rows.Next() {
		exec, err := scanExecution(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning execution row: %w", err)
		}
		items = append(items, exec)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating execution rows: %w", err)
	}

	if items == nil {
		items = []Execution{}
	}

	return items, total, nil
}

// scanSchedule scans a Schedule from a sql.Rows iterator.
func scanSchedule(rows *sql.Rows) (Schedule, error) {
	var sched Schedule
	var desc, cron, action, target, envID, lastRunAt, nextRunAt sql.NullString
	var enabled sql.NullBool

	if err := rows.Scan(&sched.ID, &sched.Name, &desc, &cron, &action, &target, &envID,
		&enabled, &lastRunAt, &nextRunAt, &sched.CreatedAt, &sched.UpdatedAt); err != nil {
		return Schedule{}, err
	}

	applyScheduleNullables(&sched, desc, cron, action, target, envID, enabled, lastRunAt, nextRunAt)

	return sched, nil
}

// scanExecution scans an Execution from a sql.Rows iterator.
func scanExecution(rows *sql.Rows) (Execution, error) {
	var e Execution
	var output sql.NullString
	var duration sql.NullInt64

	if err := rows.Scan(&e.ID, &e.ScheduleID, &e.Status, &output, &duration, &e.ExecutedAt); err != nil {
		return Execution{}, err
	}

	e.Output = output.String
	e.Duration = int(duration.Int64)

	return e, nil
}

// applyScheduleNullables maps nullable SQL fields onto the Schedule struct.
func applyScheduleNullables(sched *Schedule, desc, cron, action, target, envID sql.NullString, enabled sql.NullBool, lastRunAt, nextRunAt sql.NullString) {
	sched.Description = desc.String
	sched.Cron = cron.String
	sched.Action = action.String
	sched.Target = target.String
	sched.EnvID = envID.String
	sched.Enabled = enabled.Bool
	sched.LastRunAt = lastRunAt.String
	sched.NextRunAt = nextRunAt.String
}
