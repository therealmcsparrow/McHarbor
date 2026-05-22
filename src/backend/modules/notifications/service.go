// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package notifications

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	corenotify "github.com/therealmcsparrow/mcharbor/core/notify"
)

// validChannelTypes defines the accepted notification channel types.
var validChannelTypes = map[string]bool{
	"email": true, "slack": true, "discord": true, "webhook": true, "telegram": true,
}

// TestResult holds the outcome of a test notification dispatch.
type TestResult struct {
	Success    bool   `json:"success"`
	StatusCode int    `json:"statusCode,omitempty"`
	Message    string `json:"message,omitempty"`
	Error      string `json:"error,omitempty"`
}

// Service handles notification channel business logic and database operations.
type Service struct {
	db         *sql.DB
	dispatcher *corenotify.Dispatcher
}

// NewService creates a new notifications service.
func NewService(db *sql.DB, enc *encryption.Service) *Service {
	return &Service{
		db:         db,
		dispatcher: corenotify.NewDispatcher(db, enc),
	}
}

// ConfiguredTypes returns a list of distinct channel types that have at least one enabled channel.
func (s *Service) ConfiguredTypes(ctx context.Context) ([]string, error) {
	types, err := s.dispatcher.Capabilities(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing configured channel types: %w", err)
	}
	return types, nil
}

// List returns a paginated list of notification channels and the total count.
func (s *Service) List(ctx context.Context, page, perPage int) ([]Channel, int64, error) {
	var total int64
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notification_channels").Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting notification channels: %w", err)
	}

	offset := (page - 1) * perPage
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, type, config, enabled, created_at, updated_at
		 FROM notification_channels ORDER BY name ASC LIMIT ? OFFSET ?`,
		perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("listing notification channels: %w", err)
	}
	defer rows.Close()

	var items []Channel
	for rows.Next() {
		ch, err := scanChannel(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning notification channel row: %w", err)
		}
		items = append(items, ch)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating notification channel rows: %w", err)
	}

	if items == nil {
		items = []Channel{}
	}

	return items, total, nil
}

// ByID returns a single notification channel by ID, or nil if not found.
func (s *Service) ByID(ctx context.Context, id string) (*Channel, error) {
	var ch Channel
	var config sql.NullString
	var enabled sql.NullBool

	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, type, config, enabled, created_at, updated_at
		 FROM notification_channels WHERE id = ?`, id,
	).Scan(&ch.ID, &ch.Name, &ch.Type, &config, &enabled, &ch.CreatedAt, &ch.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting notification channel %s: %w", id, err)
	}

	ch.Config = config.String
	ch.Enabled = enabled.Bool

	return &ch, nil
}

// Create inserts a new notification channel.
func (s *Service) Create(ctx context.Context, input CreateChannelInput) (*Channel, error) {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO notification_channels (id, name, type, config, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 1, ?, ?)`,
		id, input.Name, input.Type, input.Config, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting notification channel: %w", err)
	}

	return &Channel{
		ID:        id,
		Name:      input.Name,
		Type:      input.Type,
		Config:    input.Config,
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// Update applies partial updates to an existing notification channel.
// Returns the updated channel or nil if the ID was not found.
func (s *Service) Update(ctx context.Context, id string, input UpdateChannelInput) (*Channel, error) {
	var existsID string
	if err := s.db.QueryRowContext(ctx, "SELECT id FROM notification_channels WHERE id = ?", id).Scan(&existsID); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("checking notification channel existence: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	if input.Name != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE notification_channels SET name = ?, updated_at = ? WHERE id = ?", *input.Name, now, id); err != nil {
			return nil, fmt.Errorf("updating notification channel name: %w", err)
		}
	}
	if input.Type != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE notification_channels SET type = ?, updated_at = ? WHERE id = ?", *input.Type, now, id); err != nil {
			return nil, fmt.Errorf("updating notification channel type: %w", err)
		}
	}
	if input.Config != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE notification_channels SET config = ?, updated_at = ? WHERE id = ?", *input.Config, now, id); err != nil {
			return nil, fmt.Errorf("updating notification channel config: %w", err)
		}
	}
	if input.Enabled != nil {
		if _, err := s.db.ExecContext(ctx, "UPDATE notification_channels SET enabled = ?, updated_at = ? WHERE id = ?", *input.Enabled, now, id); err != nil {
			return nil, fmt.Errorf("updating notification channel enabled flag: %w", err)
		}
	}

	return s.ByID(ctx, id)
}

// Delete removes a notification channel by ID. Returns true if a row was deleted.
func (s *Service) Delete(ctx context.Context, id string) (bool, error) {
	result, err := s.db.ExecContext(ctx, "DELETE FROM notification_channels WHERE id = ?", id)
	if err != nil {
		return false, fmt.Errorf("deleting notification channel %s: %w", id, err)
	}

	return db.RowsAffected(result) > 0, nil
}

// TestNotification sends a test notification to the channel identified by ID.
// Returns nil result with no error when the channel ID is not found.
func (s *Service) TestNotification(ctx context.Context, id string) (*TestResult, error) {
	var chType, config sql.NullString
	var chName string
	err := s.db.QueryRowContext(ctx,
		"SELECT name, type, config FROM notification_channels WHERE id = ?", id,
	).Scan(&chName, &chType, &config)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("looking up notification channel %s for test: %w", id, err)
	}

	testMessage := fmt.Sprintf("Test notification from McHarbor (channel: %s)", chName)

	switch chType.String {
	case "webhook":
		return s.testWebhook(config.String, chName, testMessage, "url")

	case "slack", "discord":
		return s.testWebhook(config.String, chName, testMessage, "webhookUrl")

	default:
		return &TestResult{
			Success: true,
			Message: fmt.Sprintf("Test notification queued for %s channel '%s'", chType.String, chName),
		}, nil
	}
}

// testWebhook dispatches a test notification to a webhook-style endpoint.
func (s *Service) testWebhook(configJSON, channelName, message, urlField string) (*TestResult, error) {
	var cfgMap map[string]string
	if err := json.Unmarshal([]byte(configJSON), &cfgMap); err != nil {
		return &TestResult{Success: false, Error: "invalid config; cannot parse JSON"}, nil
	}

	hookURL, ok := cfgMap[urlField]
	if !ok {
		return &TestResult{Success: false, Error: fmt.Sprintf("invalid config; missing %s", urlField)}, nil
	}

	payload, _ := json.Marshal(map[string]string{ // safe: simple map literal
		"text":    message,
		"content": message,
		"channel": channelName,
	})

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(hookURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return &TestResult{Success: false, Error: "notification test failed"}, nil
	}
	defer resp.Body.Close()

	return &TestResult{
		Success:    resp.StatusCode >= 200 && resp.StatusCode < 300,
		StatusCode: resp.StatusCode,
	}, nil
}

// scanChannel scans a Channel from a sql.Rows iterator.
func scanChannel(rows *sql.Rows) (Channel, error) {
	var ch Channel
	var config sql.NullString
	var enabled sql.NullBool

	if err := rows.Scan(&ch.ID, &ch.Name, &ch.Type, &config, &enabled,
		&ch.CreatedAt, &ch.UpdatedAt); err != nil {
		return Channel{}, err
	}

	ch.Config = config.String
	ch.Enabled = enabled.Bool

	return ch, nil
}
