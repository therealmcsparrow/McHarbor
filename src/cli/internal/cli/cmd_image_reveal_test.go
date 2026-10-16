// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// TestImageGetRevealIsOptIn keeps `image get` on the UI's terms: a plain get asks for the
// masked Config.Env, and only --reveal asks for the values the server audits.
func TestImageGetRevealIsOptIn(t *testing.T) {
	var mu sync.Mutex
	var reveals []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/health":
			ok(w, map[string]string{"status": "ok"})
		case "/api/about":
			ok(w, map[string]any{"version": "2026.09.01", "build": "202609250000"})
		case "/api/images/app:1":
			mu.Lock()
			reveals = append(reveals, r.URL.Query().Get("reveal"))
			mu.Unlock()
			ok(w, map[string]any{"Id": "sha256:abcdef1234567890", "RepoTags": []string{"app:1"}, "Os": "linux"})
		default:
			fail(w, http.StatusNotFound, "err.not_found", "no route "+r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	f := &fakeServer{Server: srv}

	for _, args := range [][]string{{"image", "get", "app:1"}, {"image", "get", "app:1", "--reveal"}} {
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
