// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package inappnotifications

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestListForUserIncludesReadState(t *testing.T) {
	db := openInAppNotificationsTestDB(t)
	createInAppNotificationTables(t, db)

	svc := NewService(db)
	seedNotification(t, db, "notif-1", "success", "Container started", "nginx was started", "2026-03-16T09:00:00Z")
	seedNotification(t, db, "notif-2", "warning", "Alert deleted", "cpu alert was deleted", "2026-03-16T10:00:00Z")
	seedRead(t, db, "read-1", "notif-1", "user-1")

	items, total, err := svc.ListForUser(context.Background(), "user-1", 1, 10, ListFilter{})
	if err != nil {
		t.Fatalf("ListForUser returned error: %v", err)
	}

	if total != 2 {
		t.Fatalf("expected total 2, got %d", total)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].ID != "notif-2" {
		t.Fatalf("expected unread notification notif-2 first, got %s", items[0].ID)
	}
	if items[0].Read {
		t.Fatalf("expected first item to be unread")
	}
	if items[1].ID != "notif-1" {
		t.Fatalf("expected read notification notif-1 second, got %s", items[1].ID)
	}
	if !items[1].Read {
		t.Fatalf("expected second item to be read")
	}
}

func TestMarkReadAndMarkAllRead(t *testing.T) {
	db := openInAppNotificationsTestDB(t)
	createInAppNotificationTables(t, db)

	svc := NewService(db)
	seedNotification(t, db, "notif-1", "success", "Container started", "nginx was started", "2026-03-16T09:00:00Z")
	seedNotification(t, db, "notif-2", "success", "Container restarted", "nginx was restarted", "2026-03-16T10:00:00Z")

	count, err := svc.UnreadCount(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("UnreadCount returned error: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected unread count 2, got %d", count)
	}

	marked, err := svc.MarkRead(context.Background(), "notif-1", "user-1")
	if err != nil {
		t.Fatalf("MarkRead returned error: %v", err)
	}
	if !marked {
		t.Fatal("expected MarkRead to mark existing notification")
	}

	count, err = svc.UnreadCount(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("UnreadCount after MarkRead returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected unread count 1 after MarkRead, got %d", count)
	}

	markedCount, err := svc.MarkAllRead(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("MarkAllRead returned error: %v", err)
	}
	if markedCount != 1 {
		t.Fatalf("expected MarkAllRead to mark 1 notification, got %d", markedCount)
	}

	count, err = svc.UnreadCount(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("UnreadCount after MarkAllRead returned error: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected unread count 0 after MarkAllRead, got %d", count)
	}
}

func TestDeleteForUserHidesNotificationFromListAndUnreadCount(t *testing.T) {
	db := openInAppNotificationsTestDB(t)
	createInAppNotificationTables(t, db)

	svc := NewService(db)
	seedNotification(t, db, "notif-1", "success", "Container started", "nginx was started", "2026-03-16T09:00:00Z")
	seedNotification(t, db, "notif-2", "warning", "Alert deleted", "cpu alert was deleted", "2026-03-16T10:00:00Z")

	deleted, err := svc.DeleteForUser(context.Background(), "notif-2", "user-1")
	if err != nil {
		t.Fatalf("DeleteForUser returned error: %v", err)
	}
	if !deleted {
		t.Fatal("expected DeleteForUser to hide existing notification")
	}

	items, total, err := svc.ListForUser(context.Background(), "user-1", 1, 10, ListFilter{})
	if err != nil {
		t.Fatalf("ListForUser after delete returned error: %v", err)
	}

	if total != 1 {
		t.Fatalf("expected total 1 after delete, got %d", total)
	}
	if len(items) != 1 || items[0].ID != "notif-1" {
		t.Fatalf("expected only notif-1 after delete, got %#v", items)
	}

	count, err := svc.UnreadCount(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("UnreadCount after delete returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected unread count 1 after delete, got %d", count)
	}
}

func openInAppNotificationsTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "in-app-notifications-test.db")
	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatalf("sql.Open returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing db: %v", err)
		}
	})

	return db
}

func createInAppNotificationTables(t *testing.T, db *sql.DB) {
	t.Helper()

	if _, err := db.Exec(`
CREATE TABLE users (
	id TEXT PRIMARY KEY,
	username TEXT NOT NULL
);

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
);

CREATE TABLE in_app_notification_reads (
	id TEXT PRIMARY KEY,
	notification_id TEXT NOT NULL REFERENCES in_app_notifications(id) ON DELETE CASCADE,
	user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	read_at TEXT NOT NULL,
	deleted_at TEXT,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_in_app_notification_reads_unique
	ON in_app_notification_reads(notification_id, user_id);
`); err != nil {
		t.Fatalf("creating in-app notification tables: %v", err)
	}

	if _, err := db.Exec(`INSERT INTO users (id, username) VALUES ('user-1', 'carlo')`); err != nil {
		t.Fatalf("seeding users table: %v", err)
	}
}

func seedNotification(t *testing.T, db *sql.DB, id, level, title, message, createdAt string) {
	t.Helper()

	if _, err := db.Exec(
		`INSERT INTO in_app_notifications
		 (id, level, title, message, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id,
		level,
		title,
		message,
		createdAt,
		createdAt,
	); err != nil {
		t.Fatalf("seeding in_app_notifications: %v", err)
	}
}

func seedRead(t *testing.T, db *sql.DB, id, notificationID, userID string) {
	t.Helper()

	if _, err := db.Exec(
		`INSERT INTO in_app_notification_reads
		 (id, notification_id, user_id, read_at, deleted_at, created_at, updated_at)
		 VALUES (?, ?, ?, '2026-03-16T10:00:00Z', NULL, '2026-03-16T10:00:00Z', '2026-03-16T10:00:00Z')`,
		id,
		notificationID,
		userID,
	); err != nil {
		t.Fatalf("seeding in_app_notification_reads: %v", err)
	}
}

// TestListForUserFilters guards the page's search and chips: text, level and read
// state are applied on the server, across every page, in all three match modes.
func TestListForUserFilters(t *testing.T) {
	db := openInAppNotificationsTestDB(t)
	createInAppNotificationTables(t, db)
	svc := NewService(db)
	seedNotification(t, db, "n1", "success", "Container started", "nginx was started", "2026-03-16T09:00:00Z")
	seedNotification(t, db, "n2", "warning", "Alert deleted", "cpu alert was deleted", "2026-03-16T10:00:00Z")
	seedNotification(t, db, "n3", "error", "Backup failed", "100% of volume db_data unreadable", "2026-03-16T11:00:00Z")
	seedRead(t, db, "r1", "n1", "user-1")
	ctx := context.Background()

	ids := func(f ListFilter) []string {
		items, total, err := svc.ListForUser(ctx, "user-1", 1, 10, f)
		if err != nil {
			t.Fatalf("%+v: %v", f, err)
		}
		if int(total) != len(items) {
			t.Fatalf("%+v: total %d for %d items", f, total, len(items))
		}
		out := []string{}
		for _, n := range items {
			out = append(out, n.ID)
		}
		return out
	}
	cases := []struct {
		f    ListFilter
		want string
	}{
		{ListFilter{Query: "NGINX"}, "n1"},
		{ListFilter{Query: "100%"}, "n3"},
		{ListFilter{Query: "db_d"}, "n3"},
		{ListFilter{Query: "_"}, "n3"},
		{ListFilter{Query: "alert deleted", Mode: "exact"}, "n2"},
		{ListFilter{Query: "alert", Mode: "exact"}, ""},
		{ListFilter{Query: "^backup", Mode: "regex"}, "n3"},
		{ListFilter{Levels: []string{"warning", "error"}}, "n2,n3"},
		{ListFilter{Status: "read"}, "n1"},
		{ListFilter{Status: "unread", Levels: []string{"error"}}, "n3"},
	}
	for _, c := range cases {
		got := ids(c.f)
		joined := ""
		for i, id := range got {
			if i > 0 {
				joined += ","
			}
			joined += id
		}
		if c.want == "n2,n3" && (joined == "n3,n2") {
			joined = "n2,n3"
		}
		if joined != c.want {
			t.Errorf("%+v = %q, want %q", c.f, joined, c.want)
		}
	}
	if _, _, err := svc.ListForUser(ctx, "user-1", 1, 10, ListFilter{Query: "(", Mode: "regex"}); !errors.Is(err, ErrInvalidPattern) {
		t.Fatalf("bad regex: %v", err)
	}
	counts, err := svc.CountsForUser(ctx, "user-1")
	if err != nil || counts.Total != 3 || counts.Unread != 2 || counts.ByLevel["error"] != 1 {
		t.Fatalf("counts %+v %v", counts, err)
	}
}
