// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestServiceListActionsFiltersByModule(t *testing.T) {
	db := openAuditTestDB(t)
	svc := NewService(db, nil, nil, "test-secret")

	actions, err := svc.ListActions(context.Background(), "users")
	if err != nil {
		t.Fatalf("list actions: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("expected 1 users action, got %d", len(actions))
	}
	if actions[0].Action != "user.created" || actions[0].SeverityDefault != "success" {
		t.Fatalf("unexpected action entry: %#v", actions[0])
	}
}

func TestServicePluginSummaryGroupsPluginRows(t *testing.T) {
	db := openAuditTestDB(t)
	svc := NewService(db, nil, nil, "test-secret")

	summary, err := svc.PluginSummary(context.Background())
	if err != nil {
		t.Fatalf("plugin summary: %v", err)
	}
	if len(summary) != 2 {
		t.Fatalf("expected 2 plugin summary rows, got %d", len(summary))
	}
	item := summary[0]
	if item.PluginSlug != "comm-slack" || item.Count != 3 {
		t.Fatalf("unexpected summary item: %#v", item)
	}
	if item.SeverityBreakdown["error"] != 1 || item.SeverityBreakdown["success"] != 1 || item.SeverityBreakdown["info"] != 1 {
		t.Fatalf("unexpected severity breakdown: %#v", item.SeverityBreakdown)
	}
}

func TestServicePluginSummaryMarksUninstalledPlugins(t *testing.T) {
	db := openAuditTestDB(t)
	svc := NewService(db, nil, nil, "test-secret")

	summary, err := svc.PluginSummary(context.Background())
	if err != nil {
		t.Fatalf("plugin summary: %v", err)
	}
	byslug := map[string]PluginAuditSummary{}
	for _, item := range summary {
		byslug[item.PluginSlug] = item
	}
	installed, ok := byslug["comm-slack"]
	if !ok {
		t.Fatalf("installed plugin missing from summary: %#v", summary)
	}
	if !installed.Installed || !installed.Enabled {
		t.Fatalf("comm-slack has an appstore_plugins row and must read as installed and enabled: %#v", installed)
	}
	gone, ok := byslug["gone-plugin"]
	if !ok {
		t.Fatalf("an uninstalled plugin must keep its audit history: %#v", summary)
	}
	if gone.Installed || gone.Enabled {
		t.Fatalf("gone-plugin has no appstore_plugins row and must read as uninstalled: %#v", gone)
	}
	if gone.Count != 2 || gone.LastSeen == "" {
		t.Fatalf("uninstalled plugin must keep its event count and last-seen stamp: %#v", gone)
	}
}

func openAuditTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing test database: %v", err)
		}
	})
	_, err = db.Exec(`
		CREATE TABLE audit_action_catalog (
			id TEXT PRIMARY KEY,
			module TEXT NOT NULL,
			action TEXT NOT NULL UNIQUE,
			severity_default TEXT NOT NULL,
			description TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		CREATE TABLE audit_logs (
			id TEXT PRIMARY KEY,
			action TEXT NOT NULL,
			severity TEXT NOT NULL,
			entity_type TEXT,
			entity_id TEXT,
			timestamp TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		CREATE VIEW audit_logs_by_plugin AS
		SELECT
			entity_id AS plugin_slug,
			COUNT(1) AS count,
			MAX(timestamp) AS last_seen,
			SUM(CASE WHEN severity = 'info' THEN 1 ELSE 0 END) AS info_count,
			SUM(CASE WHEN severity = 'success' THEN 1 ELSE 0 END) AS success_count,
			SUM(CASE WHEN severity = 'warning' THEN 1 ELSE 0 END) AS warning_count,
			SUM(CASE WHEN severity = 'error' THEN 1 ELSE 0 END) AS error_count
		FROM audit_logs
		WHERE entity_type = 'plugin' AND entity_id IS NOT NULL AND entity_id != ''
		GROUP BY entity_id;
		CREATE TABLE appstore_plugins (
			id TEXT PRIMARY KEY,
			slug TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 0
		);
		INSERT INTO appstore_plugins (id, slug, enabled) VALUES ('p1', 'comm-slack', 1);
		INSERT INTO audit_action_catalog (id, module, action, severity_default, description, created_at, updated_at) VALUES
		('catalog-users-create', 'users', 'user.created', 'success', 'User was created', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		('catalog-stacks-delete', 'stacks', 'stack.deleted', 'warning', 'Stack was deleted', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
		INSERT INTO audit_logs (id, action, severity, entity_type, entity_id, timestamp, created_at, updated_at) VALUES
		('plugin-1', 'plugin.install', 'success', 'plugin', 'comm-slack', '2026-08-25T09:00:00Z', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		('plugin-2', 'plugin.error', 'error', 'plugin', 'comm-slack', '2026-08-25T09:01:00Z', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		('plugin-3', 'plugin.status', 'info', 'plugin', 'comm-slack', '2026-08-25T09:02:00Z', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		('plugin-4', 'plugin.install', 'success', 'plugin', 'gone-plugin', '2026-08-24T09:00:00Z', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		('plugin-5', 'plugin.uninstall', 'warning', 'plugin', 'gone-plugin', '2026-08-24T09:05:00Z', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		('non-plugin', 'user.created', 'success', 'user', 'user-1', '2026-08-25T09:03:00Z', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
	`)
	if err != nil {
		t.Fatalf("create audit test schema: %v", err)
	}
	return db
}
