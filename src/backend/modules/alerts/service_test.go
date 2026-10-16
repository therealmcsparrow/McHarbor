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
	svc := newTestService(t)

	alert, err := svc.Create(context.Background(), CreateAlertInput{
		Name:         "Container down",
		SourceType:   sourceContainer,
		ConditionKey: conditionDown,
		Condition:    ConditionConfig{ForSeconds: 300},
		Target:       "nginx",
		Destinations: []DestinationInput{{ChannelID: "channel-1"}},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if alert.Severity != "warning" {
		t.Fatalf("expected default severity to be warning, got %q", alert.Severity)
	}
	if !alert.NotifyOnRecovery {
		t.Fatal("expected recovery notifications to default on")
	}
}

func TestCreateRejectsUnknownCondition(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.Create(context.Background(), CreateAlertInput{
		Name:         "Nonsense",
		SourceType:   "kubernetes",
		ConditionKey: "pod_crashloop",
		Destinations: []DestinationInput{{ChannelID: "channel-1"}},
	})

	if !errors.Is(err, errAlertConditionUnknown) {
		t.Fatalf("expected errAlertConditionUnknown, got %v", err)
	}
}

// A wrong operator used to fall back to a default threshold silently,
// which is how a rule ended up watching something nobody asked for.
func TestCreateRejectsOperatorTheConditionDoesNotDeclare(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.Create(context.Background(), CreateAlertInput{
		Name:         "High CPU",
		SourceType:   sourceContainer,
		ConditionKey: conditionCPU,
		Condition:    ConditionConfig{Operator: "==", Threshold: 80},
		Destinations: []DestinationInput{{ChannelID: "channel-1"}},
	})

	if !errors.Is(err, errConditionInvalid) {
		t.Fatalf("expected errConditionInvalid, got %v", err)
	}
}

func TestCreateAllowsInAppOnlyDestination(t *testing.T) {
	svc := newTestService(t)

	alert, err := svc.Create(context.Background(), CreateAlertInput{
		Name:         "High CPU",
		SourceType:   sourceContainer,
		ConditionKey: conditionCPU,
		Condition:    ConditionConfig{Operator: ">", Threshold: 80},
		SendInApp:    true,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if !alert.SendInApp {
		t.Fatal("expected in-app delivery to be stored")
	}
}

func TestCreateRejectsRuleWithNoDestination(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.Create(context.Background(), CreateAlertInput{
		Name:         "High CPU",
		SourceType:   sourceContainer,
		ConditionKey: conditionCPU,
	})

	if !errors.Is(err, errAlertDestinationRequired) {
		t.Fatalf("expected errAlertDestinationRequired, got %v", err)
	}
}

func TestCreateStoresMultipleDestinations(t *testing.T) {
	svc := newTestService(t)

	alert, err := svc.Create(context.Background(), CreateAlertInput{
		Name:         "High CPU",
		Severity:     "critical",
		SourceType:   sourceContainer,
		ConditionKey: conditionCPU,
		Condition:    ConditionConfig{Operator: ">", Threshold: 80},
		Destinations: []DestinationInput{
			{ChannelID: "channel-1"},
			{ChannelID: "channel-2", MinSeverity: "critical"},
			// A duplicate must collapse rather than double-notify.
			{ChannelID: "channel-1"},
		},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if len(alert.Destinations) != 2 {
		t.Fatalf("expected 2 destinations, got %d", len(alert.Destinations))
	}

	found := false
	for _, destination := range alert.Destinations {
		if destination.ChannelID == "channel-2" && destination.MinSeverity == "critical" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected the per-destination minimum severity to be stored")
	}
}

func TestUpdateRejectsRemovingLastDestination(t *testing.T) {
	svc := newTestService(t)

	alert, err := svc.Create(context.Background(), CreateAlertInput{
		Name:         "High CPU",
		SourceType:   sourceContainer,
		ConditionKey: conditionCPU,
		Condition:    ConditionConfig{Operator: ">", Threshold: 80},
		Destinations: []DestinationInput{{ChannelID: "channel-1"}},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	empty := []DestinationInput{}
	_, err = svc.Update(context.Background(), alert.ID, UpdateAlertInput{Destinations: &empty})

	if !errors.Is(err, errAlertDestinationRequired) {
		t.Fatalf("expected errAlertDestinationRequired, got %v", err)
	}
}

func TestUpdateChangesSeverityAndEnabledState(t *testing.T) {
	svc := newTestService(t)

	alert, err := svc.Create(context.Background(), CreateAlertInput{
		Name:         "High CPU",
		Severity:     "warning",
		SourceType:   sourceContainer,
		ConditionKey: conditionCPU,
		Condition:    ConditionConfig{Operator: ">", Threshold: 80},
		Destinations: []DestinationInput{{ChannelID: "channel-1"}},
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

	if updated.Name != name {
		t.Fatalf("unexpected name %q", updated.Name)
	}
	if updated.Severity != severity {
		t.Fatalf("unexpected severity %q", updated.Severity)
	}
	if updated.Enabled {
		t.Fatal("expected the alert to be disabled")
	}
	if len(updated.Destinations) != 1 {
		t.Fatalf("expected destinations to survive a partial update, got %d", len(updated.Destinations))
	}
}

func TestListEnabledReturnsOnlyEnabledAlerts(t *testing.T) {
	svc := newTestService(t)

	enabledAlert, err := svc.Create(context.Background(), CreateAlertInput{
		Name:         "High CPU",
		SourceType:   sourceContainer,
		ConditionKey: conditionCPU,
		Condition:    ConditionConfig{Operator: ">", Threshold: 80},
		Destinations: []DestinationInput{{ChannelID: "channel-1"}},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	disabled := false
	if _, err := svc.Update(context.Background(), enabledAlert.ID, UpdateAlertInput{Enabled: &disabled}); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	if _, err := svc.Create(context.Background(), CreateAlertInput{
		Name:         "Container down",
		SourceType:   sourceContainer,
		ConditionKey: conditionDown,
		Condition:    ConditionConfig{ForSeconds: 300},
		Target:       "nginx",
		SendInApp:    true,
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
	if items[0].ConditionKey != conditionDown {
		t.Fatalf("expected the enabled alert to be %q, got %q", conditionDown, items[0].ConditionKey)
	}
}

// A rule written before the structured builder stores its condition as
// free text. It must convert on first read, using the same parsers
// that produced the old behaviour.
func TestLegacyConditionIsBackfilledOnRead(t *testing.T) {
	db := openAlertsTestDB(t)
	createAlertsSchema(t, db)

	if _, err := db.Exec(`
		INSERT INTO alerts (id, name, severity, type, condition, target, channel_id,
		                    source_type, condition_key, condition_config, environment_ids,
		                    cooldown_seconds, notify_on_recovery, message_template,
		                    send_in_app, enabled, created_at, updated_at)
		VALUES ('legacy-1', 'Legacy CPU', 'warning', 'cpu', '>= 91%', '*', 'channel-1',
		        'container', 'cpu', '{}', '', 0, 1, '', 1, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
	); err != nil {
		t.Fatalf("seeding legacy alert: %v", err)
	}

	svc := NewService(db)
	alert, err := svc.ByID(context.Background(), "legacy-1")
	if err != nil {
		t.Fatalf("ByID returned error: %v", err)
	}
	if alert == nil {
		t.Fatal("expected the legacy alert to be found")
	}

	if alert.Condition.Operator != ">=" || alert.Condition.Threshold != 91 {
		t.Fatalf("unexpected converted condition %+v", alert.Condition)
	}

	// The conversion must be written back, not redone on every read.
	var stored string
	if err := db.QueryRow("SELECT condition_config FROM alerts WHERE id = 'legacy-1'").Scan(&stored); err != nil {
		t.Fatalf("reading back condition_config: %v", err)
	}
	if stored == "{}" {
		t.Fatal("expected the converted condition to be persisted")
	}
}

func newTestService(t *testing.T) *Service {
	t.Helper()

	db := openAlertsTestDB(t)
	createAlertsSchema(t, db)
	return NewService(db)
}

func openAlertsTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "alerts-test.db")
	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatalf("sql.Open returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing test database: %v", err)
		}
	})

	return db
}

// createAlertsSchema mirrors the post-079 shape of the tables the
// service touches.
func createAlertsSchema(t *testing.T, db *sql.DB) {
	t.Helper()

	statements := []string{`
CREATE TABLE alerts (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	severity TEXT NOT NULL DEFAULT 'warning',
	type TEXT NOT NULL,
	condition TEXT,
	target TEXT,
	channel_id TEXT,
	source_type TEXT NOT NULL DEFAULT 'container',
	condition_key TEXT NOT NULL DEFAULT '',
	condition_config TEXT NOT NULL DEFAULT '{}',
	environment_ids TEXT NOT NULL DEFAULT '',
	cooldown_seconds INTEGER NOT NULL DEFAULT 0,
	notify_on_recovery INTEGER NOT NULL DEFAULT 1,
	message_template TEXT NOT NULL DEFAULT '',
	send_in_app INTEGER NOT NULL DEFAULT 0,
	enabled INTEGER NOT NULL DEFAULT 1,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
)`, `
CREATE TABLE alert_destinations (
	id TEXT PRIMARY KEY,
	alert_id TEXT NOT NULL,
	channel_id TEXT NOT NULL,
	min_severity TEXT,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
)`, `
CREATE UNIQUE INDEX idx_alert_destinations_unique ON alert_destinations(alert_id, channel_id)`, `
CREATE TABLE alert_states (
	state_key TEXT PRIMARY KEY,
	alert_id TEXT NOT NULL,
	active INTEGER NOT NULL DEFAULT 0,
	active_since TEXT,
	last_notified_at TEXT,
	last_seen TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
	updated_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
)`, `
CREATE TABLE communication_channels (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	channel_type TEXT NOT NULL
)`}

	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("creating alerts schema: %v", err)
		}
	}
}
