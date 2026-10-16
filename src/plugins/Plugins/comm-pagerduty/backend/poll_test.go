// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

func TestPollGroupedAlertIsNotNotFound(t *testing.T) {
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Token token=RESTKEY" || r.URL.Query().Get("date_range") != "all" {
			t.Errorf("bad poll request %q %s", r.Header.Get("Authorization"), r.URL.RawQuery)
		}
		body := `{"incidents":[{"id":"P1","incident_number":7,"status":"resolved","incident_key":"live","html_url":"https://x/incidents/P1"}],"more":false}`
		if r.URL.Query().Get("incident_key") == "grouped" {
			body = `{"incidents":[],"more":false}`
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Error(err)
		}
	})
	cfg := baseCfg()
	cfg["rest_api_key"] = "RESTKEY"
	st, err := c.Poll(context.Background(), cfg, []incident.Ref{{ExternalID: "grouped"}, {ExternalID: "live"}})
	if err != nil {
		t.Fatalf("grouped alert must not error: %v", err)
	}
	if len(st) != 1 || st[0].DedupKey != "live" || st[0].State != incident.RemoteResolved || st[0].Ref.Number != "#7" {
		t.Fatalf("statuses %+v", st)
	}
}

func TestPollPicksNewestIncident(t *testing.T) {
	c, _ := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		body := `{"incidents":[
			{"id":"P2","incident_number":9,"status":"triggered","incident_key":"k","created_at":"2026-09-25T11:00:00Z","last_status_change_at":"2026-09-25T11:00:00Z"},
			{"id":"P1","incident_number":8,"status":"resolved","incident_key":"k","created_at":"2026-09-24T10:00:00Z","last_status_change_at":"2026-09-24T12:00:00Z"}
		],"more":false}`
		if _, err := w.Write([]byte(body)); err != nil {
			t.Error(err)
		}
	})
	cfg := baseCfg()
	cfg["rest_api_key"] = "RESTKEY"
	st, err := c.Poll(context.Background(), cfg, []incident.Ref{{ExternalID: "k"}})
	if err != nil || len(st) != 1 || st[0].State != incident.RemoteOpen || st[0].Ref.Number != "#9" {
		t.Fatalf("want only the newest incident: %+v %v", st, err)
	}
	if !st[0].UpdatedAt.Equal(time.Date(2026, 9, 25, 11, 0, 0, 0, time.UTC)) {
		t.Fatalf("UpdatedAt = %v", st[0].UpdatedAt)
	}
}

func TestPollWithoutRESTKeyMakesNoCall(t *testing.T) {
	c, _ := testClient(t, func(_ http.ResponseWriter, r *http.Request) { t.Errorf("unexpected call %s", r.URL.Path) })
	st, err := c.Poll(context.Background(), baseCfg(), []incident.Ref{{ExternalID: "k"}})
	if err != nil || len(st) != 0 {
		t.Fatalf("%v %v", st, err)
	}
}

func TestPollUnauthorizedIsPermanent(t *testing.T) {
	c, _ := testClient(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) })
	cfg := baseCfg()
	cfg["rest_api_key"] = "RESTKEY"
	_, err := c.Poll(context.Background(), cfg, []incident.Ref{{ExternalID: "a"}, {ExternalID: "b"}})
	if err == nil || errors.Is(err, incident.ErrRetryable) || errors.Is(err, incident.ErrNotFound) {
		t.Fatalf("want permanent error, got %v", err)
	}
}
