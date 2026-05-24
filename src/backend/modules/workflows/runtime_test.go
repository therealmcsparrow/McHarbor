// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandleIncomingWebhookExecutesWorkflowAndReturnsResponse(t *testing.T) {
	db := openWorkflowTestDB(t)
	insertWorkflow(t, db, "wf-webhook", `{
		"nodes": [
			{"id":"webhook-1","type":"trigger","action":"webhook-trigger","label":"Webhook Trigger","config":{"method":"POST","path":"test-hook","secret":"super-secret"}},
			{"id":"response-1","type":"utility","action":"webhook-response","label":"Webhook Response","config":{"status_code":202,"content_type":"text/plain","body":"accepted"}}
		],
		"edges": [
			{"id":"edge-1","sourceNodeId":"webhook-1","sourcePort":"output","targetNodeId":"response-1","targetPort":"input"}
		],
		"groups": []
	}`)

	if _, err := db.Exec(`UPDATE workflows SET status = 'active' WHERE id = ?`, "wf-webhook"); err != nil {
		t.Fatalf("activate workflow: %v", err)
	}

	h := newWorkflowTestHandler(t, db)
	req := httptest.NewRequest(http.MethodPost, "/api/workflows/webhooks/test-hook", strings.NewReader(`{"hello":"world"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Secret", "super-secret")
	w := httptest.NewRecorder()

	h.HandleIncomingWebhook(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202 response, got %d with body %s", w.Code, w.Body.String())
	}
	if body := strings.TrimSpace(w.Body.String()); body != "accepted" {
		t.Fatalf("expected webhook body %q, got %q", "accepted", body)
	}
}

func TestExecuteHTTPRequestHitsServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if got := r.Header.Get("X-Test"); got != "yes" {
			t.Fatalf("expected X-Test header, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"received":` + string(body) + `}`))
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(nil, nil, logger, nil, nil)
	node := &CanvasNode{
		ID:     "http-1",
		Action: "http-request",
		Config: map[string]interface{}{
			"url":         server.URL,
			"method":      "POST",
			"headers":     map[string]interface{}{"X-Test": "yes"},
			"body_source": "payload",
		},
	}
	msg := NewMsg(map[string]interface{}{"value": "ping"})

	port, out, err := svc.executeHTTPRequest(context.Background(), node, msg)
	if err != nil {
		t.Fatalf("execute http request: %v", err)
	}
	if port != "output" {
		t.Fatalf("expected output port, got %q", port)
	}
	if statusCode, _ := out["statusCode"].(int); statusCode != http.StatusCreated {
		t.Fatalf("expected status %d, got %#v", http.StatusCreated, out["statusCode"])
	}
	payload, ok := out["payload"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected object payload, got %#v", out["payload"])
	}
	received, ok := payload["received"].(map[string]interface{})
	if !ok || received["value"] != "ping" {
		t.Fatalf("expected echoed payload, got %#v", payload)
	}
}

func TestHandleExecuteJoinCollectsRangeMessages(t *testing.T) {
	db := openWorkflowTestDB(t)
	insertWorkflow(t, db, "wf-join-range", `{
		"nodes": [
			{"id":"trigger-1","type":"trigger","action":"manual-trigger","label":"Manual Trigger","config":{}},
			{"id":"change-1","type":"utility","action":"change","label":"Change","config":{"action_type":"set","scope":"msg","property":"payload","value":[1,2,3]}},
			{"id":"range-1","type":"logic","action":"range","label":"Range","config":{"property":"payload"}},
			{"id":"join-1","type":"logic","action":"join","label":"Join","config":{"input_count":3,"combine_mode":"array"}}
		],
		"edges": [
			{"id":"edge-1","sourceNodeId":"trigger-1","sourcePort":"output","targetNodeId":"change-1","targetPort":"input"},
			{"id":"edge-2","sourceNodeId":"change-1","sourcePort":"output","targetNodeId":"range-1","targetPort":"input"},
			{"id":"edge-3","sourceNodeId":"range-1","sourcePort":"output","targetNodeId":"join-1","targetPort":"input"}
		],
		"groups": []
	}`)

	h := newWorkflowTestHandler(t, db)
	req := httptest.NewRequest(http.MethodPost, "/api/workflows/wf-join-range/execute", strings.NewReader(`{"triggerNodeId":"trigger-1"}`))
	req = withWorkflowID(req, "wf-join-range")
	w := httptest.NewRecorder()

	h.HandleExecute(w, req)

	body := w.Body.String()
	if !strings.Contains(body, `"nodeId":"join-1"`) {
		t.Fatalf("expected join node execution, got:\n%s", body)
	}
	if !strings.Contains(body, `"payload":[1,2,3]`) {
		t.Fatalf("expected join output array in SSE stream, got:\n%s", body)
	}
}

func TestCronMatchesTime(t *testing.T) {
	now := time.Date(2026, time.April, 5, 12, 30, 0, 0, time.UTC)
	if !cronMatchesTime("30 12 * * *", now) {
		t.Fatalf("expected exact cron to match")
	}
	if !cronMatchesTime("*/15 * * * *", now) {
		t.Fatalf("expected step cron to match")
	}
	if cronMatchesTime("31 12 * * *", now) {
		t.Fatalf("expected non-matching cron to fail")
	}
}
