// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const webhookSecretValue = "whsec_0123456789abcdef"

func webhookSecretServer(t *testing.T) (*fakeServer, *[]string) {
	t.Helper()
	var mu sync.Mutex
	var seen []string
	f := &fakeServer{bodies: map[string]string{}}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Method+" "+r.URL.Path)
		mu.Unlock()
		if r.URL.Path != "/api/gitops/repos/repo1/webhook-secret/" && r.URL.Path != "/api/gitops/repos/repo1/webhook-secret" {
			fail(w, http.StatusNotFound, "err.not_found", "no route "+r.Method+" "+r.URL.Path)
			return
		}
		switch r.Method {
		case http.MethodGet:
			ok(w, map[string]any{"repoId": "repo1", "configured": true})
		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			ok(w, map[string]any{"repoId": "repo1", "secret": webhookSecretValue})
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(f.Close)
	return f, &seen
}

// TestWebhookSecretShowNeverPrintsASecret: show reports status only.
func TestWebhookSecretShowNeverPrintsASecret(t *testing.T) {
	f, _ := webhookSecretServer(t)
	code, stdout, _ := run(t, f, goodKey, "gitops", "webhook-secret", "show", "repo1")
	if code != 0 || !strings.Contains(stdout, "yes") || strings.Contains(stdout, "whsec_") {
		t.Fatalf("exit %d stdout %q; want status without a secret", code, stdout)
	}
}

// TestWebhookSecretRotatePrintsTheSecretOnStdoutOnly: the value is the only
// thing on stdout so it can be piped, and the warning goes to stderr.
func TestWebhookSecretRotatePrintsTheSecretOnStdoutOnly(t *testing.T) {
	f, _ := webhookSecretServer(t)
	code, stdout, stderr := run(t, f, goodKey, "--yes", "gitops", "webhook-secret", "rotate", "repo1")
	if code != 0 || strings.TrimSpace(stdout) != webhookSecretValue {
		t.Fatalf("exit %d stdout %q; want only the secret", code, stdout)
	}
	if strings.Contains(stderr, webhookSecretValue) || !strings.Contains(stderr, "only time") {
		t.Fatalf("stderr %q; want the show-once warning without the secret", stderr)
	}
}

// TestWebhookSecretRotateNeedsConfirmation: no request is sent without --yes.
func TestWebhookSecretRotateNeedsConfirmation(t *testing.T) {
	f, seen := webhookSecretServer(t)
	code, _, _ := run(t, f, goodKey, "gitops", "webhook-secret", "rotate", "repo1")
	if code == 0 || len(*seen) != 0 {
		t.Fatalf("exit %d, requests %v; want a refusal before any request", code, *seen)
	}
}

// TestWebhookSecretRemoveIsIrreversible: --yes is not enough, the repo id must
// be confirmed by name.
func TestWebhookSecretRemoveIsIrreversible(t *testing.T) {
	f, seen := webhookSecretServer(t)
	code, _, stderr := run(t, f, goodKey, "--yes", "gitops", "webhook-secret", "remove", "repo1")
	if code == 0 || len(*seen) != 0 || !strings.Contains(stderr, "--confirm repo1") {
		t.Fatalf("exit %d, requests %v, stderr %q; want a refusal asking for --confirm", code, *seen, stderr)
	}
	code, _, _ = run(t, f, goodKey, "--confirm", "repo1", "gitops", "webhook-secret", "remove", "repo1")
	if code != 0 || len(*seen) != 1 || (*seen)[0] != "DELETE /api/gitops/repos/repo1/webhook-secret" {
		t.Fatalf("exit %d, requests %v; want one DELETE", code, *seen)
	}
}
