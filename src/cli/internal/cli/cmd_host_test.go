// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func hostServiceServer(t *testing.T) *fakeServer {
	t.Helper()
	return groupServer(t, map[string]func(http.ResponseWriter, string){
		"GET /api/host-services": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{
				"availability": map[string]any{"supported": true, "minVersion": "2026.09.01", "minBuild": "202609271515"},
				"units": []map[string]any{
					{"name": "nginx.service", "activeState": "failed", "subState": "failed", "unitFileState": "enabled", "failureReason": "result: exit-code; main process exited with status 1"},
					{"name": "sshd.service", "activeState": "active", "subState": "running", "unitFileState": "enabled", "mainPid": 812, "protected": "ssh"},
				},
			})
		},
		"POST /api/host-services/nginx.service/restart": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{"unit": "nginx.service", "action": "restart"})
		},
		"POST /api/host-services/sshd.service/stop": func(w http.ResponseWriter, _ string) {
			fail(w, http.StatusConflict, "err.host_services.protected", "That unit is protected: McHarbor will not stop or disable it")
		},
		"GET /api/host-services/nginx.service/override": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{"unit": "nginx.service", "effective": "# /lib/systemd/system/nginx.service\n[Service]\nExecStart=/usr/sbin/nginx\n", "exists": false})
		},
		"PUT /api/host-services/nginx.service/override": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{"unit": "nginx.service", "path": "/etc/systemd/system/nginx.service.d/override.conf", "reloaded": true, "verifyOutput": "Unknown key name 'Restartt'"})
		},
		"DELETE /api/host-services/nginx.service/override": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{"code": "msg.host_services.override_removed"})
		},
		"POST /api/host-services/daemon-reload": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{"code": "msg.host_services.reloaded"})
		},
	})
}

func TestHostServiceListShowsStateAndFailure(t *testing.T) {
	f := hostServiceServer(t)
	code, stdout, stderr := run(t, f, goodKey, "host", "service", "list", "--env", "e1")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	for _, want := range []string{"nginx.service", "failed", "main process exited with status 1", "sshd.service", "ssh"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("list output lacks %q:\n%s", want, stdout)
		}
	}
	if q := f.seen()[len(f.seen())-1].URL.Query().Get("env"); q != "e1" {
		t.Fatalf("env query = %q", q)
	}
}

func TestHostServiceMutationsReachTheirRoutes(t *testing.T) {
	f := hostServiceServer(t)
	dir := t.TempDir()
	override := filepath.Join(dir, "override.conf")
	if err := os.WriteFile(override, []byte("[Service]\nRestartt=always\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"host", "service", "restart", "nginx.service", "--env", "e1", "--yes"},
		{"host", "service", "edit", "nginx.service", "--file", override, "--verify", "--env", "e1", "--yes"},
		{"host", "service", "revert", "nginx.service", "--env", "e1", "--confirm", "nginx.service"},
		{"host", "service", "daemon-reload", "--env", "e1", "--yes"},
		{"host", "service", "cat", "nginx.service", "--env", "e1"},
	} {
		if code, _, stderr := run(t, f, goodKey, args...); code != ExitOK {
			t.Fatalf("%v: exit %d: %s", args, code, stderr)
		}
	}
	if body := f.bodies["PUT /api/host-services/nginx.service/override"]; body != `{"content":"[Service]\nRestartt=always\n","reload":true,"verify":true}` {
		t.Fatalf("edit body = %s", body)
	}
}

func TestHostServiceProtectedStopSurfacesTheRefusal(t *testing.T) {
	f := hostServiceServer(t)
	code, _, stderr := run(t, f, goodKey, "host", "service", "stop", "sshd.service", "--env", "e1", "--yes")
	if code != ExitConflict || !strings.Contains(stderr, "protected") {
		t.Fatalf("exit %d: %s", code, stderr)
	}
}

func TestHostServiceRefusesBadInputBeforeSendingAnything(t *testing.T) {
	f := hostServiceServer(t)
	for _, args := range [][]string{
		{"host", "service", "start", "../../etc/passwd.service", "--env", "e1"},
		{"host", "service", "start", "nginx", "--env", "e1"},
		{"host", "service", "stop", "-H.service", "--env", "e1", "--yes"},
		{"host", "service", "start", "nginx.service"},
		{"host", "service", "edit", "nginx.service", "--env", "e1", "--yes"},
		{"host", "service", "revert", "nginx.service", "--env", "e1", "--yes"},
	} {
		if code, _, _ := run(t, f, goodKey, args...); code == ExitOK {
			t.Errorf("%v succeeded", args)
		}
	}
	for _, r := range f.seen() {
		if strings.HasPrefix(r.URL.Path, "/api/host-services") {
			t.Fatalf("a refused command reached the server: %s %s", r.Method, r.URL.Path)
		}
	}
}
