// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// appStreamServer answers the two app streams. An install of "broken" fails with
// the server's own message; every other slug installs as stack "<slug>-stack".
func appStreamServer(t *testing.T, bodies *[]map[string]any) *fakeServer {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		send := func(v map[string]any) {
			raw, err := json.Marshal(v)
			if err != nil {
				t.Errorf("encoding event: %v", err)
				return
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", raw); err != nil {
				t.Errorf("writing event: %v", err)
			}
		}
		switch {
		case r.URL.Path == "/api/app-store/install/stream":
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("reading body: %v", err)
			}
			var body map[string]any
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("decoding body: %v", err)
			}
			*bodies = append(*bodies, body)
			send(map[string]any{"step": 1, "total": 2, "status": "progress", "message": "generating compose"})
			if body["slug"] == "broken" {
				send(map[string]any{"status": "error", "message": "port 8080 is already published on this host"})
				return
			}
			send(map[string]any{"step": 2, "total": 2, "status": "done", "message": "installed", "stackName": fmt.Sprint(body["slug"]) + "-stack"})
		case strings.HasSuffix(r.URL.Path, "/uninstall/stream"):
			send(map[string]any{"status": "done", "message": "removed"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return &fakeServer{Server: srv}
}

func TestAppInstallFollowsTheStreamAndReportsTheStack(t *testing.T) {
	var bodies []map[string]any
	f := appStreamServer(t, &bodies)

	code, out, stderr := run(t, f, goodKey, "--yes", "app", "install", "nginx", "--name", "web", "--env-var", "TZ=UTC")
	if code != ExitOK {
		t.Fatalf("install exit %d, stderr %q", code, stderr)
	}
	if !strings.Contains(out, "nginx: installed as stack nginx-stack") {
		t.Fatalf("output %q does not name the stack", out)
	}
	if len(bodies) != 1 || bodies[0]["name"] != "web" || bodies[0]["envVars"].(map[string]any)["TZ"] != "UTC" {
		t.Fatalf("request body = %+v", bodies)
	}
}

func TestAppInstallExitsWithTheServersReason(t *testing.T) {
	var bodies []map[string]any
	f := appStreamServer(t, &bodies)
	code, _, stderr := run(t, f, goodKey, "--yes", "app", "install", "broken")
	if code != ExitRemote || !strings.Contains(stderr, "port 8080 is already published on this host") {
		t.Fatalf("got exit %d, stderr %q", code, stderr)
	}
}

func TestAppUninstallNeedsConfirmation(t *testing.T) {
	var bodies []map[string]any
	f := appStreamServer(t, &bodies)
	if code, _, _ := run(t, f, goodKey, "app", "uninstall", "inst-1"); code == ExitOK {
		t.Fatal("an irreversible uninstall must not run without confirmation")
	}
	if code, _, stderr := run(t, f, goodKey, "--yes", "--confirm", "inst-1", "app", "uninstall", "inst-1"); code != ExitOK {
		t.Fatalf("confirmed uninstall: exit %d, stderr %q", code, stderr)
	}
}
