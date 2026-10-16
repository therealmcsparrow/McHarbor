// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package inapp

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestCreateBroadcastSkipsIncompleteRecords(t *testing.T) {
	db := openInAppTestDB(t)

	if err := CreateBroadcast(db, Record{Message: "message only"}); err != nil {
		t.Fatalf("CreateBroadcast title missing returned error: %v", err)
	}
	if err := CreateBroadcast(db, Record{Title: "title only"}); err != nil {
		t.Fatalf("CreateBroadcast message missing returned error: %v", err)
	}

	if countRows(t, db) != 0 {
		t.Fatal("incomplete records should not be inserted")
	}
}

func TestCreateBroadcastInsertsNotification(t *testing.T) {
	db := openInAppTestDB(t)

	err := CreateBroadcast(db, Record{
		Title:      "Deploy finished",
		Message:    "Stack is running",
		Action:     "created",
		EntityType: "stack",
		EntityID:   "stack-1",
	})
	if err != nil {
		t.Fatalf("CreateBroadcast returned error: %v", err)
	}

	var level, title, message, action, entityType, entityID, createdAt, updatedAt string
	if err := db.QueryRow(`SELECT level, title, message, action, entity_type, entity_id, created_at, updated_at FROM in_app_notifications`).Scan(
		&level, &title, &message, &action, &entityType, &entityID, &createdAt, &updatedAt,
	); err != nil {
		t.Fatalf("scan inserted notification: %v", err)
	}

	if level != "info" || title != "Deploy finished" || message != "Stack is running" {
		t.Fatalf("inserted values = %q %q %q", level, title, message)
	}
	if action != "created" || entityType != "stack" || entityID != "stack-1" {
		t.Fatalf("inserted entity values = %q %q %q", action, entityType, entityID)
	}
	if createdAt == "" || updatedAt == "" {
		t.Fatalf("timestamps should be set: created=%q updated=%q", createdAt, updatedAt)
	}
}

func TestCreateBroadcastPreservesCustomLevelAndNullsEmptyOptionalFields(t *testing.T) {
	db := openInAppTestDB(t)

	err := CreateBroadcast(db, Record{
		Level:   "warning",
		Title:   "Warning",
		Message: "Something changed",
	})
	if err != nil {
		t.Fatalf("CreateBroadcast returned error: %v", err)
	}

	var level string
	var action, entityType, entityID sql.NullString
	if err := db.QueryRow(`SELECT level, action, entity_type, entity_id FROM in_app_notifications`).Scan(&level, &action, &entityType, &entityID); err != nil {
		t.Fatalf("scan inserted notification: %v", err)
	}

	if level != "warning" {
		t.Fatalf("level = %q, want warning", level)
	}
	if action.Valid || entityType.Valid || entityID.Valid {
		t.Fatalf("optional fields should be NULL: %#v %#v %#v", action, entityType, entityID)
	}
}

func TestCreateBroadcastReturnsInsertError(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing database: %v", err)
		}
	})

	err = CreateBroadcast(db, Record{Title: "Title", Message: "Message"})
	if err == nil {
		t.Fatal("CreateBroadcast error = nil, want insert error")
	}
}

func TestNullIfEmpty(t *testing.T) {
	if got := nullIfEmpty(""); got != nil {
		t.Fatalf("nullIfEmpty(empty) = %#v, want nil", got)
	}
	if got := nullIfEmpty("value"); got != "value" {
		t.Fatalf("nullIfEmpty(value) = %#v, want value", got)
	}
}

func openInAppTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing database: %v", err)
		}
	})

	_, err = db.Exec(`
		CREATE TABLE in_app_notifications (
			id TEXT PRIMARY KEY,
			level TEXT NOT NULL,
			title TEXT NOT NULL,
			message TEXT NOT NULL,
			action TEXT,
			entity_type TEXT,
			entity_id TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("create in_app_notifications: %v", err)
	}

	return db
}

func countRows(t *testing.T, db *sql.DB) int {
	t.Helper()

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM in_app_notifications`).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return count
}
