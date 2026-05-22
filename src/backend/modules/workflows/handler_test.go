// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/config"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

func TestHandleExecuteEmitsDebugNodeEvent(t *testing.T) {
	db := openWorkflowTestDB(t)
	insertWorkflow(t, db, "wf-debug-node", `{
		"nodes": [
			{"id":"trigger-1","type":"trigger","action":"manual-trigger","label":"Manual Trigger","config":{}},
			{"id":"debug-1","type":"action","action":"debug","label":"Debug","config":{"property":"payload"}}
		],
		"edges": [
			{"id":"edge-1","sourceNodeId":"trigger-1","sourcePort":"output","targetNodeId":"debug-1","targetPort":"input"}
		],
		"groups": []
	}`)

	h := newWorkflowTestHandler(t, db)
	req := httptest.NewRequest("POST", "/api/workflows/wf-debug-node/execute", strings.NewReader(`{"triggerNodeId":"trigger-1"}`))
	req = withWorkflowID(req, "wf-debug-node")
	w := httptest.NewRecorder()

	h.HandleExecute(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "event: debug") {
		t.Fatalf("expected debug event in SSE stream, got:\n%s", body)
	}
	if !strings.Contains(body, `"label":"Debug"`) {
		t.Fatalf("expected debug node label in SSE stream, got:\n%s", body)
	}
	if !strings.Contains(body, `"message":"payload"`) {
		t.Fatalf("expected debug payload message in SSE stream, got:\n%s", body)
	}
}

func TestHandleExecuteEmitsNodeDebugFlagEvent(t *testing.T) {
	db := openWorkflowTestDB(t)
	insertWorkflow(t, db, "wf-debug-flag", `{
		"nodes": [
			{"id":"trigger-1","type":"trigger","action":"manual-trigger","label":"Manual Trigger","config":{},"debug":true}
		],
		"edges": [],
		"groups": []
	}`)

	h := newWorkflowTestHandler(t, db)
	req := httptest.NewRequest("POST", "/api/workflows/wf-debug-flag/execute", strings.NewReader(`{"triggerNodeId":"trigger-1"}`))
	req = withWorkflowID(req, "wf-debug-flag")
	w := httptest.NewRecorder()

	h.HandleExecute(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "event: debug") {
		t.Fatalf("expected debug event in SSE stream, got:\n%s", body)
	}
	if !strings.Contains(body, `"label":"Manual Trigger"`) {
		t.Fatalf("expected trigger label in SSE stream, got:\n%s", body)
	}
	if !strings.Contains(body, `"message":"Node debug output"`) {
		t.Fatalf("expected generic node debug message in SSE stream, got:\n%s", body)
	}
}

func TestHandleExecuteSkipsBlockedOutputPort(t *testing.T) {
	db := openWorkflowTestDB(t)
	insertWorkflow(t, db, "wf-blocked-output", `{
		"nodes": [
			{"id":"trigger-1","type":"trigger","action":"manual-trigger","label":"Manual Trigger","config":{},"blockedPorts":["out:output"]},
			{"id":"debug-1","type":"action","action":"debug","label":"Debug","config":{"property":"payload"}}
		],
		"edges": [
			{"id":"edge-1","sourceNodeId":"trigger-1","sourcePort":"output","targetNodeId":"debug-1","targetPort":"input"}
		],
		"groups": []
	}`)

	h := newWorkflowTestHandler(t, db)
	req := httptest.NewRequest("POST", "/api/workflows/wf-blocked-output/execute", strings.NewReader(`{"triggerNodeId":"trigger-1"}`))
	req = withWorkflowID(req, "wf-blocked-output")
	w := httptest.NewRecorder()

	h.HandleExecute(w, req)

	body := w.Body.String()
	if strings.Contains(body, `"nodeId":"debug-1"`) {
		t.Fatalf("expected blocked output to prevent downstream execution, got:\n%s", body)
	}
	if strings.Contains(body, `"edgeId":"edge-1"`) {
		t.Fatalf("expected blocked output to prevent edge traversal, got:\n%s", body)
	}
}

func TestHandleExecuteSkipsBlockedInputPort(t *testing.T) {
	db := openWorkflowTestDB(t)
	insertWorkflow(t, db, "wf-blocked-input", `{
		"nodes": [
			{"id":"trigger-1","type":"trigger","action":"manual-trigger","label":"Manual Trigger","config":{}},
			{"id":"debug-1","type":"action","action":"debug","label":"Debug","config":{"property":"payload"},"blockedPorts":["in:input"]}
		],
		"edges": [
			{"id":"edge-1","sourceNodeId":"trigger-1","sourcePort":"output","targetNodeId":"debug-1","targetPort":"input"}
		],
		"groups": []
	}`)

	h := newWorkflowTestHandler(t, db)
	req := httptest.NewRequest("POST", "/api/workflows/wf-blocked-input/execute", strings.NewReader(`{"triggerNodeId":"trigger-1"}`))
	req = withWorkflowID(req, "wf-blocked-input")
	w := httptest.NewRecorder()

	h.HandleExecute(w, req)

	body := w.Body.String()
	if strings.Contains(body, `"nodeId":"debug-1"`) {
		t.Fatalf("expected blocked input to prevent downstream execution, got:\n%s", body)
	}
	if !strings.Contains(body, `"edgeId":"edge-1"`) {
		t.Fatalf("expected blocked input to preserve edge traversal, got:\n%s", body)
	}
}

func TestHandleExecuteSendNotificationUsesInternalOnly(t *testing.T) {
	db := openWorkflowTestDB(t)
	insertWorkflow(t, db, "wf-send-notification", `{
		"nodes": [
			{"id":"trigger-1","type":"trigger","action":"manual-trigger","label":"Manual Trigger","config":{}},
			{"id":"notify-1","type":"action","action":"send-notification","label":"Send Internal Notification","config":{"title":"Workflow done","message":"Internal notice","channel_id":"legacy-channel","channel_type":"slack"}}
		],
		"edges": [
			{"id":"edge-1","sourceNodeId":"trigger-1","sourcePort":"output","targetNodeId":"notify-1","targetPort":"input"}
		],
		"groups": []
	}`)

	h := newWorkflowTestHandler(t, db)
	req := httptest.NewRequest("POST", "/api/workflows/wf-send-notification/execute", strings.NewReader(`{"triggerNodeId":"trigger-1"}`))
	req = withWorkflowID(req, "wf-send-notification")
	w := httptest.NewRecorder()

	h.HandleExecute(w, req)

	body := w.Body.String()
	if !strings.Contains(body, `"channelType":"internal"`) {
		t.Fatalf("expected internal notification channel type in SSE stream, got:\n%s", body)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM in_app_notifications`).Scan(&count); err != nil {
		t.Fatalf("count in_app_notifications: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 in-app notification, got %d", count)
	}
}

func TestHandleExecuteTryCatchRoutesNodeErrorOutput(t *testing.T) {
	db := openWorkflowTestDB(t)
	insertWorkflow(t, db, "wf-try-catch", `{
		"nodes": [
			{"id":"trigger-1","type":"trigger","action":"manual-trigger","label":"Manual Trigger","config":{}},
			{"id":"try-1","type":"logic","action":"try-catch","label":"Try Catch","config":{"error_property":"error"}},
			{"id":"read-1","type":"utility","action":"read-file","label":"Read File","config":{"path":"missing.txt","output_property":"payload"}},
			{"id":"debug-1","type":"utility","action":"debug","label":"Debug Catch","config":{"property":"error.message"}}
		],
		"edges": [
			{"id":"edge-1","sourceNodeId":"trigger-1","sourcePort":"output","targetNodeId":"try-1","targetPort":"input"},
			{"id":"edge-2","sourceNodeId":"try-1","sourcePort":"output","targetNodeId":"read-1","targetPort":"input"},
			{"id":"edge-catch","sourceNodeId":"try-1","sourcePort":"catch","targetNodeId":"debug-1","targetPort":"input"}
		],
		"groups": []
	}`)

	h := newWorkflowTestHandler(t, db)
	req := httptest.NewRequest("POST", "/api/workflows/wf-try-catch/execute", strings.NewReader(`{"triggerNodeId":"trigger-1"}`))
	req = withWorkflowID(req, "wf-try-catch")
	w := httptest.NewRecorder()

	h.HandleExecute(w, req)

	body := w.Body.String()
	if !strings.Contains(body, `"nodeId":"read-1"`) {
		t.Fatalf("expected failing node id in SSE stream, got:\n%s", body)
	}
	if !strings.Contains(body, `"outputPort":"error"`) {
		t.Fatalf("expected read node to emit its error output, got:\n%s", body)
	}
	if !strings.Contains(body, `"edgeId":"edge-catch"`) {
		t.Fatalf("expected catch edge traversal in SSE stream, got:\n%s", body)
	}
	if !strings.Contains(body, `"label":"Debug Catch"`) {
		t.Fatalf("expected catch branch node execution, got:\n%s", body)
	}
	if !strings.Contains(body, `"status":"completed"`) {
		t.Fatalf("expected workflow to complete after catch handling, got:\n%s", body)
	}
}

func TestHandleExecuteMetricRecordPersistsWorkflowMetric(t *testing.T) {
	db := openWorkflowTestDB(t)
	insertWorkflow(t, db, "wf-metric-record", `{
		"nodes": [
			{"id":"trigger-1","type":"trigger","action":"manual-trigger","label":"Manual Trigger","config":{}},
			{"id":"change-1","type":"utility","action":"change","label":"Change","config":{"action_type":"set","scope":"msg","property":"payload","value":42}},
			{"id":"metric-1","type":"utility","action":"metric-record","label":"Metric Record","config":{"metric_name":"workflow.duration","property":"payload","metric_type":"gauge","unit":"ms","labels":{"scope":"workflow","source":"test"}}}
		],
		"edges": [
			{"id":"edge-1","sourceNodeId":"trigger-1","sourcePort":"output","targetNodeId":"change-1","targetPort":"input"},
			{"id":"edge-2","sourceNodeId":"change-1","sourcePort":"output","targetNodeId":"metric-1","targetPort":"input"}
		],
		"groups": []
	}`)

	h := newWorkflowTestHandler(t, db)
	req := httptest.NewRequest("POST", "/api/workflows/wf-metric-record/execute", strings.NewReader(`{"triggerNodeId":"trigger-1"}`))
	req = withWorkflowID(req, "wf-metric-record")
	w := httptest.NewRecorder()

	h.HandleExecute(w, req)

	var (
		metricName   string
		metricType   string
		sourceProp   string
		unit         string
		valueJSON    string
		numericValue float64
		labelsJSON   string
	)
	if err := db.QueryRow(
		`SELECT metric_name, metric_type, source_property, unit, value_json, numeric_value, labels
		 FROM workflow_metrics
		 WHERE workflow_id = ?
		 LIMIT 1`,
		"wf-metric-record",
	).Scan(&metricName, &metricType, &sourceProp, &unit, &valueJSON, &numericValue, &labelsJSON); err != nil {
		t.Fatalf("select workflow metric: %v", err)
	}

	if metricName != "workflow.duration" {
		t.Fatalf("expected metric name workflow.duration, got %q", metricName)
	}
	if metricType != "gauge" {
		t.Fatalf("expected gauge metric type, got %q", metricType)
	}
	if sourceProp != "payload" {
		t.Fatalf("expected payload source property, got %q", sourceProp)
	}
	if unit != "ms" {
		t.Fatalf("expected unit ms, got %q", unit)
	}
	if valueJSON != "42" {
		t.Fatalf("expected value_json 42, got %q", valueJSON)
	}
	if numericValue != 42 {
		t.Fatalf("expected numeric value 42, got %v", numericValue)
	}
	if !strings.Contains(labelsJSON, `"scope":"workflow"`) || !strings.Contains(labelsJSON, `"source":"test"`) {
		t.Fatalf("expected stored labels json, got %q", labelsJSON)
	}
}

func TestExecuteSQLQueryRejectsDisallowedTable(t *testing.T) {
	db := openWorkflowTestDB(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(db, nil, logger, nil)

	node := &CanvasNode{
		Config: map[string]interface{}{
			"query":           "SELECT id FROM in_app_notifications",
			"output_property": "payload",
		},
	}

	port, out, err := svc.executeSQLQuery(context.Background(), node, NewMsg(nil))
	if err != nil {
		t.Fatalf("execute sql query: %v", err)
	}
	if port != "error" {
		t.Fatalf("expected error port, got %q", port)
	}

	payload, ok := out["payload"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected payload map, got %#v", out["payload"])
	}
	if payload["error"] != `table "in_app_notifications" is not available to workflow SQL queries` {
		t.Fatalf("unexpected error payload: %#v", payload)
	}
}

func TestExecuteSQLQueryCapsResultRows(t *testing.T) {
	db := openWorkflowTestDB(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(db, nil, logger, nil)

	for i := 0; i < sqlQueryMaxRows+5; i++ {
		if _, err := db.Exec(
			`INSERT INTO workflow_runs (id, workflow_id, status, trigger, duration_ms, node_count, error, started_at, finished_at)
			 VALUES (?, 'wf-limit', 'completed', 'manual', 1, 1, '', '2026-04-02T00:00:00Z', '2026-04-02T00:00:01Z')`,
			fmt.Sprintf("run-%03d", i),
		); err != nil {
			t.Fatalf("insert workflow run %d: %v", i, err)
		}
	}

	node := &CanvasNode{
		Config: map[string]interface{}{
			"query":           "SELECT id FROM workflow_runs ORDER BY id ASC",
			"output_property": "payload",
		},
	}

	port, out, err := svc.executeSQLQuery(context.Background(), node, NewMsg(nil))
	if err != nil {
		t.Fatalf("execute sql query: %v", err)
	}
	if port != "output" {
		t.Fatalf("expected output port, got %q", port)
	}

	rows, ok := out["payload"].([]interface{})
	if !ok {
		t.Fatalf("expected result rows, got %#v", out["payload"])
	}
	if len(rows) != sqlQueryMaxRows {
		t.Fatalf("expected %d rows, got %d", sqlQueryMaxRows, len(rows))
	}
}

func newWorkflowTestHandler(t *testing.T, db *sql.DB) *Handler {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	app := &router.AppDeps{
		Config: &config.Config{DataDir: t.TempDir()},
		DB:     db,
		Logger: logger,
	}

	return NewHandler(app, NewHub())
}

func openWorkflowTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	schema := []string{
		`CREATE TABLE workflows (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			canvas_data TEXT NOT NULL,
			variables TEXT NOT NULL DEFAULT '{}',
			created_by TEXT NOT NULL DEFAULT '',
			updated_by TEXT NOT NULL DEFAULT '',
			last_run_at TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE workflow_runs (
			id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			status TEXT NOT NULL,
			trigger TEXT NOT NULL,
			duration_ms INTEGER NOT NULL DEFAULT 0,
			node_count INTEGER NOT NULL DEFAULT 0,
			error TEXT NOT NULL DEFAULT '',
			started_at TEXT NOT NULL,
			finished_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE in_app_notifications (
			id TEXT PRIMARY KEY,
			level TEXT NOT NULL DEFAULT 'info',
			title TEXT NOT NULL,
			message TEXT NOT NULL,
			action TEXT,
			entity_type TEXT,
			entity_id TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE workflow_kv (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			expires_at TEXT,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE workflow_link_messages (
			id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			node_id TEXT NOT NULL,
			name TEXT NOT NULL DEFAULT '',
			msg TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE workflow_metrics (
			id TEXT PRIMARY KEY,
			workflow_id TEXT,
			node_id TEXT NOT NULL,
			metric_name TEXT NOT NULL,
			metric_type TEXT NOT NULL,
			source_property TEXT NOT NULL DEFAULT 'payload',
			unit TEXT NOT NULL DEFAULT '',
			value_json TEXT NOT NULL DEFAULT 'null',
			numeric_value REAL,
			labels TEXT NOT NULL DEFAULT '{}',
			recorded_at TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT ''
		)`,
	}

	for _, stmt := range schema {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("create schema: %v", err)
		}
	}

	return db
}

func insertWorkflow(t *testing.T, db *sql.DB, id, canvasData string) {
	t.Helper()

	var compact bytes.Buffer
	if err := jsonCompact(&compact, []byte(canvasData)); err != nil {
		t.Fatalf("compact canvas data: %v", err)
	}

	if _, err := db.Exec(
		`INSERT INTO workflows (id, name, description, status, canvas_data, variables, created_by, updated_by, last_run_at, created_at, updated_at)
		 VALUES (?, ?, '', 'draft', ?, '{}', '', '', '', '2026-04-02T00:00:00Z', '2026-04-02T00:00:00Z')`,
		id,
		id,
		compact.String(),
	); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
}

func jsonCompact(dst *bytes.Buffer, src []byte) error {
	dst.Reset()
	return json.Compact(dst, src)
}

func withWorkflowID(req *http.Request, id string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}
