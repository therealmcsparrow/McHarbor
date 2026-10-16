// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"net/http"
	"strings"
	"testing"
)

func settingsServer(t *testing.T) *fakeServer {
	t.Helper()
	return groupServer(t, map[string]func(http.ResponseWriter, string){
		"GET /api/settings/security.mfa_required": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{"key": "security.mfa_required", "value": "off", "category": "security", "updatedAt": "2026-09-27T10:00:00Z"})
		},
		"PUT /api/settings/security.mfa_required": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{"key": "security.mfa_required", "value": "admins", "category": "security"})
		},
		"GET /api/settings/new_key": func(w http.ResponseWriter, _ string) {
			fail(w, http.StatusNotFound, "err.not_found", "Not found")
		},
		"PUT /api/settings/new_key": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{"key": "new_key", "value": "1"})
		},
	})
}

func sentPut(f *fakeServer, path string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.requests {
		if r.Method == http.MethodPut && r.URL.Path == path {
			return f.bodies["PUT "+path], true
		}
	}
	return "", false
}

func TestSettingsGetPrintsTheRow(t *testing.T) {
	f := settingsServer(t)
	code, out, stderr := run(t, f, goodKey, "settings", "get", "security.mfa_required")
	if code != ExitOK || !strings.Contains(out, "security.mfa_required") || !strings.Contains(out, "off") {
		t.Fatalf("exit %d, out %q, stderr %q", code, out, stderr)
	}
}

func TestSettingsSetOnASecurityKeyNeedsConfirmation(t *testing.T) {
	f := settingsServer(t)
	code, _, stderr := run(t, f, goodKey, "settings", "set", "security.mfa_required", "admins")
	if code != ExitUsage || !strings.Contains(stderr, "--yes") {
		t.Fatalf("an unconfirmed security change: exit %d, stderr %q", code, stderr)
	}
	if body, sent := sentPut(f, "/api/settings/security.mfa_required"); sent {
		t.Fatalf("a refused change still reached the server: %s", body)
	}
}

func TestSettingsSetKeepsTheExistingCategory(t *testing.T) {
	f := settingsServer(t)
	code, out, stderr := run(t, f, goodKey, "--yes", "settings", "set", "security.mfa_required", "admins")
	if code != ExitOK || !strings.Contains(out, "security.mfa_required: updated") {
		t.Fatalf("exit %d, out %q, stderr %q", code, out, stderr)
	}
	body, _ := sentPut(f, "/api/settings/security.mfa_required")
	if !strings.Contains(body, `"value":"admins"`) || !strings.Contains(body, `"category":"security"`) {
		t.Fatalf("the write dropped the value or the category: %s", body)
	}
}

func TestSettingsSetOnAnOrdinaryKeyNeedsNoConfirmation(t *testing.T) {
	f := settingsServer(t)
	code, _, stderr := run(t, f, goodKey, "settings", "set", "new_key", "1")
	if code != ExitOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if _, sent := sentPut(f, "/api/settings/new_key"); !sent {
		t.Fatal("the change never reached the server")
	}
}

func TestSettingsNameTheMissingPermission(t *testing.T) {
	f := groupServer(t, nil)
	code, _, stderr := run(t, f, goodKey, "settings", "get", "security.mfa_required")
	if code != ExitForbidden || !strings.Contains(stderr, "settings.view") {
		t.Fatalf("get: exit %d, stderr %q", code, stderr)
	}
	code, _, stderr = run(t, f, goodKey, "--yes", "settings", "set", "x", "1", "--category", "general")
	if code != ExitForbidden || !strings.Contains(stderr, "settings.manage") {
		t.Fatalf("set: exit %d, stderr %q", code, stderr)
	}
}

func TestSettingsPostureListsFindingsOrSaysNone(t *testing.T) {
	findings := `[{"envVar":"HSTS_MAX_AGE","severity":"warning","message":"HSTS is off"}]`
	f := groupServer(t, map[string]func(http.ResponseWriter, string){
		"GET /api/settings/security-posture": func(w http.ResponseWriter, _ string) {
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write([]byte(`{"success":true,"data":{"findings":` + findings + `}}`)); err != nil {
				t.Error(err)
			}
		},
	})
	code, out, stderr := run(t, f, goodKey, "settings", "posture")
	if code != ExitOK || !strings.Contains(out, "HSTS_MAX_AGE") || !strings.Contains(out, "HSTS is off") {
		t.Fatalf("exit %d, out %q, stderr %q", code, out, stderr)
	}
	findings = `[]`
	code, out, _ = run(t, f, goodKey, "settings", "posture")
	if code != ExitOK || !strings.Contains(out, "no findings") {
		t.Fatalf("an all-clear posture: exit %d, out %q", code, out)
	}
}
