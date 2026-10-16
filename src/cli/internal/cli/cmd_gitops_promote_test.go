// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// promoteServer answers the promote route with a pending promotion and the
// history route with that promotion still pending on the first read and in its
// final state afterwards, which is how the server's background deploy looks.
func promoteServer(t *testing.T, final map[string]any) *fakeServer {
	t.Helper()
	var mu sync.Mutex
	reads := 0
	f := &fakeServer{bodies: map[string]string{}}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+goodKey {
			fail(w, http.StatusUnauthorized, "err.auth.required", "Authentication required")
			return
		}
		switch {
		case r.URL.Path == "/api/gitops/pipelines/pipe1/promote" && r.Method == http.MethodPost:
			ok(w, map[string]any{"id": "prom1", "status": "pending", "stackName": "gitops-web-staging"})
		case r.URL.Path == "/api/gitops/pipelines/pipe1/promotions":
			mu.Lock()
			reads++
			n := reads
			mu.Unlock()
			row := map[string]any{"id": "prom1", "status": "pending"}
			if n > 1 {
				row = final
			}
			ok(w, map[string]any{"items": []map[string]any{{"id": "older", "status": "succeeded"}, row}, "total": 2, "page": 1, "perPage": 100})
		default:
			fail(w, http.StatusNotFound, "err.not_found", "no route "+r.Method+" "+r.URL.Path)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

// TestPromoteWaitFailsOnAFailedDeploy: a CI job that promotes with --wait exits 7
// with the server's recorded reason when the stack did not come up.
func TestPromoteWaitFailsOnAFailedDeploy(t *testing.T) {
	restore := pollInterval
	pollInterval = 10 * time.Millisecond
	t.Cleanup(func() { pollInterval = restore })

	f := promoteServer(t, map[string]any{"id": "prom1", "status": "failed", "lastError": "docker compose up failed: pull access denied"})
	code, _, stderr := run(t, f, goodKey, "--yes", "gitops", "pipeline", "promote", "pipe1", "--stage", "s1", "--wait")
	if code != ExitRemote || !strings.Contains(stderr, "pull access denied") {
		t.Fatalf("exit %d stderr %q; want 7 with the recorded reason", code, stderr)
	}
}

// TestPromoteWaitSucceedsOnASucceededDeploy prints the final row and exits 0.
func TestPromoteWaitSucceedsOnASucceededDeploy(t *testing.T) {
	restore := pollInterval
	pollInterval = 10 * time.Millisecond
	t.Cleanup(func() { pollInterval = restore })

	f := promoteServer(t, map[string]any{"id": "prom1", "status": "succeeded", "stackName": "gitops-web-staging"})
	code, out, stderr := run(t, f, goodKey, "--yes", "gitops", "pipeline", "promote", "pipe1", "--stage", "s1", "--wait")
	if code != ExitOK || !strings.Contains(out, "succeeded") || !strings.Contains(out, "gitops-web-staging") {
		t.Fatalf("exit %d out %q stderr %q", code, out, stderr)
	}
}

// TestPromoteWithoutWaitDoesNotPoll keeps the default a fire-and-report action.
func TestPromoteWithoutWaitDoesNotPoll(t *testing.T) {
	f := promoteServer(t, map[string]any{"id": "prom1", "status": "failed"})
	code, _, stderr := run(t, f, goodKey, "--yes", "gitops", "pipeline", "promote", "pipe1", "--stage", "s1")
	if code != ExitOK {
		t.Fatalf("exit %d stderr %q", code, stderr)
	}
}
