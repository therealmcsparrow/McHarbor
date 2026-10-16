// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"net/http"
	"strings"
	"testing"
)

func TestPluginInstallSendsUnsignedConsentOnlyWhenAsked(t *testing.T) {
	f := groupServer(t, map[string]func(http.ResponseWriter, string){
		"GET /api/appstore/plugins/catalog": func(w http.ResponseWriter, _ string) {
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write([]byte(`{"success":true,"data":` + pluginCatalog + `}`)); err != nil {
				t.Error(err)
			}
		},
		"POST /api/appstore/plugins/install/stream": func(w http.ResponseWriter, _ string) {
			sse(w, `{"phase":"error","status":"error","error_code":"err.plugin.unsigned_confirmation_required","error_text":"plugin is unsigned and the install was not confirmed"}`)
		},
		"POST /api/appstore/plugins/comm-ntfy/update/stream": func(w http.ResponseWriter, _ string) {
			sse(w, `{"phase":"done","step":100,"total":100,"status":"done","result":{"slug":"comm-ntfy","version":"2026.09.02"}}`)
		},
	})
	body := func(key string) string {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.bodies[key]
	}

	code, _, stderr := run(t, f, goodKey, "--yes", "plugin", "install", "comm-ntfy", "--version", "2026.09.01")
	if code != ExitRemote || !strings.Contains(stderr, "err.plugin.unsigned_confirmation_required") {
		t.Fatalf("a refused unsigned install: exit %d, stderr %q", code, stderr)
	}
	if strings.Contains(body("POST /api/appstore/plugins/install/stream"), `"allow_unsigned":true`) {
		t.Fatalf("consent was sent without --allow-unsigned: %s", body("POST /api/appstore/plugins/install/stream"))
	}

	run(t, f, goodKey, "--yes", "plugin", "install", "comm-ntfy", "--version", "2026.09.01", "--allow-unsigned")
	if !strings.Contains(body("POST /api/appstore/plugins/install/stream"), `"allow_unsigned":true`) {
		t.Fatalf("--allow-unsigned did not reach the server: %s", body("POST /api/appstore/plugins/install/stream"))
	}

	if code, _, _ := run(t, f, goodKey, "--yes", "plugin", "update", "comm-ntfy", "--allow-unsigned"); code != ExitOK {
		t.Fatalf("update --allow-unsigned: exit %d", code)
	}
	if !strings.Contains(body("POST /api/appstore/plugins/comm-ntfy/update/stream"), `"allow_unsigned":true`) {
		t.Fatalf("update --allow-unsigned did not reach the server: %s", body("POST /api/appstore/plugins/comm-ntfy/update/stream"))
	}
}
