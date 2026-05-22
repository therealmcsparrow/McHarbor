// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package webhooks

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
)

// Webhook represents a webhook configuration stored in the database.
type Webhook struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	Secret    string `json:"secret,omitempty"` // HMAC secret for signature verification
	Events    string `json:"events"`           // JSON array of event types to trigger on
	IsActive  bool   `json:"isActive"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// Delivery represents a single webhook delivery attempt.
type Delivery struct {
	ID             string `json:"id"`
	WebhookID      string `json:"webhookId"`
	Event          string `json:"event"`
	Payload        string `json:"payload"`
	ResponseStatus int    `json:"responseStatus"`
	ResponseBody   string `json:"responseBody"`
	Success        bool   `json:"success"`
	Duration       int    `json:"duration"` // milliseconds
	CreatedAt      string `json:"createdAt"`
}

// CreateWebhookInput is the request body for creating a webhook.
type CreateWebhookInput struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Secret string `json:"secret"`
	Events string `json:"events"`
}

// UpdateWebhookInput is the request body for updating a webhook.
type UpdateWebhookInput struct {
	Name     *string `json:"name"`
	URL      *string `json:"url"`
	Secret   *string `json:"secret"`
	Events   *string `json:"events"`
	IsActive *bool   `json:"isActive"`
}

// Service handles webhook database operations.
type Service struct {
	db  *sql.DB
	enc *encryption.Service
}

// NewService creates a new webhooks service.
func NewService(db *sql.DB, enc *encryption.Service) *Service {
	return &Service{db: db, enc: enc}
}

// List returns all webhooks with pagination.
func (s *Service) List(page, perPage int) ([]Webhook, int64, error) {
	var total int64
	err := s.db.QueryRow("SELECT COUNT(*) FROM webhooks").Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting webhooks: %w", err)
	}

	offset := (page - 1) * perPage
	rows, err := s.db.Query(
		`SELECT id, name, url, secret, events, is_active, created_at, updated_at
		 FROM webhooks ORDER BY name ASC LIMIT ? OFFSET ?`,
		perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("querying webhooks: %w", err)
	}
	defer rows.Close()

	var items []Webhook
	for rows.Next() {
		var wh Webhook
		var secret, events sql.NullString
		var isActive sql.NullBool
		if err := rows.Scan(&wh.ID, &wh.Name, &wh.URL, &secret, &events, &isActive,
			&wh.CreatedAt, &wh.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning webhook: %w", err)
		}
		if secret.String != "" {
			decrypted, err := s.enc.Decrypt(secret.String)
			if err != nil {
				slog.Error("webhooks: failed to decrypt secret", "error", err, "id", wh.ID)
				wh.Secret = ""
			} else {
				wh.Secret = decrypted
			}
		}
		wh.Events = events.String
		wh.IsActive = isActive.Bool
		items = append(items, wh)
	}

	if items == nil {
		items = []Webhook{}
	}

	return items, total, nil
}

// ByID returns a single webhook.
func (s *Service) ByID(id string) (*Webhook, error) {
	var wh Webhook
	var secret, events sql.NullString
	var isActive sql.NullBool

	err := s.db.QueryRow(
		`SELECT id, name, url, secret, events, is_active, created_at, updated_at
		 FROM webhooks WHERE id = ?`, id,
	).Scan(&wh.ID, &wh.Name, &wh.URL, &secret, &events, &isActive, &wh.CreatedAt, &wh.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying webhook: %w", err)
	}

	if secret.String != "" {
		decrypted, err := s.enc.Decrypt(secret.String)
		if err != nil {
			slog.Error("webhooks: failed to decrypt secret", "error", err, "id", wh.ID)
			wh.Secret = ""
		} else {
			wh.Secret = decrypted
		}
	}
	wh.Events = events.String
	wh.IsActive = isActive.Bool

	return &wh, nil
}

// Create inserts a new webhook.
func (s *Service) Create(input CreateWebhookInput) (*Webhook, error) {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	// Encrypt secret before storing
	secret := input.Secret
	if secret != "" {
		encrypted, err := s.enc.Encrypt(secret)
		if err != nil {
			return nil, fmt.Errorf("encrypting webhook secret: %w", err)
		}
		secret = encrypted
	}

	_, err := s.db.Exec(
		`INSERT INTO webhooks (id, name, url, secret, events, is_active, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 1, ?, ?)`,
		id, input.Name, input.URL, secret, input.Events, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting webhook: %w", err)
	}

	return s.ByID(id)
}

// Update modifies an existing webhook.
func (s *Service) Update(id string, input UpdateWebhookInput) (*Webhook, error) {
	existing, err := s.ByID(id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, nil
	}

	now := time.Now().UTC().Format(time.RFC3339)

	name := existing.Name
	if input.Name != nil {
		name = *input.Name
	}
	url := existing.URL
	if input.URL != nil {
		url = *input.URL
	}
	secret := existing.Secret
	if input.Secret != nil {
		secret = *input.Secret
	}
	// Encrypt secret before storing
	encryptedSecret := secret
	if secret != "" {
		enc, encErr := s.enc.Encrypt(secret)
		if encErr != nil {
			return nil, fmt.Errorf("encrypting webhook secret: %w", encErr)
		}
		encryptedSecret = enc
	}
	events := existing.Events
	if input.Events != nil {
		events = *input.Events
	}
	isActive := existing.IsActive
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	_, err = s.db.Exec(
		`UPDATE webhooks SET name = ?, url = ?, secret = ?, events = ?, is_active = ?, updated_at = ?
		 WHERE id = ?`,
		name, url, encryptedSecret, events, isActive, now, id,
	)
	if err != nil {
		return nil, fmt.Errorf("updating webhook: %w", err)
	}

	return s.ByID(id)
}

// Delete removes a webhook.
func (s *Service) Delete(id string) error {
	result, err := s.db.Exec("DELETE FROM webhooks WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting webhook: %w", err)
	}
	if db.RowsAffected(result) == 0 {
		return fmt.Errorf("webhook not found")
	}
	return nil
}

// ListDeliveries returns deliveries for a webhook.
func (s *Service) ListDeliveries(webhookID string, page, perPage int) ([]Delivery, int64, error) {
	var total int64
	err := s.db.QueryRow("SELECT COUNT(*) FROM webhook_deliveries WHERE webhook_id = ?", webhookID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting deliveries: %w", err)
	}

	offset := (page - 1) * perPage
	rows, err := s.db.Query(
		`SELECT id, webhook_id, event, payload, response_status, response_body, success, duration, created_at
		 FROM webhook_deliveries WHERE webhook_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		webhookID, perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("querying deliveries: %w", err)
	}
	defer rows.Close()

	var items []Delivery
	for rows.Next() {
		var d Delivery
		var payload, respBody sql.NullString
		var respStatus, duration sql.NullInt64
		var success sql.NullBool
		if err := rows.Scan(&d.ID, &d.WebhookID, &d.Event, &payload, &respStatus, &respBody,
			&success, &duration, &d.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning delivery: %w", err)
		}
		d.Payload = payload.String
		d.ResponseStatus = int(respStatus.Int64)
		d.ResponseBody = respBody.String
		d.Success = success.Bool
		d.Duration = int(duration.Int64)
		items = append(items, d)
	}

	if items == nil {
		items = []Delivery{}
	}

	return items, total, nil
}

// RecordDelivery records a webhook delivery attempt.
func (s *Service) RecordDelivery(webhookID, event, payload string, respStatus int, respBody string, success bool, duration int) error {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.Exec(
		`INSERT INTO webhook_deliveries (id, webhook_id, event, payload, response_status, response_body, success, duration, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, webhookID, event, payload, respStatus, respBody, success, duration, now,
	)
	return err
}
