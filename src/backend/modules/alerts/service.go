// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// Service handles alert rule business logic and database operations.
type Service struct {
	db *sql.DB
}

var errAlertDestinationRequired = errors.New("alert destination required")

// NewService creates a new alerts service.
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// List returns a paginated list of alert rules and the total count.
func (s *Service) List(ctx context.Context, page, perPage int) ([]Alert, int64, error) {
	var total int64
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM alerts").Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting alerts: %w", err)
	}

	offset := (page - 1) * perPage
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, severity, type, condition, target, channel_id, send_in_app, enabled, created_at, updated_at
		 FROM alerts ORDER BY name ASC LIMIT ? OFFSET ?`,
		perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("listing alerts: %w", err)
	}
	defer rows.Close()

	var items []Alert
	for rows.Next() {
		a, err := scanAlert(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning alert row: %w", err)
		}
		items = append(items, a)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating alert rows: %w", err)
	}

	if items == nil {
		items = []Alert{}
	}

	return items, total, nil
}

// ListEnabled returns all enabled alert rules with a safety limit for background evaluation.
func (s *Service) ListEnabled(ctx context.Context) ([]Alert, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, severity, type, condition, target, channel_id, send_in_app, enabled, created_at, updated_at
		 FROM alerts
		 WHERE enabled = 1
		 ORDER BY name ASC
		 LIMIT 1000`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing enabled alerts: %w", err)
	}
	defer rows.Close()

	items := make([]Alert, 0, 32)
	for rows.Next() {
		a, err := scanAlert(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning enabled alert row: %w", err)
		}
		items = append(items, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating enabled alert rows: %w", err)
	}

	return items, nil
}

// Create inserts a new alert rule. Returns the created alert.
func (s *Service) Create(ctx context.Context, input CreateAlertInput) (*Alert, error) {
	if !hasAlertDestination(input.ChannelID, input.SendInApp) {
		return nil, errAlertDestinationRequired
	}

	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	severity := input.Severity
	if severity == "" {
		severity = "warning"
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO alerts (id, name, severity, type, condition, target, channel_id, send_in_app, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		id, input.Name, severity, input.Type, input.Condition, input.Target, input.ChannelID, input.SendInApp, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting alert: %w", err)
	}

	return &Alert{
		ID:        id,
		Name:      input.Name,
		Severity:  severity,
		Type:      input.Type,
		Condition: input.Condition,
		Target:    input.Target,
		ChannelID: input.ChannelID,
		SendInApp: input.SendInApp,
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// Update applies partial updates to an alert rule.
func (s *Service) Update(ctx context.Context, id string, input UpdateAlertInput) (*Alert, error) {
	current, err := s.byID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("checking alert existence: %w", err)
	}
	if current == nil {
		return nil, nil
	}

	channelID := current.ChannelID
	if input.ChannelID != nil {
		channelID = *input.ChannelID
	}

	sendInApp := current.SendInApp
	if input.SendInApp != nil {
		sendInApp = *input.SendInApp
	}

	if !hasAlertDestination(channelID, sendInApp) {
		return nil, errAlertDestinationRequired
	}

	now := time.Now().UTC().Format(time.RFC3339)

	if input.Name != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE alerts SET name = ?, updated_at = ? WHERE id = ?", *input.Name, now, id); err != nil {
			return nil, fmt.Errorf("updating alert name: %w", err)
		}
	}
	if input.Severity != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE alerts SET severity = ?, updated_at = ? WHERE id = ?", *input.Severity, now, id); err != nil {
			return nil, fmt.Errorf("updating alert severity: %w", err)
		}
	}
	if input.Type != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE alerts SET type = ?, updated_at = ? WHERE id = ?", *input.Type, now, id); err != nil {
			return nil, fmt.Errorf("updating alert type: %w", err)
		}
	}
	if input.Condition != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE alerts SET condition = ?, updated_at = ? WHERE id = ?", *input.Condition, now, id); err != nil {
			return nil, fmt.Errorf("updating alert condition: %w", err)
		}
	}
	if input.Target != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE alerts SET target = ?, updated_at = ? WHERE id = ?", *input.Target, now, id); err != nil {
			return nil, fmt.Errorf("updating alert target: %w", err)
		}
	}
	if input.ChannelID != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE alerts SET channel_id = ?, updated_at = ? WHERE id = ?", *input.ChannelID, now, id); err != nil {
			return nil, fmt.Errorf("updating alert channel: %w", err)
		}
	}
	if input.SendInApp != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE alerts SET send_in_app = ?, updated_at = ? WHERE id = ?", *input.SendInApp, now, id); err != nil {
			return nil, fmt.Errorf("updating alert in-app delivery flag: %w", err)
		}
	}
	if input.Enabled != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE alerts SET enabled = ?, updated_at = ? WHERE id = ?", *input.Enabled, now, id); err != nil {
			return nil, fmt.Errorf("updating alert enabled flag: %w", err)
		}
	}

	return s.byID(ctx, id)
}

// Delete removes an alert rule by ID. Returns true if a row was deleted.
func (s *Service) Delete(ctx context.Context, id string) (bool, error) {
	result, err := s.db.ExecContext(ctx, "DELETE FROM alerts WHERE id = ?", id)
	if err != nil {
		return false, fmt.Errorf("deleting alert %s: %w", id, err)
	}

	return db.RowsAffected(result) > 0, nil
}

func (s *Service) byID(ctx context.Context, id string) (*Alert, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, severity, type, condition, target, channel_id, send_in_app, enabled, created_at, updated_at
		 FROM alerts WHERE id = ?`,
		id,
	)

	alert, err := scanAlertRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting alert %s: %w", id, err)
	}

	return alert, nil
}

// scanAlert scans a single Alert from a sql.Rows iterator.
func scanAlert(rows *sql.Rows) (Alert, error) {
	alert, err := scanAlertRow(rows)
	if err != nil {
		return Alert{}, err
	}
	return *alert, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAlertRow(rows rowScanner) (*Alert, error) {
	var a Alert
	var severity, condition, target, channelID sql.NullString
	var sendInApp sql.NullBool
	var enabled sql.NullBool

	if err := rows.Scan(&a.ID, &a.Name, &severity, &a.Type, &condition, &target, &channelID, &sendInApp,
		&enabled, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return nil, err
	}

	a.Severity = severity.String
	if a.Severity == "" {
		a.Severity = "warning"
	}
	a.Condition = condition.String
	a.Target = target.String
	a.ChannelID = channelID.String
	a.SendInApp = sendInApp.Bool
	a.Enabled = enabled.Bool

	return &a, nil
}

func hasAlertDestination(channelID string, sendInApp bool) bool {
	return strings.TrimSpace(channelID) != "" || sendInApp
}
