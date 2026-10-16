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

func probeServer(t *testing.T) *fakeServer {
	t.Helper()
	report := map[string]any{"os": "linux", "items": []map[string]any{
		{"key": "docker", "label": "Docker Engine", "present": false, "required": true, "remediation": "install docker"},
	}}
	return groupServer(t, map[string]func(http.ResponseWriter, string){
		"POST /api/agents/probe": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{"success": true, "prerequisites": report})
		},
		"POST /api/agents/install-prereqs": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{"success": true, "output": "installed docker", "prerequisites": report})
		},
	})
}

func TestAgentProbeSendsTheSSHRequestAndShowsTheReport(t *testing.T) {
	f := probeServer(t)
	key := filepath.Join(t.TempDir(), "id")
	if err := os.WriteFile(key, []byte("PRIVATE"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run(t, f, goodKey, "agent", "probe", "10.0.0.5", "--user", "root", "--fingerprint", "SHA256:abc", "--key-file", key, "--scope", "vm")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	for _, want := range []string{"Docker Engine", "install docker"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output lacks %q:\n%s", want, stdout)
		}
	}
	body := f.bodies["POST /api/agents/probe"]
	for _, want := range []string{`"sshHost":"10.0.0.5"`, `"sshUser":"root"`, `"hostKeyFingerprint":"SHA256:abc"`, `"sshKey":"PRIVATE"`, `"scope":"vm"`, `"method":"docker"`} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %s: %s", want, body)
		}
	}
	if strings.Contains(body, "installPrereqs") {
		t.Errorf("a probe must not ask for an install: %s", body)
	}
}

func TestAgentInstallPrereqsConfirmsAndNamesWhatToInstall(t *testing.T) {
	f := probeServer(t)
	args := []string{"agent", "install-prereqs", "10.0.0.5", "--user", "root", "--fingerprint", "SHA256:abc", "--secret-id", "sec1", "--prereq", "docker", "--prereq", "compose"}
	if code, _, _ := run(t, f, goodKey, args...); code == ExitOK {
		t.Fatal("install-prereqs ran without --yes")
	}
	for _, r := range f.seen() {
		if r.Method == http.MethodPost {
			t.Fatalf("an unconfirmed install still sent %s %s", r.Method, r.URL.Path)
		}
	}
	code, stdout, stderr := run(t, f, goodKey, append(args, "--yes")...)
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "installed docker") {
		t.Errorf("install log missing:\n%s", stdout)
	}
	body := f.bodies["POST /api/agents/install-prereqs"]
	for _, want := range []string{`"installPrereqs":["docker","compose"]`, `"sshSecretId":"sec1"`} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %s: %s", want, body)
		}
	}
}

func TestAgentProbeRefusesBadInputBeforeSending(t *testing.T) {
	f := probeServer(t)
	base := []string{"agent", "probe", "h", "--user", "u", "--fingerprint", "fp"}
	for _, extra := range [][]string{
		nil,
		{"--secret-id", "a", "--key-file", "b"},
		{"--secret-id", "a", "--method", "rpm"},
		{"--secret-id", "a", "--scope", "gpu"},
		{"--password-stdin", "--root-password-stdin"},
	} {
		if code, _, _ := run(t, f, goodKey, append(append([]string{}, base...), extra...)...); code != ExitUsage {
			t.Errorf("%v: exit %d, want %d", extra, code, ExitUsage)
		}
	}
	if code, _, _ := run(t, f, goodKey, "agent", "install-prereqs", "h", "--user", "u", "--fingerprint", "fp", "--secret-id", "a", "--yes"); code != ExitUsage {
		t.Errorf("install-prereqs with no --prereq: exit %d, want %d", code, ExitUsage)
	}
	for _, r := range f.seen() {
		if r.Method == http.MethodPost {
			t.Errorf("a refused command still sent %s %s", r.Method, r.URL.Path)
		}
	}
}
