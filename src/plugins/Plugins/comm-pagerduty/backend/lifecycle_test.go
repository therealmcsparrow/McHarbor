// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

const testRoutingKey = "R0UT1NGKEY0000000000000000000000"

func baseCfg() map[string]string {
	return map[string]string{"region": "us", "routing_key": testRoutingKey}
}

type enqueueRecorder struct {
	mu     sync.Mutex
	bodies []map[string]any
	echo   string
}

func (r *enqueueRecorder) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/v2/enqueue" || req.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", req.Method, req.URL.Path)
		}
		var b map[string]any
		if err := json.NewDecoder(req.Body).Decode(&b); err != nil {
			t.Error(err)
		}
		r.mu.Lock()
		r.bodies = append(r.bodies, b)
		echo := r.echo
		r.mu.Unlock()
		if echo == "" {
			echo, _ = b["dedup_key"].(string)
		}
		w.WriteHeader(http.StatusAccepted)
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Event processed", "dedup_key": echo}); err != nil {
			t.Error(err)
		}
	}
}

func fire() incident.Event {
	return incident.Event{DedupKey: "a1:e1:c1", State: incident.StateFiring, Severity: incident.SeverityCritical,
		Title: "CPU high on web", Message: "92% for 5m", Source: "prod/web", EnvironmentName: "prod",
		AlertName: "CPU", SubjectName: "web", Link: "https://mcharbor.example/alerts/history?event=1",
		Labels: map[string]string{"team": "platform"}}
}

func TestOpenIsIdempotentOnDedupKey(t *testing.T) {
	rec := &enqueueRecorder{}
	c, _ := testClient(t, rec.handler(t))
	for i := 0; i < 2; i++ {
		ref, err := c.Open(context.Background(), baseCfg(), fire())
		if err != nil {
			t.Fatal(err)
		}
		if ref.ExternalID != "a1:e1:c1" {
			t.Fatalf("ExternalID = %q", ref.ExternalID)
		}
	}
	if len(rec.bodies) != 2 || rec.bodies[0]["dedup_key"] != "a1:e1:c1" || rec.bodies[0]["dedup_key"] != rec.bodies[1]["dedup_key"] {
		t.Fatalf("retried Open must reuse the host's dedup key verbatim: %v", rec.bodies)
	}
	b := rec.bodies[0]
	p := b["payload"].(map[string]any)
	if b["event_action"] != "trigger" || p["severity"] != "critical" || p["source"] != "prod/web" || b["routing_key"] != testRoutingKey {
		t.Fatalf("bad trigger body: %v", b)
	}
}

func TestOpenRejectsRewrittenDedupKey(t *testing.T) {
	rec := &enqueueRecorder{echo: "something-else"}
	c, _ := testClient(t, rec.handler(t))
	if _, err := c.Open(context.Background(), baseCfg(), fire()); !errors.Is(err, errDedupRewritten) || errors.Is(err, incident.ErrRetryable) {
		t.Fatalf("want permanent rewrite error, got %v", err)
	}
}

func TestSeverityMapOverrideReachesPayload(t *testing.T) {
	rec := &enqueueRecorder{}
	c, _ := testClient(t, rec.handler(t))
	cfg := baseCfg()
	cfg["severity_map"] = `{"warning":"error"}`
	ev := fire()
	ev.Severity = incident.SeverityWarning
	if _, err := c.Open(context.Background(), cfg, ev); err != nil {
		t.Fatal(err)
	}
	if s := rec.bodies[0]["payload"].(map[string]any)["severity"]; s != "error" {
		t.Fatalf("severity %v", s)
	}
}

func TestUpdateRetriggersOnRefKey(t *testing.T) {
	rec := &enqueueRecorder{}
	c, _ := testClient(t, rec.handler(t))
	ev := fire()
	ev.DedupKey = "ignored"
	ev.State = incident.StateUpdate
	if err := c.Update(context.Background(), baseCfg(), incident.Ref{ExternalID: "a1:e1:c1"}, ev); err != nil {
		t.Fatal(err)
	}
	if b := rec.bodies[0]; b["event_action"] != "trigger" || b["dedup_key"] != "a1:e1:c1" {
		t.Fatalf("bad update body: %v", b)
	}
}

func TestResolveSendsResolveOnSameKey(t *testing.T) {
	rec := &enqueueRecorder{}
	c, _ := testClient(t, rec.handler(t))
	ev := fire()
	ev.State = incident.StateResolved
	if err := c.Resolve(context.Background(), baseCfg(), incident.Ref{ExternalID: "a1:e1:c1"}, ev); err != nil {
		t.Fatal(err)
	}
	b := rec.bodies[0]
	if b["event_action"] != "resolve" || b["dedup_key"] != "a1:e1:c1" {
		t.Fatalf("bad resolve body: %v", b)
	}
	if _, has := b["payload"]; has {
		t.Fatalf("resolve must carry no payload: %v", b)
	}
}

func TestResolveWithoutRefSendsNothing(t *testing.T) {
	c, _ := testClient(t, func(_ http.ResponseWriter, r *http.Request) { t.Errorf("unexpected call %s", r.URL.Path) })
	if err := c.Resolve(context.Background(), baseCfg(), incident.Ref{}, fire()); !errors.Is(err, incident.ErrOperatorMessage) {
		t.Fatalf("want operator error, got %v", err)
	}
}

func TestSummaryTruncatedTo1024(t *testing.T) {
	rec := &enqueueRecorder{}
	c, _ := testClient(t, rec.handler(t))
	ev := fire()
	ev.Title = strings.Repeat("x", 3000)
	if _, err := c.Open(context.Background(), baseCfg(), ev); err != nil {
		t.Fatal(err)
	}
	if s := rec.bodies[0]["payload"].(map[string]any)["summary"].(string); len([]rune(s)) > 1024 {
		t.Fatalf("summary length %d", len([]rune(s)))
	}
}
