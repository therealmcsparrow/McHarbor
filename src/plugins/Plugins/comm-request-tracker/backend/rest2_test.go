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

const testToken = "1-14-abcdef"

func fakeRT(t *testing.T, h http.HandlerFunc) (*httptest.Server, map[string]string) {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "token "+testToken {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"message":"Unauthorized"}`))
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/rt/REST/2.0/") {
			t.Errorf("path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, map[string]string{
		"instance_url": srv.URL + "/rt", "auth_token": testToken, "queue": "Operations",
		"dedup_cf_name": "McHarbor Dedup Key", "resolve_status": "resolved",
	}
}

func TestConfigRejectsQuoteInQueue(t *testing.T) {
	_, cfg := fakeRT(t, func(http.ResponseWriter, *http.Request) {})
	cfg["queue"] = "Ops' OR Queue != '"
	if _, err := parseConfig(cfg); !errors.Is(err, incident.ErrOperatorMessage) {
		t.Fatalf("err = %v", err)
	}
	cfg["queue"] = "Operations"
	cfg["dedup_cf_name"] = "Key}' = '' OR 'CF.{x"
	if _, err := parseConfig(cfg); !errors.Is(err, incident.ErrOperatorMessage) {
		t.Fatalf("cf name err = %v", err)
	}
	cfg["dedup_cf_name"] = ""
	c, err := parseConfig(cfg)
	if err != nil || c.cfName != defaultCFName || c.queue != "Operations" {
		t.Fatalf("valid config: %+v %v", c, err)
	}
}

func TestSQLQuoteEscapes(t *testing.T) {
	if got := sqlQuote(`a'b\c`); got != `'a\'b\\c'` {
		t.Fatalf("sqlQuote = %s", got)
	}
}

func TestBaseURLRefusesNonHTTPS(t *testing.T) {
	bad := []string{"http://rt.example.edu", "file:///etc/passwd", "https://user:pw@rt.example.edu", "https://rt.example.edu/?x=1", "rt.example.edu", "https://"}
	for _, u := range bad {
		if _, err := baseURL(u); !errors.Is(err, incident.ErrOperatorMessage) {
			t.Errorf("%q accepted: %v", u, err)
		}
	}
	if got, err := baseURL("https://rt.example.edu/rt/REST/2.0/"); err != nil || got != "https://rt.example.edu/rt" {
		t.Fatalf("got %q %v", got, err)
	}
}

func TestInvalidStatusIsOperatorMessage(t *testing.T) {
	srv, cfg := fakeRT(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"message":"Status 'resolved' isn't a valid status for this ticket."}`))
	})
	err := NewClient(srv.Client()).Resolve(context.Background(), cfg, incident.Ref{ExternalID: "77"},
		incident.Event{State: incident.StateResolved, Title: "ok"})
	if !errors.Is(err, incident.ErrOperatorMessage) || !strings.Contains(err.Error(), "resolve_status") || !strings.Contains(err.Error(), "Operations") {
		t.Fatalf("err = %v", err)
	}
	if errors.Is(err, incident.ErrRetryable) {
		t.Fatal("an invalid status must be permanent")
	}
}

func TestDoClassifiesStatus(t *testing.T) {
	cases := []struct {
		code   int
		method string
		path   string
		check  func(error) bool
	}{
		{401, http.MethodGet, "/ticket/1", func(err error) bool {
			return errors.Is(err, incident.ErrOperatorMessage) && strings.Contains(err.Error(), "auth token rejected")
		}},
		{403, http.MethodPost, "/ticket", func(err error) bool {
			return errors.Is(err, incident.ErrOperatorMessage) && strings.Contains(err.Error(), "CreateTicket") && strings.Contains(err.Error(), "Operations")
		}},
		{403, http.MethodPost, "/ticket/1/comment", func(err error) bool { return strings.Contains(err.Error(), "CommentOnTicket") }},
		{403, http.MethodPut, "/ticket/1", func(err error) bool { return strings.Contains(err.Error(), "ModifyTicket") }},
		{404, http.MethodGet, "/ticket/1", func(err error) bool { return errors.Is(err, incident.ErrNotFound) }},
		{400, http.MethodPost, "/ticket", func(err error) bool {
			return errors.Is(err, incident.ErrOperatorMessage) && strings.Contains(err.Error(), "Queue is required") && !errors.Is(err, incident.ErrRetryable)
		}},
		{429, http.MethodGet, "/tickets", func(err error) bool {
			var ra *incident.RetryAfterError
			return errors.As(err, &ra) && ra.After == 7*time.Second && errors.Is(err, incident.ErrRetryable)
		}},
		{503, http.MethodGet, "/tickets", func(err error) bool { return errors.Is(err, incident.ErrRetryable) }},
		{302, http.MethodGet, "/tickets", func(err error) bool {
			return errors.Is(err, incident.ErrOperatorMessage) && strings.Contains(err.Error(), "redirect")
		}},
	}
	for _, tc := range cases {
		srv, raw := fakeRT(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "7")
			if tc.code == 302 {
				w.Header().Set("Location", "https://evil.example/")
			}
			w.WriteHeader(tc.code)
			w.Write([]byte(`{"message":"Queue is required; token ` + testToken + `"}`))
		})
		if tc.code == 401 {
			raw["auth_token"] = "wrong"
		}
		cfg, err := parseConfig(raw)
		if err != nil {
			t.Fatal(err)
		}
		_, err = NewClient(srv.Client()).do(context.Background(), cfg, tc.method, tc.path, map[string]string{"a": "b"}, nil)
		if err == nil || !tc.check(err) {
			t.Errorf("%d %s %s: unexpected classification %v", tc.code, tc.method, tc.path, err)
		}
		if err != nil && strings.Contains(err.Error(), testToken) {
			t.Errorf("%d: token leaked into error: %v", tc.code, err)
		}
	}
}

func TestTransportErrorIsRetryable(t *testing.T) {
	srv, raw := fakeRT(t, func(http.ResponseWriter, *http.Request) {})
	srv.Close()
	cfg, err := parseConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewClient(srv.Client()).do(context.Background(), cfg, http.MethodGet, "/tickets", nil, nil); !errors.Is(err, incident.ErrRetryable) {
		t.Fatalf("err = %v", err)
	}
}

func TestTablesRejectTypos(t *testing.T) {
	if _, err := statusTable(`{"working":"acknowleged"}`); !errors.Is(err, incident.ErrOperatorMessage) {
		t.Fatalf("status typo accepted: %v", err)
	}
	st, err := statusTable(`{"Working":"acknowledged"}`)
	if err != nil || st["working"] != incident.RemoteAcknowledged || st["resolved"] != incident.RemoteResolved {
		t.Fatalf("status override: %v %v", st, err)
	}
	if _, err := priorityTable(`{"crit":90}`); !errors.Is(err, incident.ErrOperatorMessage) {
		t.Fatalf("severity typo accepted: %v", err)
	}
	p, err := priorityTable(`{"critical":99,"info":"5"}`)
	if err != nil || p[incident.SeverityCritical] != 99 || p[incident.SeverityInfo] != 5 || p[incident.SeverityWarning] != 50 {
		t.Fatalf("priority override: %v %v", p, err)
	}
}
