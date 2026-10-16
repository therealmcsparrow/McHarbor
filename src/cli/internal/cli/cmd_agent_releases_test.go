// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"net/http"
	"strings"
	"testing"
)

func agentReleaseServer(t *testing.T) *fakeServer {
	t.Helper()
	return groupServer(t, map[string]func(http.ResponseWriter, string){
		"GET /api/updates/agents/releases": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{
				"target": "2026.09.02",
				"releases": []map[string]any{
					{"version": "2026.09.02", "build": "202609281200", "status": "ready", "isTarget": true, "signatureStatus": "verified"},
					{"version": "2026.09.03", "status": "failed", "signatureStatus": "refused", "lastError": "SHA256SUMS signature does not verify"},
				},
				"settings": map[string]any{"sources": []map[string]any{{"kind": "github", "url": "https://github.com/x/y"}}},
			})
		},
		"POST /api/updates/agents/releases/check": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{"latestVersion": "2026.09.03", "latestSource": "https://github.com/x/y"})
		},
		"POST /api/updates/agents/releases/2026.09.03/pull": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{"code": "msg.agent_release.pull_started"})
		},
		"DELETE /api/updates/agents/releases/2026.09.02": func(w http.ResponseWriter, _ string) {
			fail(w, http.StatusConflict, "err.agent_release.is_target", "That release is the current target")
		},
		"PUT /api/updates/agents/releases/target": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{"code": "msg.agent_release.target_set"})
		},
	})
}

func TestAgentReleasesListShowsTheRefusalReason(t *testing.T) {
	f := agentReleaseServer(t)
	code, stdout, stderr := run(t, f, goodKey, "agent", "releases", "list")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	for _, want := range []string{"2026.09.02", "2026.09.03", "refused", "SHA256SUMS signature does not verify"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("list output lacks %q:\n%s", want, stdout)
		}
	}
}

func TestAgentReleasesMutationsReachTheirRoutes(t *testing.T) {
	f := agentReleaseServer(t)
	if code, _, stderr := run(t, f, goodKey, "agent", "releases", "check"); code != ExitOK {
		t.Fatalf("check: exit %d: %s", code, stderr)
	}
	if code, _, stderr := run(t, f, goodKey, "agent", "releases", "pull", "v2026.09.03"); code != ExitOK {
		t.Fatalf("pull: exit %d: %s", code, stderr)
	}
	if code, _, stderr := run(t, f, goodKey, "agent", "releases", "target", "2026.09.02", "--yes"); code != ExitOK {
		t.Fatalf("target: exit %d: %s", code, stderr)
	}
	if body := f.bodies["PUT /api/updates/agents/releases/target"]; body != `{"version":"2026.09.02"}` {
		t.Fatalf("target body = %s", body)
	}
	if code, _, stderr := run(t, f, goodKey, "agent", "releases", "target", "--clear", "--yes"); code != ExitOK {
		t.Fatalf("target --clear: exit %d: %s", code, stderr)
	}
	if body := f.bodies["PUT /api/updates/agents/releases/target"]; body != `{"version":""}` {
		t.Fatalf("clear body = %s", body)
	}
}

func TestAgentReleasesBuildFlagNamesTheBuild(t *testing.T) {
	f := agentReleaseServer(t)
	if code, _, stderr := run(t, f, goodKey, "agent", "releases", "target", "2026.09.02", "--build", "202609281200", "--yes"); code != ExitOK {
		t.Fatalf("target --build: exit %d: %s", code, stderr)
	}
	if body := f.bodies["PUT /api/updates/agents/releases/target"]; body != `{"build":"202609281200","version":"2026.09.02"}` {
		t.Fatalf("target --build body = %s", body)
	}
	if code, _, _ := run(t, f, goodKey, "agent", "releases", "target", "--clear", "--build", "202609281200", "--yes"); code != ExitUsage {
		t.Errorf("target --clear --build: exit %d, want %d", code, ExitUsage)
	}
	run(t, f, goodKey, "agent", "releases", "delete", "2026.09.02", "--build", "202609281200", "--confirm", "2026.09.02")
	deleted := false
	for _, r := range f.seen() {
		if r.Method != http.MethodDelete {
			continue
		}
		deleted = true
		if got := r.URL.Query().Get("build"); got != "202609281200" {
			t.Errorf("delete sent build %q, want 202609281200", got)
		}
	}
	if !deleted {
		t.Error("delete --build sent no DELETE")
	}
}

func TestAgentReleasesRefuseBadInputBeforeSendingAnything(t *testing.T) {
	f := agentReleaseServer(t)
	for _, args := range [][]string{
		{"agent", "releases", "pull", "../../etc"},
		{"agent", "releases", "delete", "latest", "--confirm", "latest"},
		{"agent", "releases", "target"},
		{"agent", "releases", "target", "2026.09.02", "--clear"},
	} {
		if code, _, _ := run(t, f, goodKey, args...); code != ExitUsage {
			t.Errorf("%v: exit %d, want %d", args, code, ExitUsage)
		}
	}
	if code, _, _ := run(t, f, goodKey, "agent", "releases", "delete", "2026.09.02", "--yes"); code == ExitOK {
		t.Error("an irreversible delete ran on --yes alone")
	}
	for _, r := range f.seen() {
		if r.Method != http.MethodGet {
			t.Errorf("a refused command still sent %s %s", r.Method, r.URL.Path)
		}
	}
	code, _, stderr := run(t, f, goodKey, "agent", "releases", "delete", "2026.09.02", "--confirm", "2026.09.02")
	if code == ExitOK || !strings.Contains(stderr, "current target") {
		t.Fatalf("deleting the target: exit %d: %s", code, stderr)
	}
}
