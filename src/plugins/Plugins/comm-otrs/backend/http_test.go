// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

func TestHTTP200ErrorEnvelope(t *testing.T) {
	f, srv := newFakeGI(t)
	f.handlers["POST /Ticket"] = func(map[string]any) any {
		return map[string]any{"Error": map[string]any{"ErrorCode": "TicketCreate.InvalidParameter", "ErrorMessage": "TicketCreate: Ticket->Queue is invalid!\x07"}}
	}
	c := NewClient()
	cfg, err := parseConfig(testConfig(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	var out createResp
	err = c.call(context.Background(), cfg, "/Ticket", "POST", map[string]any{}, &out)
	if !errors.Is(err, incident.ErrOperatorMessage) {
		t.Fatalf("want ErrOperatorMessage, got %v", err)
	}
	if errors.Is(err, incident.ErrRetryable) || strings.ContainsRune(err.Error(), '\x07') {
		t.Fatalf("must be permanent and sanitised: %q", err)
	}
	if !strings.Contains(err.Error(), "Queue is invalid") {
		t.Fatalf("operator text lost: %q", err)
	}
	_, err = c.Open(context.Background(), testConfig(srv.URL), firing())
	if err == nil {
		t.Fatal("Open decoded an error envelope as a created ticket")
	}
}

func TestHTTP200ErrorEnvelopeRedactsSecrets(t *testing.T) {
	f, srv := newFakeGI(t)
	f.handlers["POST /Ticket"] = func(map[string]any) any {
		return map[string]any{"Error": map[string]any{"ErrorCode": "TicketCreate.InvalidParameter", "ErrorMessage": "bad value pw for S1"}}
	}
	cfg, err := parseConfig(testConfig(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	err = NewClient().call(context.Background(), cfg, "/Ticket", "POST", map[string]any{}, nil)
	if err == nil || strings.Contains(err.Error(), " pw ") || strings.Contains(err.Error(), "S1") {
		t.Fatalf("secret echoed: %v", err)
	}
}

func TestSessionReloginOnAuthFail(t *testing.T) {
	f, srv := newFakeGI(t)
	f.handlers["POST /TicketGet"] = func(map[string]any) any { return map[string]any{"Ticket": []any{}} }
	c := NewClient()
	cfg, err := parseConfig(testConfig(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := c.call(ctx, cfg, "/TicketGet", "POST", map[string]any{"TicketID": "1"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := c.call(ctx, cfg, "/TicketGet", "POST", map[string]any{"TicketID": "1"}, nil); err != nil {
		t.Fatal(err)
	}
	logins := 0
	f.locked(func() { logins = f.logins })
	if logins != 1 {
		t.Fatalf("session not cached: logins = %d", logins)
	}
	f.locked(func() { f.sessionOK = "S2" })
	if err := c.call(ctx, cfg, "/TicketGet", "POST", map[string]any{"TicketID": "1"}, nil); err != nil {
		t.Fatalf("expired session must re-login once: %v", err)
	}
	f.locked(func() { logins = f.logins })
	if logins != 2 {
		t.Fatalf("logins = %d, want 2", logins)
	}
	cfg.Password = "wrong"
	c.dropSession(cfg)
	err = c.call(ctx, cfg, "/TicketGet", "POST", map[string]any{"TicketID": "1"}, nil)
	if err == nil || errors.Is(err, incident.ErrRetryable) || strings.Contains(err.Error(), "wrong") {
		t.Fatalf("bad password must be permanent and not echo the secret: %v", err)
	}
	if !errors.Is(err, incident.ErrOperatorMessage) {
		t.Fatalf("bad password must tell the operator: %v", err)
	}
}

func TestSessionAuthFailTwiceIsPermanent(t *testing.T) {
	f, srv := newFakeGI(t)
	f.handlers["POST /TicketGet"] = func(map[string]any) any { return map[string]any{"Ticket": []any{}} }
	f.handlers["POST /Session"] = func(map[string]any) any {
		f.logins++
		return map[string]any{"SessionID": "never-valid"}
	}
	cfg, err := parseConfig(testConfig(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	err = NewClient().call(context.Background(), cfg, "/TicketGet", "POST", map[string]any{"TicketID": "1"}, nil)
	if err == nil || errors.Is(err, incident.ErrRetryable) || !errors.Is(err, incident.ErrOperatorMessage) {
		t.Fatalf("want permanent operator error, got %v", err)
	}
	if n := f.count("POST /TicketGet"); n != 2 {
		t.Fatalf("TicketGet attempts = %d, want exactly 2", n)
	}
}

func TestSessionCacheIsPerCredential(t *testing.T) {
	_, srv := newFakeGI(t)
	c := NewClient()
	good, err := parseConfig(testConfig(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.session(context.Background(), good); err != nil {
		t.Fatal(err)
	}
	bad := good
	bad.Password = "other"
	if _, err := c.session(context.Background(), bad); err == nil {
		t.Fatal("a channel with a wrong password reused another channel's session")
	}
}

func TestSessionConcurrentUse(t *testing.T) {
	f, srv := newFakeGI(t)
	f.handlers["POST /TicketGet"] = func(map[string]any) any { return map[string]any{"Ticket": []any{}} }
	c := NewClient()
	var wg sync.WaitGroup
	var failures atomic.Int32
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cfg, err := parseConfig(testConfig(srv.URL))
			if err != nil {
				failures.Add(1)
				return
			}
			if err := c.call(context.Background(), cfg, "/TicketGet", "POST", map[string]any{"TicketID": "1"}, nil); err != nil {
				failures.Add(1)
			}
		}()
	}
	wg.Wait()
	if failures.Load() != 0 {
		t.Fatalf("%d concurrent calls failed", failures.Load())
	}
}

func TestTransportStatusMapping(t *testing.T) {
	for code, want := range map[int]error{429: incident.ErrRetryable, 503: incident.ErrRetryable, 404: incident.ErrNotFound} {
		if got := statusError(code, ""); !errors.Is(got, want) {
			t.Errorf("%d: got %v", code, got)
		}
	}
	if got := statusError(400, ""); errors.Is(got, incident.ErrRetryable) {
		t.Errorf("400 must be permanent: %v", got)
	}
	var ra *incident.RetryAfterError
	if !errors.As(statusError(429, "7"), &ra) || ra.After.Seconds() != 7 {
		t.Fatalf("Retry-After not honoured: %v", ra)
	}
}

func TestRouteMissingIsOperatorMessageNotOrphan(t *testing.T) {
	_, srv := newFakeGI(t)
	m := testConfig(srv.URL)
	m["webservice"] = "Typo"
	err := NewClient().Update(context.Background(), m, incident.Ref{ExternalID: "42"}, firing())
	if !errors.Is(err, incident.ErrOperatorMessage) || errors.Is(err, incident.ErrNotFound) {
		t.Fatalf("a missing web service must not orphan the link: %v", err)
	}
}

func TestParseConfigRejectsUnsafeURLs(t *testing.T) {
	allowHTTP = false
	defer func() { allowHTTP = true }()
	for _, u := range []string{"http://helpdesk.example.com", "https://user:pw@helpdesk.example.com", "https://helpdesk.example.com?x=1", "helpdesk.example.com", "ftp://h", ""} {
		m := testConfig(u)
		if _, err := parseConfig(m); err == nil || !errors.Is(err, incident.ErrOperatorMessage) {
			t.Errorf("%q accepted: %v", u, err)
		}
	}
	m := testConfig("https://helpdesk.example.com/")
	cfg, err := parseConfig(m)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Endpoint != "https://helpdesk.example.com/otrs/nph-genericinterface.pl/Webservice/McHarborConnectorREST" {
		t.Fatalf("endpoint %s", cfg.Endpoint)
	}
	m["flavour"] = "otobo"
	if cfg, err = parseConfig(m); err != nil || !strings.Contains(cfg.Endpoint, "/otobo/nph-genericinterface.pl/") {
		t.Fatalf("otobo script path: %s %v", cfg.Endpoint, err)
	}
	m["dedup_field"] = "Bad Field"
	if _, err := parseConfig(m); err == nil {
		t.Fatal("dedup field with a space accepted")
	}
}

func TestRedirectIsNotFollowed(t *testing.T) {
	var hits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
	}))
	t.Cleanup(target.Close)
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(src.Close)
	err := NewClient().Test(context.Background(), testConfig(src.URL))
	if err == nil || hits.Load() != 0 {
		t.Fatalf("redirect followed with the password: hits=%d err=%v", hits.Load(), err)
	}
}
