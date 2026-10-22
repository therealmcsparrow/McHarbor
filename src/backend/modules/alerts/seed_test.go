// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"context"
	"database/sql"
	"testing"
)

func openSeedDB(t *testing.T) *sql.DB {
	t.Helper()
	db := openAlertsTestDB(t)
	createAlertsSchema(t, db)
	for _, stmt := range []string{
		`ALTER TABLE communication_channels ADD COLUMN enabled INTEGER NOT NULL DEFAULT 1`,
		`CREATE TABLE settings (id TEXT PRIMARY KEY, key TEXT NOT NULL UNIQUE, value TEXT, category TEXT DEFAULT 'general',
			created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP), updated_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP))`,
		`INSERT INTO communication_channels (id, name, channel_type, enabled) VALUES ('c1', 'Slack', 'slack', 1), ('c2', 'Mail', 'smtp', 1), ('c3', 'Off', 'smtp', 0)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	return db
}

func countRows(t *testing.T, db *sql.DB, query string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func TestSeedDefaultRulesCreatesThreeRulesOnce(t *testing.T) {
	db := openSeedDB(t)
	svc := NewService(db)
	for i := 0; i < 3; i++ {
		if err := svc.SeedDefaultRules(context.Background()); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM alerts`); n != 3 {
		t.Fatalf("alerts = %d, want 3 after three boots", n)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM alerts WHERE send_in_app = 1 AND enabled = 1 AND source_type = 'host'`); n != 3 {
		t.Fatalf("seeded rules are not all enabled in-app host rules: %d", n)
	}
}

func TestSeedDefaultRulesAttachesEnabledChannelsAtCritical(t *testing.T) {
	db := openSeedDB(t)
	if err := NewService(db).SeedDefaultRules(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM alert_destinations`); n != 6 {
		t.Fatalf("destinations = %d, want 2 enabled channels x 3 rules", n)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM alert_destinations WHERE min_severity = 'critical'`); n != 6 {
		t.Fatalf("every destination must be critical-only, got %d", n)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM alert_destinations WHERE channel_id = 'c3'`); n != 0 {
		t.Fatal("a disabled channel must not be attached")
	}
}

func TestSeedDefaultRulesDoesNotRecreateADeletedRule(t *testing.T) {
	db := openSeedDB(t)
	svc := NewService(db)
	if err := svc.SeedDefaultRules(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM alerts`); err != nil {
		t.Fatal(err)
	}
	if err := svc.SeedDefaultRules(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM alerts`); n != 0 {
		t.Fatalf("a rule the operator deleted came back: %d rows", n)
	}
}

func TestSeedDefaultRulesRollsBackWithoutMarker(t *testing.T) {
	db := openSeedDB(t)
	if _, err := db.Exec(`DROP TABLE alert_destinations`); err != nil {
		t.Fatal(err)
	}
	if err := NewService(db).SeedDefaultRules(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM alerts`) + countRows(t, db, `SELECT COUNT(*) FROM settings`); n != 0 {
		t.Fatalf("a failed seed left %d rows; rules and marker must commit together", n)
	}
}

func TestHostFSThresholdsFollowTheRules(t *testing.T) {
	db := openSeedDB(t)
	svc := NewService(db)
	w, c, err := svc.HostFSThresholds(context.Background(), "env1")
	if err != nil || w != 85 || c != 95 {
		t.Fatalf("no rules: %v %v %v, want 85/95", w, c, err)
	}
	if err := svc.SeedDefaultRules(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE alerts SET condition_config = '{"operator":">=","threshold":70,"unit":"%"}' WHERE name = 'Host disk usage warning'`); err != nil {
		t.Fatal(err)
	}
	w, c, err = svc.HostFSThresholds(context.Background(), "env1")
	if err != nil || w != 70 || c != 95 {
		t.Fatalf("edited rule: %v %v %v, want 70/95", w, c, err)
	}
	if _, err := db.Exec(`UPDATE alerts SET environment_ids = '["other"]' WHERE name = 'Host disk usage warning'`); err != nil {
		t.Fatal(err)
	}
	if w, _, err = svc.HostFSThresholds(context.Background(), "env1"); err != nil || w != 85 {
		t.Fatalf("a rule scoped to another environment must not apply, got %v", w)
	}
}

func TestSeedDefaultRulesLosesTheClaimToAnExistingMarkerRow(t *testing.T) {
	db := openSeedDB(t)
	if _, err := db.Exec(`INSERT INTO settings (id, category, key, value) VALUES ('m', 'alerts', ?, '')`, defaultDiskRulesKey); err != nil {
		t.Fatal(err)
	}
	if err := NewService(db).SeedDefaultRules(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM alerts`); n != 0 {
		t.Fatalf("a marker row already claimed by another node still produced %d rules", n)
	}
}
