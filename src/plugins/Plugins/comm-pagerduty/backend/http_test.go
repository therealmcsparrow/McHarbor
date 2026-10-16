// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

func testClient(t *testing.T, h http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := NewClient()
	c.hc = srv.Client()
	c.eventsBase = srv.URL
	c.restBase = srv.URL
	c.now = func() time.Time { return time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC) }
	return c, srv
}

func TestDoJSONClassifiesStatus(t *testing.T) {
	cases := []struct {
		code   int
		header string
		check  func(error) bool
	}{
		{429, "7", func(err error) bool {
			var ra *incident.RetryAfterError
			return errors.As(err, &ra) && ra.After == 7*time.Second && errors.Is(err, incident.ErrRetryable)
		}},
		{503, "", func(err error) bool { return errors.Is(err, incident.ErrRetryable) }},
		{404, "", func(err error) bool { return errors.Is(err, incident.ErrNotFound) }},
		{400, "", func(err error) bool {
			return errors.Is(err, incident.ErrOperatorMessage) && !errors.Is(err, incident.ErrRetryable) &&
				strings.Contains(err.Error(), "Event object is invalid") && strings.Contains(err.Error(), "[redacted]")
		}},
		{401, "", func(err error) bool {
			return !errors.Is(err, incident.ErrRetryable) && !errors.Is(err, incident.ErrOperatorMessage) && !errors.Is(err, incident.ErrNotFound)
		}},
	}
	for _, tc := range cases {
		c, srv := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			if tc.header != "" {
				w.Header().Set("Retry-After", tc.header)
			}
			w.WriteHeader(tc.code)
			_, err := w.Write([]byte(`{"status":"invalid event","message":"Event object is invalid","errors":["routing_key R0UT1NGKEY0000000000000000000000 is invalid"]}`))
			if err != nil {
				t.Error(err)
			}
		})
		err := c.doJSON(context.Background(), http.MethodPost, srv.URL+"/v2/enqueue", nil, map[string]string{"a": "b"}, nil, "R0UT1NGKEY0000000000000000000000")
		if err == nil || !tc.check(err) {
			t.Errorf("status %d: unexpected classification %v", tc.code, err)
		}
		if err != nil && strings.Contains(err.Error(), "R0UT1NGKEY") {
			t.Errorf("status %d: secret leaked into error: %v", tc.code, err)
		}
	}
}

func TestPDValidationTextReadsRESTShape(t *testing.T) {
	got := pdValidationText([]byte(`{"error":{"message":"Invalid Input Provided","code":2001,"errors":["Key SECRETKEY bad"]}}`), []string{"SECRETKEY"})
	if got != "Invalid Input Provided; Key [redacted] bad" {
		t.Fatalf("got %q", got)
	}
	if got := pdValidationText([]byte(`not json SECRETKEY`), []string{"SECRETKEY"}); strings.Contains(got, "SECRETKEY") {
		t.Fatalf("raw body leaked: %q", got)
	}
}

func TestParseConfigValidatesOffline(t *testing.T) {
	bad := []map[string]string{
		{"region": "ap", "routing_key": testRoutingKey},
		{"region": "us", "routing_key": "short"},
		{"region": "us", "routing_key": testRoutingKey, "severity_map": `{"warning":"high"}`},
		{"region": "us", "routing_key": testRoutingKey, "severity_map": `{"warn":"error"}`},
		{"region": "us", "routing_key": testRoutingKey, "severity_map": `not json`},
	}
	for _, cfg := range bad {
		if _, err := parseConfig(cfg); !errors.Is(err, incident.ErrOperatorMessage) {
			t.Errorf("config %v accepted: %v", cfg, err)
		}
	}
	cfg, err := parseConfig(map[string]string{"routing_key": testRoutingKey, "severity_map": `{"warning":"error"}`})
	if err != nil || cfg.Region != "us" || cfg.SeverityMap[incident.SeverityWarning] != "error" || cfg.SeverityMap[incident.SeverityCritical] != "critical" {
		t.Fatalf("defaults/override: %+v %v", cfg, err)
	}
}

func TestRegionPicksHosts(t *testing.T) {
	c := NewClient()
	eu := pdConfig{Region: "eu"}
	us := pdConfig{Region: "us"}
	if c.eventsURL(eu) != "https://events.eu.pagerduty.com" || c.restURL(eu) != "https://api.eu.pagerduty.com" {
		t.Fatal("eu hosts")
	}
	if c.eventsURL(us) != "https://events.pagerduty.com" || c.restURL(us) != "https://api.pagerduty.com" {
		t.Fatal("us hosts")
	}
}
