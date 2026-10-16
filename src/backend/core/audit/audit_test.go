// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import (
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestLoggerUsesCatalogSeverityBeforeHeuristic(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer closeTest(t, db)
	db.SetMaxOpenConns(1)

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
			user_id TEXT,
			username TEXT,
			action TEXT NOT NULL,
			severity TEXT NOT NULL,
			entity_type TEXT,
			entity_id TEXT,
			entity_name TEXT,
			details TEXT,
			state_before TEXT,
			state_after TEXT,
			changed_keys TEXT,
			ip_address TEXT,
			environment_id TEXT,
			environment_name TEXT,
			timestamp TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			prev_hash TEXT,
			row_hash TEXT,
			api_key_id TEXT,
			api_key_name TEXT,
			node_id TEXT,
			exec_node_id TEXT
		);
		INSERT INTO audit_action_catalog (id, module, action, severity_default, description, created_at, updated_at)
		VALUES ('catalog-delete', 'test', 'delete', 'success', 'catalog override', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
	`)
	if err != nil {
		t.Fatalf("create test schema: %v", err)
	}

	SetNodeID("node-a")
	t.Cleanup(func() { SetNodeID("") })
	logger := NewLogger(db, "test-secret")
	logger.Log(nil, Entry{Action: "delete", EntityType: "test"})

	var severity string
	var node sql.NullString
	if err := db.QueryRow("SELECT severity, node_id FROM audit_logs WHERE action = 'delete'").Scan(&severity, &node); err != nil {
		t.Fatalf("query audit severity: %v", err)
	}
	if severity != "success" {
		t.Fatalf("expected catalog severity success, got %q", severity)
	}
	if node.String != "node-a" {
		t.Fatalf("audit row node_id = %q, want the node that served it", node.String)
	}
}

// TestExecutingNodeIsRecorded is the audit half of cross-node agent routing: a request
// served here whose agent call ran elsewhere must say so, or the trail claims work
// happened on a node that never touched the daemon.
func TestExecutingNodeIsRecorded(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeTest(t, db) })
	if _, err := db.Exec(`
		CREATE TABLE audit_logs (
			id TEXT PRIMARY KEY, user_id TEXT, username TEXT, action TEXT NOT NULL, severity TEXT NOT NULL,
			entity_type TEXT, entity_id TEXT, entity_name TEXT, details TEXT, state_before TEXT,
			state_after TEXT, changed_keys TEXT, ip_address TEXT, environment_id TEXT, environment_name TEXT,
			timestamp TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, prev_hash TEXT,
			row_hash TEXT, api_key_id TEXT, api_key_name TEXT, node_id TEXT, exec_node_id TEXT
		);
	`); err != nil {
		t.Fatal(err)
	}

	SetNodeID("node-b")
	t.Cleanup(func() { SetNodeID("") })
	logger := NewLogger(db, "test-secret")

	req := httptest.NewRequest(http.MethodPost, "/api/containers/abc/stop", nil)
	req = req.WithContext(WithExecutor(req.Context()))
	RecordExecutor(req.Context(), "node-a")
	logger.Log(req, Entry{Action: "container.stop", EntityType: "container", EntityID: "abc"})

	var served, executed sql.NullString
	if err := db.QueryRow("SELECT node_id, exec_node_id FROM audit_logs WHERE action = 'container.stop'").
		Scan(&served, &executed); err != nil {
		t.Fatal(err)
	}
	if served.String != "node-b" {
		t.Fatalf("node_id = %q, want the node that served the request", served.String)
	}
	if executed.String != "node-a" {
		t.Fatalf("exec_node_id = %q, want the node that ran the agent call", executed.String)
	}

	// A request that never left this node leaves the column null rather than
	// repeating node_id, so a forwarded call is distinguishable from a local one.
	local := httptest.NewRequest(http.MethodPost, "/api/containers/def/stop", nil)
	local = local.WithContext(WithExecutor(local.Context()))
	logger.Log(local, Entry{Action: "container.stop", EntityType: "container", EntityID: "def"})
	if err := db.QueryRow("SELECT exec_node_id FROM audit_logs WHERE entity_id = 'def'").Scan(&executed); err != nil {
		t.Fatal(err)
	}
	if executed.Valid {
		t.Fatalf("a local action must leave exec_node_id null, got %q", executed.String)
	}
}

// closeTest closes c and fails the test on error, because a swallowed Close
// would hide a leaked connection.
func closeTest(t testing.TB, c io.Closer) {
	t.Helper()
	if err := c.Close(); err != nil {
		t.Errorf("close: %v", err)
	}
}
