// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type automationServer struct {
	*httptest.Server
	mu     sync.Mutex
	bodies []string
}

func newAutomationServer(t *testing.T) *automationServer {
	t.Helper()
	s := &automationServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := valueOr(io.ReadAll(r.Body))
		if r.Method == http.MethodPut && r.URL.Path == "/api/runbooks/rb1/automation" {
			s.mu.Lock()
			s.bodies = append(s.bodies, string(raw))
			s.mu.Unlock()
			ok(w, map[string]any{"id": "rb1"})
			return
		}
		ok(w, map[string]string{"status": "ok"})
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *automationServer) sent() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.bodies...)
}

func runAgainst(t *testing.T, url string, args ...string) int {
	t.Helper()
	code, _, _ := run(t, &fakeServer{Server: &httptest.Server{URL: url}}, goodKey, args...)
	return code
}

// TestRunbookAutomateAsksAndSendsWhatWasChosen: turning on unattended runs is
// Disruptive, so it refuses without --yes off a terminal and sends nothing; with
// --yes it sends exactly the principal and triggers given, and --off clears them.
func TestRunbookAutomateAsksAndSendsWhatWasChosen(t *testing.T) {
	s := newAutomationServer(t)

	if code := runAgainst(t, s.URL, "runbook", "automate", "rb1", "--principal", "svc1", "--on-alert"); code != ExitUsage {
		t.Fatalf("automate without --yes off a terminal: exit %d, want %d", code, ExitUsage)
	}
	if code := runAgainst(t, s.URL, "--yes", "runbook", "automate", "rb1"); code == ExitOK {
		t.Fatal("automate without --principal or --off must be refused")
	}
	if got := s.sent(); len(got) != 0 {
		t.Fatalf("a refused command reached the server: %v", got)
	}

	if code := runAgainst(t, s.URL, "--yes", "runbook", "automate", "rb1", "--principal", "svc1", "--on-alert", "--cron", "0 3 * * *"); code != ExitOK {
		t.Fatalf("automate: exit %d", code)
	}
	if code := runAgainst(t, s.URL, "runbook", "automate", "rb1", "--off"); code != ExitOK {
		t.Fatalf("automate --off: exit %d", code)
	}
	got := s.sent()
	if len(got) != 2 {
		t.Fatalf("want two PUTs, got %d", len(got))
	}
	var on, off runbookAutomationBody
	if err := json.Unmarshal([]byte(got[0]), &on); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(got[1]), &off); err != nil {
		t.Fatal(err)
	}
	if on != (runbookAutomationBody{PrincipalID: "svc1", OnAlert: true, Cron: "0 3 * * *"}) {
		t.Fatalf("automate sent %+v", on)
	}
	if off != (runbookAutomationBody{}) {
		t.Fatalf("--off must clear the principal and both triggers, sent %+v", off)
	}
}
