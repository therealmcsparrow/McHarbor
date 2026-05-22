// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/inapp"
	corenotify "github.com/therealmcsparrow/mcharbor/core/notify"
)

func TestMatchesAlertTarget(t *testing.T) {
	if !matchesAlertTarget("frontend-*,database", "frontend-api", "ctr-1") {
		t.Fatal("expected wildcard target to match container name")
	}
	if !matchesAlertTarget("*", "anything") {
		t.Fatal("expected catch-all target to match")
	}
	if matchesAlertTarget("database", "frontend-api") {
		t.Fatal("expected unmatched target to return false")
	}
}

func TestParseDiskConditionSupportsByteThresholds(t *testing.T) {
	condition := parseDiskCondition(">= 10 GB")
	if !condition.UseBytes {
		t.Fatal("expected disk condition to use byte comparison")
	}
	if condition.Comparison.Operator != ">=" {
		t.Fatalf("expected operator >=, got %q", condition.Comparison.Operator)
	}
	if condition.Comparison.Threshold != 10 {
		t.Fatalf("expected threshold 10, got %v", condition.Comparison.Threshold)
	}
	if condition.UnitLabel != "GB" {
		t.Fatalf("expected unit label GB, got %q", condition.UnitLabel)
	}
}

func TestResolveDiskCapacityUsesDriverStatus(t *testing.T) {
	info := &SystemInfo{
		DriverStatus: [][]string{
			{"Data Space Total", "100 GB"},
		},
	}

	capacity, ok := resolveDiskCapacity(info, 0)
	if !ok {
		t.Fatal("expected disk capacity to be resolved")
	}
	if capacity != 100*1024*1024*1024 {
		t.Fatalf("unexpected disk capacity %d", capacity)
	}
}

func TestTransitionStateOnlyNotifiesOnRisingEdge(t *testing.T) {
	engine := &Engine{states: make(map[string]engineState)}
	now := time.Date(2026, time.March, 16, 12, 0, 0, 0, time.UTC)

	if !engine.transitionState("rule:env:subject", now, true) {
		t.Fatal("expected first active transition to notify")
	}
	if engine.transitionState("rule:env:subject", now.Add(time.Second), true) {
		t.Fatal("did not expect repeated active state to notify")
	}
	if engine.transitionState("rule:env:subject", now.Add(2*time.Second), false) {
		t.Fatal("did not expect inactive transition to notify")
	}
	if !engine.transitionState("rule:env:subject", now.Add(3*time.Second), true) {
		t.Fatal("expected state reactivation to notify")
	}
}

func TestDeliverWritesInAppNotification(t *testing.T) {
	db := openAlertsTestDB(t)
	createInAppNotificationTable(t, db)

	engine := &Engine{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		states: make(map[string]engineState),
		sendChannel: func(context.Context, corenotify.ChannelRequest) (*corenotify.Delivery, error) {
			return nil, nil
		},
		sendInApp: func(record inapp.Record) error {
			return inapp.CreateBroadcast(db, record)
		},
	}

	engine.deliver(context.Background(), Alert{
		ID:        "alert-1",
		Name:      "High CPU",
		Severity:  "critical",
		SendInApp: true,
	}, "Alert triggered: High CPU", "Container frontend-api in environment Local reached 92.1% of CPU usage (rule > 80.0%).")

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM in_app_notifications").Scan(&count); err != nil {
		t.Fatalf("counting in-app notifications: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 in-app notification, got %d", count)
	}

	var level, title, message, action, entityType, entityID string
	if err := db.QueryRow(
		`SELECT level, title, message, action, entity_type, entity_id
		 FROM in_app_notifications
		 LIMIT 1`,
	).Scan(&level, &title, &message, &action, &entityType, &entityID); err != nil {
		t.Fatalf("reading in-app notification: %v", err)
	}

	if level != "warning" {
		t.Fatalf("expected critical alert to map to warning in-app level, got %q", level)
	}
	if title != "Alert triggered: High CPU" {
		t.Fatalf("unexpected title %q", title)
	}
	if action != "alert.triggered" {
		t.Fatalf("unexpected action %q", action)
	}
	if entityType != "alert" || entityID != "alert-1" {
		t.Fatalf("unexpected entity metadata %q/%q", entityType, entityID)
	}
	if message == "" {
		t.Fatal("expected message to be stored")
	}
}

func createInAppNotificationTable(t *testing.T, db *sql.DB) {
	t.Helper()

	if _, err := db.Exec(`
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
)`); err != nil {
		t.Fatalf("creating in-app notification table: %v", err)
	}
}
