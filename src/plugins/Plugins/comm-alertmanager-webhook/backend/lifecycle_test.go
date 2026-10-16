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
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

type amPayloadT struct {
	Version     string            `json:"version"`
	GroupKey    string            `json:"groupKey"`
	Status      string            `json:"status"`
	Receiver    string            `json:"receiver"`
	GroupLabels map[string]string `json:"groupLabels"`
	Alerts      []struct {
		Status      string            `json:"status"`
		Labels      map[string]string `json:"labels"`
		Annotations map[string]string `json:"annotations"`
		StartsAt    string            `json:"startsAt"`
		EndsAt      string            `json:"endsAt"`
		Fingerprint string            `json:"fingerprint"`
	} `json:"alerts"`
}

type captured struct {
	mu sync.Mutex
	p  []amPayloadT
}

func (c *captured) all() []amPayloadT {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]amPayloadT(nil), c.p...)
}

func capture(t *testing.T) (*Client, string, *captured) {
	t.Helper()
	got := &captured{}
	c, base := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		var p amPayloadT
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			t.Error(err)
		}
		got.mu.Lock()
		got.p = append(got.p, p)
		got.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
	return c, base, got
}

func ev(key, subject string) incident.Event {
	return incident.Event{DedupKey: key, State: incident.StateFiring, Severity: incident.SeverityCritical,
		Title: "CPU high", AlertName: "CPU", SubjectName: subject, EnvironmentName: "prod", Labels: map[string]string{"team-name": "ops"}}
}

func TestStartsAtStableAcrossLifecycle(t *testing.T) {
	c, base, got := capture(t)
	cfg := map[string]string{"webhook_url": base + "/hook", "allow_http": "true"}
	ref, err := c.Open(context.Background(), cfg, ev("a1:e1:c1", "web"))
	if err != nil {
		t.Fatal(err)
	}
	c.now = func() time.Time { return time.Date(2026, 9, 25, 13, 0, 0, 0, time.UTC) }
	if err := c.Update(context.Background(), cfg, ref, ev("a1:e1:c1", "web")); err != nil {
		t.Fatal(err)
	}
	if err := c.Resolve(context.Background(), cfg, ref, ev("a1:e1:c1", "web")); err != nil {
		t.Fatal(err)
	}
	p := got.all()
	if len(p) != 3 || p[0].Alerts[0].StartsAt != p[1].Alerts[0].StartsAt || p[1].Alerts[0].StartsAt != p[2].Alerts[0].StartsAt {
		t.Fatalf("startsAt drifted: %+v", p)
	}
	if p[0].Alerts[0].StartsAt != "2026-09-25T12:00:00Z" || ref.ExternalID != p[0].Alerts[0].Fingerprint+"@2026-09-25T12:00:00Z" {
		t.Fatalf("open identity wrong: %q %+v", ref.ExternalID, p[0].Alerts[0])
	}
	if p[2].Status != "resolved" || p[2].Alerts[0].Status != "resolved" || p[2].Alerts[0].EndsAt != "2026-09-25T13:00:00Z" {
		t.Fatalf("resolve payload wrong: %+v", p[2])
	}
	if p[1].Status != "firing" || p[1].Alerts[0].EndsAt != zeroTime {
		t.Fatalf("update payload wrong: %+v", p[1])
	}
	if p[0].Version != "4" || p[0].Alerts[0].EndsAt != zeroTime || p[0].Alerts[0].Labels["team_name"] != "ops" || p[0].Receiver != "mcharbor" {
		t.Fatalf("firing payload wrong: %+v", p[0])
	}
	if p[0].GroupKey != p[2].GroupKey || p[0].Alerts[0].Fingerprint != p[2].Alerts[0].Fingerprint {
		t.Fatal("group or fingerprint changed between open and resolve")
	}
}

func TestOccurredAtPinsStartsAt(t *testing.T) {
	c, base, got := capture(t)
	e := ev("k", "web")
	e.OccurredAt = time.Date(2026, 9, 24, 8, 30, 15, 999, time.FixedZone("CEST", 2*3600))
	ref, err := c.Open(context.Background(), map[string]string{"webhook_url": base + "/hook", "allow_http": "true"}, e)
	if err != nil {
		t.Fatal(err)
	}
	if got.all()[0].Alerts[0].StartsAt != "2026-09-24T06:30:15Z" || !strings.HasSuffix(ref.ExternalID, "@2026-09-24T06:30:15Z") {
		t.Fatalf("startsAt not taken from OccurredAt: %q", ref.ExternalID)
	}
}

func TestResolveNeverEndsBeforeStart(t *testing.T) {
	c, base, got := capture(t)
	ref := incident.Ref{ExternalID: fingerprint("k") + "@2026-09-25T14:00:00Z"}
	if err := c.Resolve(context.Background(), map[string]string{"webhook_url": base + "/hook", "allow_http": "true"}, ref, ev("k", "web")); err != nil {
		t.Fatal(err)
	}
	if end := got.all()[0].Alerts[0].EndsAt; end != "2026-09-25T14:00:00Z" {
		t.Fatalf("endsAt %s precedes startsAt", end)
	}
}

func TestDistinctConditionsDistinctGroups(t *testing.T) {
	c, base, got := capture(t)
	cfg := map[string]string{"webhook_url": base + "/hook", "allow_http": "true", "profile": "zenduty"}
	for _, e := range []incident.Event{ev("a1:e1:web", "web"), ev("a1:e1:db", "db")} {
		if _, err := c.Open(context.Background(), cfg, e); err != nil {
			t.Fatal(err)
		}
	}
	p := got.all()
	a, b := p[0], p[1]
	if a.GroupKey == b.GroupKey || a.Alerts[0].Fingerprint == b.Alerts[0].Fingerprint || a.Alerts[0].Labels["alertname"] == b.Alerts[0].Labels["alertname"] {
		t.Fatalf("conditions collapsed: %s / %s", a.GroupKey, b.GroupKey)
	}
	if a.GroupLabels["mcharbor_dedup"] == b.GroupLabels["mcharbor_dedup"] || a.GroupLabels["mcharbor_dedup"] != a.Alerts[0].Fingerprint {
		t.Fatalf("groupLabels not derived from the fingerprint: %v / %v", a.GroupLabels, b.GroupLabels)
	}
	if len(a.Alerts[0].Fingerprint) != 16 || a.GroupKey != `{}:{mcharbor_dedup="`+a.Alerts[0].Fingerprint+`"}` {
		t.Fatalf("fingerprint %q groupKey %q", a.Alerts[0].Fingerprint, a.GroupKey)
	}
	if !strings.HasPrefix(a.Alerts[0].Labels["alertname"], "CPU [") {
		t.Fatalf("zenduty alertname %q", a.Alerts[0].Labels["alertname"])
	}
}

func TestSquadcastProfileFields(t *testing.T) {
	c, base, got := capture(t)
	e := ev("k", "web")
	e.Message = ""
	if _, err := c.Open(context.Background(), map[string]string{"webhook_url": base + "/hook", "allow_http": "true", "profile": "squadcast"}, e); err != nil {
		t.Fatal(err)
	}
	al := got.all()[0].Alerts[0]
	if al.Labels["severity"] == "" || al.Annotations["summary"] == "" || al.Annotations["description"] == "" {
		t.Fatalf("squadcast mandatory fields missing: %+v", al)
	}
}

func TestLabelsSanitizedDeterministically(t *testing.T) {
	for i := 0; i < 20; i++ {
		e := ev("k", "web")
		e.Labels = map[string]string{"a-b": "first", "a.b": "second", "9lives": "cat", "alertname": "spoof", "a_b": "third"}
		p := buildPayload(mustCfg(t, map[string]string{"webhook_url": "https://r.example/"}), e, fingerprint("k"), "2026-09-25T12:00:00Z", statusFiring, time.Time{})
		l := p.Alerts[0].Labels
		if l["a_b"] != "first" || l["_9lives"] != "cat" || l["alertname"] != "CPU" || l["severity"] != "critical" {
			t.Fatalf("labels %v", l)
		}
		for k := range l {
			if labelNameRE.MatchString(k) {
				t.Fatalf("invalid label name %q", k)
			}
		}
	}
}

func TestSeverityMapApplied(t *testing.T) {
	c, base, got := capture(t)
	if _, err := c.Open(context.Background(), map[string]string{"webhook_url": base + "/hook", "allow_http": "true", "severity_map": `{"critical":"P1"}`}, ev("k", "web")); err != nil {
		t.Fatal(err)
	}
	if s := got.all()[0].Alerts[0].Labels["severity"]; s != "P1" {
		t.Fatalf("severity %q", s)
	}
}

func TestForeignRefIsPermanent(t *testing.T) {
	c, base, got := capture(t)
	cfg := map[string]string{"webhook_url": base + "/hook", "allow_http": "true"}
	for _, id := range []string{"", "INC0012345", "zzzzzzzzzzzzzzzz@2026-09-25T12:00:00Z", "0123456789abcdef@yesterday", "0123@2026-09-25T12:00:00Z"} {
		err := c.Resolve(context.Background(), cfg, incident.Ref{ExternalID: id}, ev("k", "web"))
		if err == nil || errors.Is(err, incident.ErrRetryable) {
			t.Fatalf("ref %q: %v", id, err)
		}
	}
	if n := len(got.all()); n != 0 {
		t.Fatalf("%d requests sent for foreign refs", n)
	}
}
