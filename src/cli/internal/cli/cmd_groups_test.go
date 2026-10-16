// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// groupServer answers the routes the newer command groups use, and denies
// anything under /api/forbidden-* so a 403 can be provoked per operation.
func groupServer(t *testing.T, routes map[string]func(w http.ResponseWriter, body string)) *fakeServer {
	t.Helper()
	f := &fakeServer{bodies: map[string]string{}}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		key := r.Method + " " + r.URL.Path
		f.mu.Lock()
		f.requests = append(f.requests, r.Clone(r.Context()))
		f.bodies[key] = string(raw)
		f.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer "+goodKey {
			fail(w, http.StatusUnauthorized, "err.auth.required", "Authentication required")
			return
		}
		if h, found := routes[key]; found {
			h(w, string(raw))
			return
		}
		fail(w, http.StatusForbidden, "err.rbac.permission_denied", "You do not have permission to perform this action")
	}))
	t.Cleanup(f.Close)
	return f
}

func TestNewGroupsNameTheMissingPermission(t *testing.T) {
	f := groupServer(t, nil)
	cases := []struct {
		args []string
		key  string
	}{
		{[]string{"user", "list"}, "users.view"},
		{[]string{"group", "list"}, "groups.view"},
		{[]string{"api-key", "list"}, "api_keys.view"},
		{[]string{"audit", "list"}, "audit.view"},
		{[]string{"audit", "export", "--format", "csv"}, "audit.export"},
		{[]string{"policy", "list"}, "rbac.policies.view"},
		{[]string{"--yes", "elevation", "approve", "el1"}, "users.manage"},
		{[]string{"plugin", "list"}, "plugins.view"},
		{[]string{"--yes", "ai", "approve", "act1"}, "ai_assistant.act"},
	}
	for _, c := range cases {
		code, _, stderr := run(t, f, goodKey, c.args...)
		if code != ExitForbidden || !strings.Contains(stderr, "missing permission: "+c.key) {
			t.Errorf("%v: exit %d, stderr %q; want 4 naming %s", c.args, code, stderr, c.key)
		}
	}
}

func TestNewIrreversibleCommandsNeedTheTargetNamed(t *testing.T) {
	f := groupServer(t, nil)
	for _, args := range [][]string{
		{"user", "delete", "u1"},
		{"group", "delete", "g1"},
		{"role", "delete", "r1"},
		{"api-key", "revoke", "k1"},
		{"policy", "delete", "p1"},
		{"identity", "provider", "delete", "idp1"},
		{"scim", "delete", "s1"},
	} {
		before := len(f.seen())
		code, _, stderr := run(t, f, goodKey, append([]string{"--yes"}, args...)...)
		if code != ExitUsage || !strings.Contains(stderr, "--confirm "+args[len(args)-1]) {
			t.Errorf("%v with --yes alone: exit %d, stderr %q", args, code, stderr)
		}
		if got := len(f.seen()) - before; got != 0 {
			t.Errorf("%v: a refused change still sent %d requests", args, got)
		}
	}
}

func TestAPIKeyCreatePrintsTheKeyOnceOnStdout(t *testing.T) {
	f := groupServer(t, map[string]func(http.ResponseWriter, string){
		"POST /api/api-keys": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{"id": "k9", "name": "ci", "key": "mch_secretvalue"})
		},
	})
	if code, _, _ := run(t, f, goodKey, "api-key", "create", "ci"); code != ExitUsage {
		t.Fatalf("a key with no grants was not refused locally: exit %d", code)
	}
	code, out, stderr := run(t, f, goodKey, "api-key", "create", "ci", "--scope", "containers.view", "--scope", "volumes.view")
	if code != ExitOK || strings.TrimSpace(out) != "mch_secretvalue" {
		t.Fatalf("exit %d, stdout %q; want only the key", code, out)
	}
	if strings.Contains(stderr, "mch_secretvalue") || !strings.Contains(stderr, "only time") {
		t.Fatalf("stderr %q must warn and must not repeat the key", stderr)
	}
	f.mu.Lock()
	body := f.bodies["POST /api/api-keys"]
	f.mu.Unlock()
	if !strings.Contains(body, `"scopes":["containers.view","volumes.view"]`) || !strings.Contains(body, `"roleIds":[]`) {
		t.Fatalf("request body %s", body)
	}
}

func TestUserCreateTakesThePasswordFromStdinNeverTheCommandLine(t *testing.T) {
	f := groupServer(t, map[string]func(http.ResponseWriter, string){
		"POST /api/users": func(w http.ResponseWriter, _ string) { ok(w, map[string]any{"id": "u9"}) },
	})
	restore, restoreTTY := stdin, stdinIsTerminal
	t.Cleanup(func() { stdin, stdinIsTerminal = restore, restoreTTY })
	stdinIsTerminal = func() bool { return false }
	stdin = strings.NewReader("S3cret-pass!\n")
	if code, _, stderr := run(t, f, goodKey, "user", "create", "dana", "--email", "dana@example.test"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	f.mu.Lock()
	body := f.bodies["POST /api/users"]
	f.mu.Unlock()
	if !strings.Contains(body, `"password":"S3cret-pass!"`) || !strings.Contains(body, `"username":"dana"`) {
		t.Fatalf("request body %s", body)
	}
	if strings.Contains(body, `"roleId"`) {
		t.Fatalf("an unset role was sent: %s", body)
	}
}

func TestElevationListUnwrapsTheRequestsShape(t *testing.T) {
	f := groupServer(t, map[string]func(http.ResponseWriter, string){
		"GET /api/elevations/pending": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{"requests": []map[string]any{{"id": "el7", "permissionKey": "containers.manage", "status": "pending"}}})
		},
	})
	code, out, stderr := run(t, f, goodKey, "elevation", "list", "--show", "pending")
	if code != ExitOK || !strings.Contains(out, "el7") || !strings.Contains(out, "containers.manage") {
		t.Fatalf("exit %d, out %q, stderr %q", code, out, stderr)
	}
}

func TestAuditExportWritesAnOwnerOnlyFile(t *testing.T) {
	f := groupServer(t, map[string]func(http.ResponseWriter, string){
		"GET /api/audit/export": func(w http.ResponseWriter, _ string) {
			w.Header().Set("Content-Type", "text/csv")
			if _, err := w.Write([]byte("id,action\n1,create\n")); err != nil {
				t.Error(err)
			}
		},
	})
	dest := filepath.Join(t.TempDir(), "audit.csv")
	if code, _, stderr := run(t, f, goodKey, "audit", "export", "--format", "csv", "-f", dest); code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	raw, err := os.ReadFile(dest)
	if err != nil || string(raw) != "id,action\n1,create\n" {
		t.Fatalf("file %q, %v", raw, err)
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(dest); err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("the export is readable by others: %v", info.Mode())
		}
	}
}
