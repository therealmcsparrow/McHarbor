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
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/alerting"
)

func TestMatchesAlertTarget(t *testing.T) {
	if !matchesAlertTarget("frontend-*,database", "frontend-api", "ctr-1") {
		t.Fatal("expected wildcard target to match")
	}
	if !matchesAlertTarget("*", "anything") {
		t.Fatal("expected star target to match everything")
	}
	if matchesAlertTarget("database", "frontend-api") {
		t.Fatal("did not expect unrelated target to match")
	}
}

func TestParseDiskConditionSupportsByteThresholds(t *testing.T) {
	condition := parseDiskCondition(">= 10 GB")

	if !condition.UseBytes {
		t.Fatal("expected byte-based disk condition")
	}
	if condition.Comparison.Operator != ">=" {
		t.Fatalf("unexpected operator %q", condition.Comparison.Operator)
	}
	if condition.Comparison.Threshold != 10 {
		t.Fatalf("unexpected threshold %v", condition.Comparison.Threshold)
	}
	if condition.UnitLabel != "GB" {
		t.Fatalf("unexpected unit label %q", condition.UnitLabel)
	}
}

func TestResolveDiskCapacityUsesDriverStatus(t *testing.T) {
	info := &SystemInfo{DriverStatus: [][]string{
		{"Data Space Total", "100 GB"},
	}}

	capacity, ok := resolveDiskCapacity(info, 0)
	if !ok {
		t.Fatal("expected disk capacity to be resolved")
	}
	if capacity != 100*1024*1024*1024 {
		t.Fatalf("unexpected disk capacity %d", capacity)
	}
}

func TestDecideNotifiesOnRisingEdgeOnly(t *testing.T) {
	now := time.Date(2026, time.March, 16, 12, 0, 0, 0, time.UTC)
	state := alertState{Key: "rule:env:subject", AlertID: "rule"}

	first := decide(state, true, time.Time{}, now, 0, 0, true)
	if !first.Notify || first.State != EventStateFiring {
		t.Fatal("expected the first active evaluation to notify")
	}

	second := decide(first.Next, true, time.Time{}, now.Add(time.Second), 0, 0, true)
	if second.Notify {
		t.Fatal("did not expect a repeated active state to notify without a cooldown")
	}

	recovered := decide(second.Next, false, time.Time{}, now.Add(2*time.Second), 0, 0, true)
	if !recovered.Notify || recovered.State != EventStateResolved {
		t.Fatal("expected recovery to notify")
	}

	again := decide(recovered.Next, true, time.Time{}, now.Add(3*time.Second), 0, 0, true)
	if !again.Notify || again.State != EventStateFiring {
		t.Fatal("expected re-activation to notify as a fresh incident")
	}
}

func TestDecideHoldsUntilSustainWindowElapses(t *testing.T) {
	now := time.Date(2026, time.March, 16, 12, 0, 0, 0, time.UTC)
	sustain := 5 * time.Minute

	first := decide(alertState{}, true, time.Time{}, now, sustain, 0, true)
	if first.Notify {
		t.Fatal("did not expect a notification inside the sustain window")
	}
	if !first.Next.Active {
		t.Fatal("expected the state to be recorded as active while sustaining")
	}

	tooEarly := decide(first.Next, true, time.Time{}, now.Add(4*time.Minute), sustain, 0, true)
	if tooEarly.Notify {
		t.Fatal("did not expect a notification before the sustain window elapsed")
	}

	held := decide(tooEarly.Next, true, time.Time{}, now.Add(5*time.Minute), sustain, 0, true)
	if !held.Notify || held.State != EventStateFiring {
		t.Fatal("expected a notification once the sustain window elapsed")
	}
}

func TestDecideSkipsRecoveryWhenNothingWasDelivered(t *testing.T) {
	now := time.Date(2026, time.March, 16, 12, 0, 0, 0, time.UTC)
	sustain := 5 * time.Minute

	// Active, but it never held long enough to notify anyone.
	sustaining := decide(alertState{}, true, time.Time{}, now, sustain, 0, true)
	if sustaining.Notify {
		t.Fatal("precondition failed: expected no notification inside the sustain window")
	}

	recovered := decide(sustaining.Next, false, time.Time{}, now.Add(time.Minute), sustain, 0, true)
	if recovered.Notify {
		t.Fatal("did not expect a recovery message for an incident that never fired")
	}
}

func TestDecideRepeatsOnlyAfterCooldown(t *testing.T) {
	now := time.Date(2026, time.March, 16, 12, 0, 0, 0, time.UTC)
	cooldown := 10 * time.Minute

	first := decide(alertState{}, true, time.Time{}, now, 0, cooldown, true)
	if !first.Notify {
		t.Fatal("expected the first evaluation to notify")
	}

	early := decide(first.Next, true, time.Time{}, now.Add(9*time.Minute), 0, cooldown, true)
	if early.Notify {
		t.Fatal("did not expect a repeat inside the cooldown")
	}

	late := decide(early.Next, true, time.Time{}, now.Add(10*time.Minute), 0, cooldown, true)
	if !late.Notify || late.State != EventStateFiring {
		t.Fatal("expected a repeat once the cooldown elapsed")
	}
}

func TestDecideUsesObservedSinceForSustain(t *testing.T) {
	now := time.Date(2026, time.March, 16, 12, 0, 0, 0, time.UTC)

	// A container that has been down for an hour must fire on the
	// first evaluation, not an hour after the engine noticed it.
	result := decide(alertState{}, true, now.Add(-time.Hour), now, 30*time.Minute, 0, true)
	if !result.Notify {
		t.Fatal("expected a source-reported start time to satisfy the sustain window immediately")
	}
}

func TestDecideHonoursRecoveryOptOut(t *testing.T) {
	now := time.Date(2026, time.March, 16, 12, 0, 0, 0, time.UTC)

	firing := decide(alertState{}, true, time.Time{}, now, 0, 0, false)
	recovered := decide(firing.Next, false, time.Time{}, now.Add(time.Minute), 0, 0, false)

	if recovered.Notify {
		t.Fatal("did not expect a recovery message when the rule opted out")
	}
	if recovered.Next.Active {
		t.Fatal("expected the state to clear even without a recovery message")
	}
}

func TestConditionTriggeredByKind(t *testing.T) {
	threshold := alerting.AlertConditionDecl{Kind: alerting.KindThreshold, DefaultOperator: ">"}
	cfg := ConditionConfig{Operator: ">", Threshold: 80}

	if !conditionTriggered(alerting.AlertObservation{Value: 92}, cfg, threshold) {
		t.Fatal("expected 92 > 80 to trigger")
	}
	if conditionTriggered(alerting.AlertObservation{Value: 12}, cfg, threshold) {
		t.Fatal("did not expect 12 > 80 to trigger")
	}

	event := alerting.AlertConditionDecl{Kind: alerting.KindEvent}
	if !conditionTriggered(alerting.AlertObservation{Active: true}, ConditionConfig{}, event) {
		t.Fatal("expected an active event observation to trigger")
	}
	if conditionTriggered(alerting.AlertObservation{Active: false}, ConditionConfig{}, event) {
		t.Fatal("did not expect an inactive event observation to trigger")
	}
}

func TestMeetsMinSeverity(t *testing.T) {
	if !meetsMinSeverity("warning", "") {
		t.Fatal("an unset minimum must accept every severity")
	}
	if !meetsMinSeverity("critical", "warning") {
		t.Fatal("critical must satisfy a warning minimum")
	}
	if meetsMinSeverity("info", "critical") {
		t.Fatal("info must not satisfy a critical minimum")
	}
}

func TestDeliverWritesInAppNotification(t *testing.T) {
	db := openAlertsTestDB(t)
	createInAppNotificationTable(t, db)

	n := &notifier{
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		sendInApp: func(record inapp.Record) error { return inapp.CreateBroadcast(db, record) },
	}

	rule := Alert{ID: "alert-1", Name: "High CPU", Severity: "critical", SendInApp: true}
	deliveries := n.deliver(context.Background(), rule, deliveryInput{
		Title:   alertTitle(rule, EventStateFiring),
		Message: "CPU usage on frontend-api in Local — 92.1% (rule > 80.0%).",
		State:   EventStateFiring,
	})

	if len(deliveries) != 0 {
		t.Fatalf("expected no channel deliveries, got %d", len(deliveries))
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM in_app_notifications").Scan(&count); err != nil {
		t.Fatalf("counting in-app notifications: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 in-app notification, got %d", count)
	}

	var level, title, action, entityType, entityID string
	if err := db.QueryRow(
		`SELECT level, title, action, entity_type, entity_id FROM in_app_notifications LIMIT 1`,
	).Scan(&level, &title, &action, &entityType, &entityID); err != nil {
		t.Fatalf("reading in-app notification: %v", err)
	}

	if level != "warning" {
		t.Fatalf("expected critical alerts to map to the warning in-app level, got %q", level)
	}
	if title != "Alert triggered: High CPU" {
		t.Fatalf("unexpected title %q", title)
	}
	if action != "alert.firing" {
		t.Fatalf("unexpected action %q", action)
	}
	if entityType != "alert" || entityID != "alert-1" {
		t.Fatalf("unexpected entity metadata %q/%q", entityType, entityID)
	}
}

func TestRenderMessageFallsBackToBuiltInSentence(t *testing.T) {
	mc := messageContext{
		ConditionName:   "CPU usage",
		ConditionRule:   "> 80.0%",
		EnvironmentName: "Local",
		SubjectName:     "frontend-api",
		ValueLabel:      "92.1%",
		State:           EventStateFiring,
	}

	message := renderMessage("", mc)
	if message != "CPU usage on frontend-api in Local — 92.1% (rule > 80.0%)." {
		t.Fatalf("unexpected default message %q", message)
	}

	templated := renderMessage("{{subject}} is at {{value}} in {{environment}}", mc)
	if templated != "frontend-api is at 92.1% in Local" {
		t.Fatalf("unexpected templated message %q", templated)
	}
}

func TestRenderMessageLeavesUnknownPlaceholders(t *testing.T) {
	message := renderMessage("{{subject}} / {{nonsense}}", messageContext{SubjectName: "db"})
	if message != "db / {{nonsense}}" {
		t.Fatalf("expected unknown placeholders to survive verbatim, got %q", message)
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

// A count over a window must be sampled at least twice per window.
//
// The bug this pins came in two layers. A rule counting failed
// sign-ins over 60 seconds was first evaluated only every 5 minutes
// (the source's declared interval), so 4 minutes of every 5 went
// uncounted. Capping the interval at the window was not enough:
// evaluations are quantised to the engine tick, so a 60-second
// interval landed 80 seconds apart while each evaluation still looked
// back only 60 seconds — a real burst was missed by 8 seconds.
// Sampling at half the window makes consecutive windows overlap.
func TestDueSamplesAtLeastTwicePerWindow(t *testing.T) {
	engine := &Engine{lastRun: make(map[string]time.Time)}
	decl := alerting.AlertConditionDecl{
		Kind:         alerting.KindCountInWindow,
		PollInterval: 5 * time.Minute,
	}
	rule := Alert{ID: "rule-1", ConditionKey: "failed_logins", Condition: ConditionConfig{WindowSeconds: 60}}

	start := time.Date(2026, time.March, 16, 12, 0, 0, 0, time.UTC)
	if !engine.due(rule, decl, start) {
		t.Fatal("expected the first evaluation to run")
	}
	if engine.due(rule, decl, start.Add(29*time.Second)) {
		t.Fatal("did not expect an evaluation before half the window elapsed")
	}
	if !engine.due(rule, decl, start.Add(30*time.Second)) {
		t.Fatal("expected an evaluation once half the window had elapsed")
	}
}

// A window longer than the declared interval must not make the source
// busier than it asked to be.
func TestDueKeepsDeclaredIntervalForLongWindows(t *testing.T) {
	engine := &Engine{lastRun: make(map[string]time.Time)}
	decl := alerting.AlertConditionDecl{
		Kind:         alerting.KindCountInWindow,
		PollInterval: 5 * time.Minute,
	}
	// Half of an hour is still longer than the declared 5 minutes, so
	// the source's own cadence wins.
	rule := Alert{ID: "rule-2", ConditionKey: "failed_logins", Condition: ConditionConfig{WindowSeconds: 3600}}

	start := time.Date(2026, time.March, 16, 12, 0, 0, 0, time.UTC)
	if !engine.due(rule, decl, start) {
		t.Fatal("expected the first evaluation to run")
	}
	if engine.due(rule, decl, start.Add(4*time.Minute)) {
		t.Fatal("did not expect an evaluation before the declared interval elapsed")
	}
	if !engine.due(rule, decl, start.Add(5*time.Minute)) {
		t.Fatal("expected an evaluation once the declared interval elapsed")
	}
}

// Conditions without a window keep the interval their source declared.
func TestDueLeavesWindowlessConditionsAlone(t *testing.T) {
	engine := &Engine{lastRun: make(map[string]time.Time)}
	decl := alerting.AlertConditionDecl{
		Kind:         alerting.KindEvent,
		PollInterval: 30 * time.Minute,
	}
	rule := Alert{ID: "rule-3", ConditionKey: "image_update"}

	start := time.Date(2026, time.March, 16, 12, 0, 0, 0, time.UTC)
	if !engine.due(rule, decl, start) {
		t.Fatal("expected the first evaluation to run")
	}
	if engine.due(rule, decl, start.Add(29*time.Minute)) {
		t.Fatal("did not expect an early evaluation")
	}
	if !engine.due(rule, decl, start.Add(30*time.Minute)) {
		t.Fatal("expected an evaluation once the declared interval elapsed")
	}
}
