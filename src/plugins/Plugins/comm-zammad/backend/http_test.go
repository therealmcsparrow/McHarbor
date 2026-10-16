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

func fakeZammad(t *testing.T, h http.HandlerFunc) (*httptest.Server, map[string]string) {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	return srv, map[string]string{
		"instance_url": srv.URL, "access_token": "tok", "group": "Operations",
		"customer": "mcharbor@example.de", "resolve_state": "closed", "on_behalf_of": "ops-bot",
	}
}

func write(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	if _, err := w.Write([]byte(body)); err != nil {
		t.Error(err)
	}
}

func TestTokenAndFromHeaders(t *testing.T) {
	srv, cfg := fakeZammad(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Token token=tok" || r.Header.Get("From") != "ops-bot" {
			t.Errorf("headers: %v", r.Header)
		}
		write(t, w, `{"id":1,"login":"ops-bot"}`)
	})
	parsed, err := parseConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var me map[string]any
	if err := NewClient(srv.Client()).do(context.Background(), parsed, http.MethodGet, "/api/v1/users/me", nil, &me); err != nil {
		t.Fatal(err)
	}
}

func TestForbiddenGroupIsOperatorMessage(t *testing.T) {
	for _, code := range []int{http.StatusUnprocessableEntity, http.StatusForbidden} {
		srv, cfg := fakeZammad(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
			write(t, w, `{"error":"Not authorized","error_human":"Not authorized"}`)
		})
		parsed, err := parseConfig(cfg)
		if err != nil {
			t.Fatal(err)
		}
		err = NewClient(srv.Client()).do(context.Background(), parsed, http.MethodPost, "/api/v1/tickets", map[string]any{}, nil)
		if !errors.Is(err, incident.ErrOperatorMessage) || errors.Is(err, incident.ErrRetryable) || !strings.Contains(err.Error(), "Operations") {
			t.Fatalf("status %d: err = %v", code, err)
		}
	}
}

func TestStatusClassification(t *testing.T) {
	cases := []struct {
		code   int
		header string
		body   string
		check  func(error) bool
	}{
		{429, "7", "", func(err error) bool {
			var ra *incident.RetryAfterError
			return errors.As(err, &ra) && ra.After == 7*time.Second && errors.Is(err, incident.ErrRetryable)
		}},
		{503, "", "", func(err error) bool { return errors.Is(err, incident.ErrRetryable) }},
		{404, "", "", func(err error) bool { return errors.Is(err, incident.ErrNotFound) }},
		{401, "", `{"error":"S3CRET-TOKEN-VALUE is invalid"}`, func(err error) bool {
			return errors.Is(err, incident.ErrOperatorMessage) && strings.Contains(err.Error(), "access token rejected")
		}},
		{422, "", `{"error":"x","error_human":"Missing required value for field 'resolution' S3CRET-TOKEN-VALUE"}`, func(err error) bool {
			return errors.Is(err, incident.ErrOperatorMessage) && strings.Contains(err.Error(), "resolution") && !errors.Is(err, incident.ErrRetryable)
		}},
		{409, "", "", func(err error) bool {
			return !errors.Is(err, incident.ErrRetryable) && !errors.Is(err, incident.ErrOperatorMessage)
		}},
	}
	for _, tc := range cases {
		srv, cfg := fakeZammad(t, func(w http.ResponseWriter, _ *http.Request) {
			if tc.header != "" {
				w.Header().Set("Retry-After", tc.header)
			}
			w.WriteHeader(tc.code)
			write(t, w, tc.body)
		})
		cfg["access_token"] = "S3CRET-TOKEN-VALUE"
		parsed, err := parseConfig(cfg)
		if err != nil {
			t.Fatal(err)
		}
		err = NewClient(srv.Client()).do(context.Background(), parsed, http.MethodPost, "/api/v1/tickets", map[string]any{}, nil)
		if err == nil || !tc.check(err) {
			t.Errorf("status %d: unexpected classification %v", tc.code, err)
		}
		if err != nil && strings.Contains(err.Error(), "S3CRET") {
			t.Errorf("status %d: token leaked: %v", tc.code, err)
		}
	}
}

func TestRedirectRefused(t *testing.T) {
	srv, cfg := fakeZammad(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://elsewhere.invalid/", http.StatusFound)
	})
	parsed, err := parseConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	c := NewClient(nil)
	c.hc.Transport = srv.Client().Transport
	err = c.do(context.Background(), parsed, http.MethodGet, "/api/v1/users/me", nil, nil)
	if !errors.Is(err, incident.ErrOperatorMessage) || errors.Is(err, incident.ErrRetryable) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseConfigRequiresHTTPS(t *testing.T) {
	base := map[string]string{"access_token": "t", "group": "g", "customer": "c"}
	for _, u := range []string{"http://z.lan", "file:///etc/passwd", "https://", "https://u:p@z.lan", "z.lan", "https://z.lan/?x=1"} {
		cfg := map[string]string{"instance_url": u}
		for k, v := range base {
			cfg[k] = v
		}
		if _, err := parseConfig(cfg); !errors.Is(err, incident.ErrOperatorMessage) {
			t.Errorf("%q accepted: %v", u, err)
		}
	}
	cfg := map[string]string{"instance_url": "https://z.lan/zammad/"}
	for k, v := range base {
		cfg[k] = v
	}
	got, err := parseConfig(cfg)
	if err != nil || got.base != "https://z.lan/zammad" || got.resolveState != "closed" {
		t.Fatalf("got %+v err %v", got, err)
	}
	cfg["status_map"] = `{"waiting":"sleeping"}`
	if _, err := parseConfig(cfg); !errors.Is(err, incident.ErrOperatorMessage) {
		t.Fatalf("bad status_map accepted: %v", err)
	}
	cfg["status_map"] = ""
	cfg["severity_map"] = `{"crit":"3 high"}`
	if _, err := parseConfig(cfg); !errors.Is(err, incident.ErrOperatorMessage) {
		t.Fatalf("bad severity_map accepted: %v", err)
	}
}
