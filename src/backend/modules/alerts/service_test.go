// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestCreateDefaultsSeverityToWarning(t *testing.T) {
	db := openAlertsTestDB(t)
	createAlertsTable(t, db)

	svc := NewService(db)

	alert, err := svc.Create(context.Background(), CreateAlertInput{
		Name:      "Container down",
		Type:      "container_down",
		Condition: "stopped",
		Target:    "nginx",
		ChannelID: "channel-1",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if alert.Severity != "warning" {
		t.Fatalf("expected default severity to be warning, got %q", alert.Severity)
	}
}

func TestUpdateChangesSeverityAndEnabledState(t *testing.T) {
	db := openAlertsTestDB(t)
	createAlertsTable(t, db)

	svc := NewService(db)
	alert, err := svc.Create(context.Background(), CreateAlertInput{
		Name:      "High CPU",
		Type:      "cpu",
		Severity:  "warning",
		Condition: "> 80%",
		Target:    "*",
		ChannelID: "channel-1",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	name := "Critical CPU"
	severity := "critical"
	enabled := false

	updated, err := svc.Update(context.Background(), alert.ID, UpdateAlertInput{
		Name:     &name,
		Severity: &severity,
		Enabled:  &enabled,
	})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if updated == nil {
		t.Fatal("expected updated alert, got nil")
	}
	if updated.Name != name {
		t.Fatalf("expected updated name %q, got %q", name, updated.Name)
	}
	if updated.Severity != severity {
		t.Fatalf("expected updated severity %q, got %q", severity, updated.Severity)
	}
	if updated.Enabled {
		t.Fatal("expected updated alert to be disabled")
	}
}

func TestCreateAllowsInAppOnlyDestination(t *testing.T) {
	db := openAlertsTestDB(t)
	createAlertsTable(t, db)

	svc := NewService(db)

	alert, err := svc.Create(context.Background(), CreateAlertInput{
		Name:      "Disk notice",
		Type:      "disk",
		Severity:  "info",
		Condition: "> 75%",
		Target:    "*",
		SendInApp: true,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if !alert.SendInApp {
		t.Fatal("expected alert to enable in-app delivery")
	}
	if alert.ChannelID != "" {
		t.Fatalf("expected in-app only alert to have empty channel id, got %q", alert.ChannelID)
	}
}

func TestUpdateRejectsRemovingLastDestination(t *testing.T) {
	db := openAlertsTestDB(t)
	createAlertsTable(t, db)

	svc := NewService(db)
	alert, err := svc.Create(context.Background(), CreateAlertInput{
		Name:      "Container down",
		Type:      "container_down",
		Condition: "stopped",
		Target:    "nginx",
		ChannelID: "channel-1",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	emptyChannelID := ""
	sendInApp := false

	_, err = svc.Update(context.Background(), alert.ID, UpdateAlertInput{
		ChannelID: &emptyChannelID,
		SendInApp: &sendInApp,
	})
	if !errors.Is(err, errAlertDestinationRequired) {
		t.Fatalf("expected errAlertDestinationRequired, got %v", err)
	}
}

func TestListEnabledReturnsOnlyEnabledAlerts(t *testing.T) {
	db := openAlertsTestDB(t)
	createAlertsTable(t, db)

	svc := NewService(db)
	enabledAlert, err := svc.Create(context.Background(), CreateAlertInput{
		Name:      "High CPU",
		Type:      "cpu",
		Condition: "> 80%",
		Target:    "*",
		ChannelID: "channel-1",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	disabled := false
	if _, err := svc.Update(context.Background(), enabledAlert.ID, UpdateAlertInput{Enabled: &disabled}); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	if _, err := svc.Create(context.Background(), CreateAlertInput{
		Name:      "Container down",
		Type:      "container_down",
		Condition: "stopped > 5m",
		Target:    "nginx",
		SendInApp: true,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	items, err := svc.ListEnabled(context.Background())
	if err != nil {
		t.Fatalf("ListEnabled returned error: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("expected exactly 1 enabled alert, got %d", len(items))
	}
	if items[0].Type != "container_down" {
		t.Fatalf("expected enabled alert type container_down, got %q", items[0].Type)
	}
}

func openAlertsTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "alerts-test.db")
	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatalf("sql.Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	return db
}

func createAlertsTable(t *testing.T, db *sql.DB) {
	t.Helper()

	if _, err := db.Exec(`
CREATE TABLE alerts (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	severity TEXT NOT NULL DEFAULT 'warning',
	type TEXT NOT NULL,
	condition TEXT,
	target TEXT,
	channel_id TEXT,
	send_in_app INTEGER NOT NULL DEFAULT 0,
	enabled INTEGER NOT NULL DEFAULT 1,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
)`); err != nil {
		t.Fatalf("creating alerts table: %v", err)
	}
}
