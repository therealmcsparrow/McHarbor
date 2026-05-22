// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package inapp

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/rs/xid"
)

// Record represents a broadcast in-app notification entry.
type Record struct {
	Level      string
	Title      string
	Message    string
	Action     string
	EntityType string
	EntityID   string
}

// CreateBroadcast inserts a broadcast notification visible to all users.
func CreateBroadcast(db *sql.DB, record Record) error {
	if record.Title == "" || record.Message == "" {
		return nil
	}

	level := record.Level
	if level == "" {
		level = "info"
	}

	now := time.Now().UTC().Format(time.RFC3339)

	if _, err := db.Exec(
		`INSERT INTO in_app_notifications
		 (id, level, title, message, action, entity_type, entity_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		xid.New().String(),
		level,
		record.Title,
		record.Message,
		nullIfEmpty(record.Action),
		nullIfEmpty(record.EntityType),
		nullIfEmpty(record.EntityID),
		now,
		now,
	); err != nil {
		return fmt.Errorf("inserting in-app notification: %w", err)
	}

	return nil
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}

	return value
}
