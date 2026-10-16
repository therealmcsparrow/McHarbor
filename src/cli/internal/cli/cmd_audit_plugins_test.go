// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestAuditSearchSendsTheServerFilters(t *testing.T) {
	var got url.Values
	f := groupServer(t, map[string]func(http.ResponseWriter, string){
		"GET /api/audit": func(w http.ResponseWriter, _ string) { ok(w, map[string]any{"items": []any{}, "total": 0}) },
	})
	f.Config.Handler = recordQuery(f.Config.Handler, "/api/audit", &got)

	code, _, stderr := run(t, f, goodKey, "audit", "search", "--action", "container.remove", "--action", "stack.down",
		"--entity-type", "container", "--search", "alice", "--exact", "--since", "7d")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if a := got["action"]; len(a) != 2 || a[0] != "container.remove" || a[1] != "stack.down" {
		t.Fatalf("actions sent %v; both must reach the server", a)
	}
	if got.Get("entity_type") != "container" || got.Get("search") != "alice" || got.Get("search_mode") != "exact" {
		t.Fatalf("filters sent %v", got)
	}
	from, err := time.Parse(time.RFC3339, got.Get("from"))
	if err != nil || time.Since(from) < 7*24*time.Hour-time.Minute || time.Since(from) > 7*24*time.Hour+time.Minute {
		t.Fatalf("--since 7d sent from=%q", got.Get("from"))
	}

	if code, _, _ := run(t, f, goodKey, "audit", "search", "--since", "1d", "--from", "2026-09-01T00:00:00Z"); code != ExitUsage {
		t.Fatalf("--since with --from: exit %d, want 2", code)
	}
	if code, _, _ := run(t, f, goodKey, "audit", "search", "--from", "yesterday"); code != ExitUsage {
		t.Fatalf("a non-RFC 3339 --from: exit %d, want 2", code)
	}
}

// recordQuery captures the query string of every request to path before the
// fake server answers it.
func recordQuery(next http.Handler, path string, into *url.Values) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == path {
			*into = r.URL.Query()
		}
		next.ServeHTTP(w, r)
	})
}

const pluginCatalog = `[{"slug":"comm-ntfy","name":"ntfy","version":"2026.09.02","versions":[
	{"version":"2026.09.02","download_url":"https://catalog/ntfy-2.tgz","checksum":"abc","signature":"sig2"},
	{"version":"2026.09.01","download_url":"https://catalog/ntfy-1.tgz","checksum":"def"}]}]`

func sse(w http.ResponseWriter, events ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	for _, e := range events {
		if _, err := w.Write([]byte("data: " + e + "\n\n")); err != nil {
			return
		}
	}
}

func TestPluginInstallFollowsTheStreamWithTheCatalogEntry(t *testing.T) {
	f := groupServer(t, map[string]func(http.ResponseWriter, string){
		"GET /api/appstore/plugins/catalog": func(w http.ResponseWriter, _ string) {
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write([]byte(`{"success":true,"data":` + pluginCatalog + `}`)); err != nil {
				t.Error(err)
			}
		},
		"POST /api/appstore/plugins/install/stream": func(w http.ResponseWriter, _ string) {
			sse(w, `{"phase":"download","step":40,"total":100,"status":"progress","message":"fetching"}`,
				`{"phase":"done","step":100,"total":100,"status":"done","result":{"slug":"comm-ntfy","version":"2026.09.01"}}`)
		},
	})
	code, out, stderr := run(t, f, goodKey, "--yes", "plugin", "install", "comm-ntfy", "--version", "2026.09.01")
	if code != ExitOK || !strings.Contains(out, "comm-ntfy: installed 2026.09.01") || !strings.Contains(stderr, "download 40% fetching") {
		t.Fatalf("exit %d, out %q, stderr %q", code, out, stderr)
	}
	f.mu.Lock()
	body := f.bodies["POST /api/appstore/plugins/install/stream"]
	f.mu.Unlock()
	if !strings.Contains(body, `"url":"https://catalog/ntfy-1.tgz"`) || !strings.Contains(body, `"checksum":"def"`) {
		t.Fatalf("installed the wrong catalog entry: %s", body)
	}

	if code, _, _ := run(t, f, goodKey, "--yes", "plugin", "install", "comm-nope"); code != ExitNotFound {
		t.Fatalf("an unknown slug: exit %d, want 5", code)
	}
}

func TestPluginStreamErrorsAndDenialsAreReported(t *testing.T) {
	f := groupServer(t, map[string]func(http.ResponseWriter, string){
		"POST /api/appstore/plugins/comm-ntfy/update/stream": func(w http.ResponseWriter, _ string) {
			sse(w, `{"phase":"error","status":"error","error_code":"err.plugin.signature_invalid","error_text":"signature does not verify"}`)
		},
		"POST /api/appstore/plugins/uninstall": func(w http.ResponseWriter, _ string) { ok(w, map[string]any{}) },
	})
	code, _, stderr := run(t, f, goodKey, "--yes", "plugin", "update", "comm-ntfy")
	if code != ExitRemote || !strings.Contains(stderr, "err.plugin.signature_invalid") || !strings.Contains(stderr, "signature does not verify") {
		t.Fatalf("a failed update: exit %d, stderr %q", code, stderr)
	}
	code, _, stderr = run(t, f, goodKey, "--yes", "plugin", "update", "comm-other")
	if code != ExitForbidden || !strings.Contains(stderr, "missing permission: plugins.manage") {
		t.Fatalf("a denied update: exit %d, stderr %q", code, stderr)
	}

	before := len(f.seen())
	if code, _, stderr := run(t, f, goodKey, "--yes", "plugin", "uninstall", "comm-ntfy", "--purge-data"); code != ExitUsage || !strings.Contains(stderr, "--confirm comm-ntfy") {
		t.Fatalf("purge without naming it: exit %d, %q", code, stderr)
	}
	if got := len(f.seen()) - before; got != 0 {
		t.Fatalf("a refused purge sent %d requests", got)
	}
	if code, _, stderr := run(t, f, goodKey, "--yes", "plugin", "uninstall", "comm-ntfy"); code != ExitOK {
		t.Fatalf("a plain uninstall: exit %d, %s", code, stderr)
	}
	f.mu.Lock()
	body := f.bodies["POST /api/appstore/plugins/uninstall"]
	f.mu.Unlock()
	if !strings.Contains(body, `"purge_data":false`) {
		t.Fatalf("a plain uninstall asked to purge: %s", body)
	}
}
