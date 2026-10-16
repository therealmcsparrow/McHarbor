// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestPluginReconcilePinsTheVersion: the command sends the exact version and the
// enabled flag, and refuses to run without --version rather than guessing one.
func TestPluginReconcilePinsTheVersion(t *testing.T) {
	var got map[string]any
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading body: %v", err)
		}
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Errorf("decoding body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"success":true,"data":{"slug":"comm-ntfy","version":"2026.09.02","action":"updated"}}`)); err != nil {
			t.Errorf("writing response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	f := &fakeServer{Server: srv}

	if code, _, _ := run(t, f, goodKey, "--yes", "plugin", "reconcile", "comm-ntfy"); code != ExitUsage {
		t.Fatalf("without --version: exit %d, want usage", code)
	}
	code, _, stderr := run(t, f, goodKey, "--yes", "plugin", "reconcile", "comm-ntfy", "--version", "2026.09.02", "--disabled")
	if code != ExitOK {
		t.Fatalf("reconcile exit %d, stderr %q", code, stderr)
	}
	if path != "/api/appstore/plugins/comm-ntfy/reconcile" || got["version"] != "2026.09.02" || got["enabled"] != false {
		t.Fatalf("request %s %+v", path, got)
	}
}
