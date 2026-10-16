// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// TestStackComposeRevealIsOptIn keeps `stack compose` on the UI's terms: the masked file by
// default, the audited values only with --reveal.
func TestStackComposeRevealIsOptIn(t *testing.T) {
	var mu sync.Mutex
	var reveals []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/health":
			ok(w, map[string]string{"status": "ok"})
		case "/api/about":
			ok(w, map[string]any{"version": "2026.09.01", "build": "202609250000"})
		case "/api/stacks/web/compose":
			mu.Lock()
			reveals = append(reveals, r.URL.Query().Get("reveal"))
			mu.Unlock()
			ok(w, map[string]string{"content": "services: {}\n"})
		default:
			fail(w, http.StatusNotFound, "err.not_found", "no route "+r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	f := &fakeServer{Server: srv}

	for _, args := range [][]string{{"stack", "compose", "web"}, {"stack", "compose", "web", "--reveal"}} {
		if code, _, stderr := run(t, f, goodKey, args...); code != ExitOK {
			t.Fatalf("%v: exit %d: %s", args, code, stderr)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(reveals) != 2 || reveals[0] != "" || reveals[1] != "true" {
		t.Fatalf("reveal query per call = %q, want [\"\" \"true\"]", reveals)
	}
}
