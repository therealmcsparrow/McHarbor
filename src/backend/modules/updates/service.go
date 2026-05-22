// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package updates

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// Service handles update policy business logic and database operations.
type Service struct {
	db *sql.DB
}

// NewService creates a new updates service.
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// List returns a paginated list of update policies and the total count.
func (s *Service) List(ctx context.Context, page, perPage int) ([]Policy, int64, error) {
	var total int64
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM update_policies").Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting update policies: %w", err)
	}

	offset := (page - 1) * perPage
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, container_match, image_match, schedule, strategy, auto_restart,
		        enabled, last_run_at, last_run_status, created_at, updated_at
		 FROM update_policies ORDER BY name ASC LIMIT ? OFFSET ?`,
		perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("listing update policies: %w", err)
	}
	defer rows.Close()

	var items []Policy
	for rows.Next() {
		p, err := scanPolicy(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning policy row: %w", err)
		}
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating policy rows: %w", err)
	}

	if items == nil {
		items = []Policy{}
	}

	return items, total, nil
}

// ByID returns a single update policy by ID, or nil if not found.
func (s *Service) ByID(ctx context.Context, id string) (*Policy, error) {
	var p Policy
	var containerMatch, imageMatch, schedule, strategy, lastRunAt, lastRunStatus sql.NullString
	var autoRestart, enabled sql.NullBool

	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, container_match, image_match, schedule, strategy, auto_restart,
		        enabled, last_run_at, last_run_status, created_at, updated_at
		 FROM update_policies WHERE id = ?`, id,
	).Scan(&p.ID, &p.Name, &containerMatch, &imageMatch, &schedule,
		&strategy, &autoRestart, &enabled, &lastRunAt, &lastRunStatus,
		&p.CreatedAt, &p.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting update policy %s: %w", id, err)
	}

	applyPolicyNullables(&p, containerMatch, imageMatch, schedule, strategy, autoRestart, enabled, lastRunAt, lastRunStatus)

	return &p, nil
}

// Create inserts a new update policy.
func (s *Service) Create(ctx context.Context, input CreatePolicyInput) (*Policy, error) {
	if input.Strategy == "" {
		input.Strategy = "latest"
	}

	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO update_policies (id, name, container_match, image_match, schedule, strategy,
		 auto_restart, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		id, input.Name, input.ContainerMatch, input.ImageMatch, input.Schedule,
		input.Strategy, input.AutoRestart, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting update policy: %w", err)
	}

	return &Policy{
		ID: id, Name: input.Name, ContainerMatch: input.ContainerMatch,
		ImageMatch: input.ImageMatch, Schedule: input.Schedule, Strategy: input.Strategy,
		AutoRestart: input.AutoRestart, Enabled: true, CreatedAt: now, UpdatedAt: now,
	}, nil
}

// Update applies partial updates to an existing policy. Returns the updated
// policy or nil if the ID was not found.
func (s *Service) Update(ctx context.Context, id string, input UpdatePolicyInput) (*Policy, error) {
	// Verify existence
	var existsID string
	if err := s.db.QueryRowContext(ctx, "SELECT id FROM update_policies WHERE id = ?", id).Scan(&existsID); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("checking update policy existence: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	if input.Name != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE update_policies SET name = ?, updated_at = ? WHERE id = ?", *input.Name, now, id); err != nil {
			return nil, fmt.Errorf("updating policy name: %w", err)
		}
	}
	if input.ContainerMatch != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE update_policies SET container_match = ?, updated_at = ? WHERE id = ?", *input.ContainerMatch, now, id); err != nil {
			return nil, fmt.Errorf("updating policy container match: %w", err)
		}
	}
	if input.ImageMatch != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE update_policies SET image_match = ?, updated_at = ? WHERE id = ?", *input.ImageMatch, now, id); err != nil {
			return nil, fmt.Errorf("updating policy image match: %w", err)
		}
	}
	if input.Schedule != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE update_policies SET schedule = ?, updated_at = ? WHERE id = ?", *input.Schedule, now, id); err != nil {
			return nil, fmt.Errorf("updating policy schedule: %w", err)
		}
	}
	if input.Strategy != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE update_policies SET strategy = ?, updated_at = ? WHERE id = ?", *input.Strategy, now, id); err != nil {
			return nil, fmt.Errorf("updating policy strategy: %w", err)
		}
	}
	if input.AutoRestart != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE update_policies SET auto_restart = ?, updated_at = ? WHERE id = ?", *input.AutoRestart, now, id); err != nil {
			return nil, fmt.Errorf("updating policy auto restart: %w", err)
		}
	}
	if input.Enabled != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE update_policies SET enabled = ?, updated_at = ? WHERE id = ?", *input.Enabled, now, id); err != nil {
			return nil, fmt.Errorf("updating policy enabled: %w", err)
		}
	}

	return s.ByID(ctx, id)
}

// Delete removes an update policy by ID. Returns true if a row was deleted.
func (s *Service) Delete(ctx context.Context, id string) (bool, error) {
	result, err := s.db.ExecContext(ctx, "DELETE FROM update_policies WHERE id = ?", id)
	if err != nil {
		return false, fmt.Errorf("deleting update policy %s: %w", id, err)
	}

	return db.RowsAffected(result) > 0, nil
}

// History returns a paginated list of update execution records for a policy.
func (s *Service) History(ctx context.Context, policyID string, page, perPage int) ([]UpdateHistory, int64, error) {
	var total int64
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM update_history WHERE policy_id = ?", policyID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting update history for policy %s: %w", policyID, err)
	}

	offset := (page - 1) * perPage
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, policy_id, container, old_image, new_image, status, message, executed_at
		 FROM update_history WHERE policy_id = ? ORDER BY executed_at DESC LIMIT ? OFFSET ?`,
		policyID, perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("listing update history for policy %s: %w", policyID, err)
	}
	defer rows.Close()

	var items []UpdateHistory
	for rows.Next() {
		uh, err := scanHistory(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning history row: %w", err)
		}
		items = append(items, uh)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating history rows: %w", err)
	}

	if items == nil {
		items = []UpdateHistory{}
	}

	return items, total, nil
}

// scanPolicy scans a Policy from a sql.Rows iterator.
func scanPolicy(rows *sql.Rows) (Policy, error) {
	var p Policy
	var containerMatch, imageMatch, schedule, strategy, lastRunAt, lastRunStatus sql.NullString
	var autoRestart, enabled sql.NullBool

	if err := rows.Scan(&p.ID, &p.Name, &containerMatch, &imageMatch, &schedule,
		&strategy, &autoRestart, &enabled, &lastRunAt, &lastRunStatus,
		&p.CreatedAt, &p.UpdatedAt); err != nil {
		return Policy{}, err
	}

	applyPolicyNullables(&p, containerMatch, imageMatch, schedule, strategy, autoRestart, enabled, lastRunAt, lastRunStatus)

	return p, nil
}

// scanHistory scans an UpdateHistory from a sql.Rows iterator.
func scanHistory(rows *sql.Rows) (UpdateHistory, error) {
	var uh UpdateHistory
	var oldImage, newImage, message sql.NullString

	if err := rows.Scan(&uh.ID, &uh.PolicyID, &uh.Container, &oldImage, &newImage,
		&uh.Status, &message, &uh.ExecutedAt); err != nil {
		return UpdateHistory{}, err
	}

	uh.OldImage = oldImage.String
	uh.NewImage = newImage.String
	uh.Message = message.String

	return uh, nil
}

// applyPolicyNullables maps nullable SQL fields onto the Policy struct.
func applyPolicyNullables(p *Policy, containerMatch, imageMatch, schedule, strategy sql.NullString, autoRestart, enabled sql.NullBool, lastRunAt, lastRunStatus sql.NullString) {
	p.ContainerMatch = containerMatch.String
	p.ImageMatch = imageMatch.String
	p.Schedule = schedule.String
	p.Strategy = strategy.String
	p.AutoRestart = autoRestart.Bool
	p.Enabled = enabled.Bool
	p.LastRunAt = lastRunAt.String
	p.LastRunStatus = lastRunStatus.String
}
