// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type versionOrderCases struct {
	Compare []struct {
		A    string `yaml:"a"`
		B    string `yaml:"b"`
		Want int    `yaml:"want"`
	} `yaml:"compare"`
	Parse []struct {
		Input string `yaml:"input"`
		Valid bool   `yaml:"valid"`
	} `yaml:"parse"`
	Prerelease []struct {
		Input string `yaml:"input"`
		Want  bool   `yaml:"want"`
	} `yaml:"prerelease"`
}

// TestVersionOrderCases runs every row of the shared version-order contract.
func TestVersionOrderCases(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "contracts", "versionorder", "cases.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var c versionOrderCases
	if err := yaml.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	if len(c.Compare) == 0 || len(c.Parse) == 0 || len(c.Prerelease) == 0 {
		t.Fatal("cases.yaml read as empty")
	}
	for _, row := range c.Compare {
		if got := compareVersions(row.A, row.B); got != row.Want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", row.A, row.B, got, row.Want)
		}
	}
	for _, row := range c.Parse {
		if _, valid := parseVersion(row.Input); valid != row.Valid {
			t.Errorf("parseVersion(%q) valid = %v, want %v", row.Input, valid, row.Valid)
		}
	}
	for _, row := range c.Prerelease {
		if got := isPrerelease(row.Input); got != row.Want {
			t.Errorf("isPrerelease(%q) = %v, want %v", row.Input, got, row.Want)
		}
	}
}

func channelServer(t *testing.T) *fakeServer {
	t.Helper()
	settings := map[string]any{
		"mcharborPrereleases": true, "agentPrereleases": false, "pluginPrereleases": false,
		"mcharborAutoPrereleases": false, "agentAutoPrereleases": false,
	}
	return groupServer(t, map[string]func(http.ResponseWriter, string){
		"GET /api/updates/channels": func(w http.ResponseWriter, _ string) { ok(w, settings) },
		"PUT /api/updates/channels": func(w http.ResponseWriter, _ string) { ok(w, settings) },
		"PUT /api/appstore/plugins/addon-kvm/channel": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{"slug": "addon-kvm", "prereleases": true})
		},
		"GET /api/appstore/plugins/addon-kvm/return/check": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{"ok": false, "outcome": "unsigned", "unsigned": true, "installed_version": "2026.11.02-rc.1", "target_version": "2026.11.01"})
		},
		"GET /api/appstore/plugins/addon-lxc/return/check": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{"ok": false, "outcome": "blocked", "blocking_migration": "014_snapshots.sql"})
		},
		"POST /api/appstore/plugins/addon-kvm/return": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{"slug": "addon-kvm", "from": "2026.11.02-rc.1", "to": "2026.11.01"})
		},
		"POST /api/updates/agents/env1/return": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{"envId": "env1", "from": "2026.11.02-rc.1", "to": "2026.11.01", "pinned": true, "started": true})
		},
		"GET /api/updates/agents": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{
				"agents": []any{
					map[string]any{"envId": "env1", "name": "homelab01", "version": "2026.11.02-rc.1", "pinned": true, "returnReason": "digest mismatch for mcharbor-agent"},
					map[string]any{"envId": "env3", "name": "nas", "version": "2026.11.01"},
					map[string]any{"envId": "env4", "name": "edge", "version": "2026.11.01"},
				},
				"excludedEnvIds": []any{"env1", "env4"},
			})
		},
		"POST /api/updates/agents/env2/return": func(w http.ResponseWriter, _ string) {
			fail(w, http.StatusConflict, "err.agent.not_connected", "This agent is not connected")
		},
		"DELETE /api/updates/agents/env1/pin": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{"envId": "env1", "pinned": false})
		},
		"GET /api/about": func(w http.ResponseWriter, _ string) { ok(w, map[string]any{"version": "2026.11.02-rc.1"}) },
	})
}

func requestCount(f *fakeServer, method, path string) int {
	n := 0
	for _, r := range f.seen() {
		if r.Method == method && r.URL.Path == path {
			n++
		}
	}
	return n
}

// TestChannelCommands drives every channel command against a fake server and
// proves the body each one sends, the refusals that send nothing, and the
// wording an operator reads for the pinned state.
func TestChannelCommands(t *testing.T) {
	f := channelServer(t)

	t.Run("show prints the switches", func(t *testing.T) {
		code, stdout, stderr := run(t, f, goodKey, "settings", "channels", "show")
		if code != ExitOK {
			t.Fatalf("exit %d: %s", code, stderr)
		}
		if !strings.Contains(stdout, "McHarbor prereleases") || !strings.Contains(stdout, "McHarbor prereleases:   yes") {
			t.Errorf("show lacks the switches:\n%s", stdout)
		}
	})

	t.Run("set sends only the flags given", func(t *testing.T) {
		code, _, stderr := run(t, f, goodKey, "settings", "channels", "set", "--agent-prereleases=true", "--mcharbor-auto-prereleases=false", "--yes")
		if code != ExitOK {
			t.Fatalf("exit %d: %s", code, stderr)
		}
		if got := f.bodies["PUT /api/updates/channels"]; got != `{"agentPrereleases":true,"mcharborAutoPrereleases":false}` {
			t.Errorf("body = %s", got)
		}
	})

	t.Run("set with no flag is a usage error and sends nothing", func(t *testing.T) {
		before := requestCount(f, http.MethodPut, "/api/updates/channels")
		code, _, stderr := run(t, f, goodKey, "settings", "channels", "set", "--yes")
		if code != ExitUsage || !strings.Contains(stderr, "nothing to change") {
			t.Errorf("exit %d, stderr %q", code, stderr)
		}
		if requestCount(f, http.MethodPut, "/api/updates/channels") != before {
			t.Error("a request was sent")
		}
	})

	t.Run("plugin channel on, off and inherit", func(t *testing.T) {
		for arg, want := range map[string]string{"on": `{"prereleases":true}`, "off": `{"prereleases":false}`, "inherit": `{"prereleases":null}`} {
			if code, _, stderr := run(t, f, goodKey, "plugin", "channel", "addon-kvm", arg); code != ExitOK {
				t.Fatalf("%s: exit %d: %s", arg, code, stderr)
			}
			if got := f.bodies["PUT /api/appstore/plugins/addon-kvm/channel"]; got != want {
				t.Errorf("%s: body = %s, want %s", arg, got, want)
			}
		}
		if code, _, _ := run(t, f, goodKey, "plugin", "channel", "addon-kvm", "maybe"); code != ExitUsage {
			t.Errorf("a bad value exits %d, want usage", code)
		}
	})

	t.Run("plugin return check reads, never posts, and says what is true", func(t *testing.T) {
		code, stdout, stderr := run(t, f, goodKey, "plugin", "return", "addon-kvm", "--check")
		if code != ExitOK {
			t.Fatalf("exit %d: %s", code, stderr)
		}
		if !strings.Contains(stdout, "no signature") || !strings.Contains(stdout, "It was not opened") {
			t.Errorf("unsigned wording missing:\n%s", stdout)
		}
		_, stdout, _ = run(t, f, goodKey, "plugin", "return", "addon-lxc", "--check")
		if !strings.Contains(stdout, "014_snapshots.sql") || !strings.Contains(stdout, "Nothing was changed") {
			t.Errorf("blocked wording missing:\n%s", stdout)
		}
		if requestCount(f, http.MethodPost, "/api/appstore/plugins/addon-kvm/return") != 0 {
			t.Error("--check posted a return")
		}
	})

	t.Run("plugin return needs the slug named and consent is explicit", func(t *testing.T) {
		code, _, stderr := run(t, f, goodKey, "plugin", "return", "addon-kvm", "--yes")
		if code != ExitUsage || !strings.Contains(stderr, "--confirm addon-kvm") {
			t.Errorf("--yes alone: exit %d, %q", code, stderr)
		}
		if requestCount(f, http.MethodPost, "/api/appstore/plugins/addon-kvm/return") != 0 {
			t.Fatal("a refused return sent a request")
		}
		code, stdout, stderr := run(t, f, goodKey, "plugin", "return", "addon-kvm", "--confirm", "addon-kvm")
		if code != ExitOK {
			t.Fatalf("exit %d: %s", code, stderr)
		}
		if got := f.bodies["POST /api/appstore/plugins/addon-kvm/return"]; got != `{"allow_unsigned":false}` {
			t.Errorf("body = %s", got)
		}
		if !strings.Contains(stdout, "2026.11.02-rc.1") || !strings.Contains(stdout, "2026.11.01") {
			t.Errorf("result lacks the versions:\n%s", stdout)
		}
		if code, _, stderr = run(t, f, goodKey, "plugin", "return", "addon-kvm", "--confirm", "addon-kvm", "--allow-unsigned"); code != ExitOK {
			t.Fatalf("exit %d: %s", code, stderr)
		}
		if got := f.bodies["POST /api/appstore/plugins/addon-kvm/return"]; got != `{"allow_unsigned":true}` {
			t.Errorf("consent body = %s", got)
		}
	})

	t.Run("agent return pins and says so", func(t *testing.T) {
		code, _, stderr := run(t, f, goodKey, "agent", "return", "env1", "--yes")
		if code != ExitUsage || requestCount(f, http.MethodPost, "/api/updates/agents/env1/return") != 0 {
			t.Fatalf("--yes alone: exit %d, %q", code, stderr)
		}
		code, stdout, stderr := run(t, f, goodKey, "agent", "return", "env1", "--confirm", "env1")
		if code != ExitOK {
			t.Fatalf("exit %d: %s", code, stderr)
		}
		if !strings.Contains(stdout, "Pinned: no updates") || !strings.Contains(stdout, "2026.11.01") {
			t.Errorf("pinned wording missing:\n%s", stdout)
		}
		if !strings.Contains(stdout, "started") || strings.Contains(stdout, "returned to stable") {
			t.Errorf("a started move must not read as finished:\n%s", stdout)
		}
		if !strings.Contains(stdout, "agent pins") {
			t.Errorf("the output does not say where to read the result:\n%s", stdout)
		}
	})

	t.Run("agent pins lists pinned and excluded agents with the return failure", func(t *testing.T) {
		code, stdout, stderr := run(t, f, goodKey, "agent", "pins")
		if code != ExitOK {
			t.Fatalf("exit %d: %s", code, stderr)
		}
		for _, want := range []string{"env1", "homelab01", "digest mismatch for mcharbor-agent", "env4"} {
			if !strings.Contains(stdout, want) {
				t.Errorf("agent pins lacks %q:\n%s", want, stdout)
			}
		}
		if strings.Contains(stdout, "env3") {
			t.Errorf("an agent that is neither pinned nor excluded is listed:\n%s", stdout)
		}
	})

	t.Run("agent return refusal arrives with the server message", func(t *testing.T) {
		code, _, stderr := run(t, f, goodKey, "agent", "return", "env2", "--confirm", "env2")
		if code == ExitOK || !strings.Contains(stderr, "This agent is not connected") {
			t.Errorf("exit %d, stderr %q", code, stderr)
		}
	})

	t.Run("agent unpin removes the exclusion", func(t *testing.T) {
		if code, _, _ := run(t, f, goodKey, "agent", "unpin", "env1", "--yes"); code != ExitUsage {
			t.Errorf("--yes alone exits %d, want usage", code)
		}
		code, stdout, stderr := run(t, f, goodKey, "agent", "unpin", "env1", "--confirm", "env1")
		if code != ExitOK {
			t.Fatalf("exit %d: %s", code, stderr)
		}
		if !strings.Contains(stdout, "Updates reach it again") {
			t.Errorf("unpin wording missing:\n%s", stdout)
		}
		if requestCount(f, http.MethodDelete, "/api/updates/agents/env1/pin") != 1 {
			t.Error("expected exactly one DELETE")
		}
	})

	t.Run("version names a prerelease server", func(t *testing.T) {
		code, stdout, stderr := run(t, f, goodKey, "version")
		if code != ExitOK {
			t.Fatalf("exit %d: %s", code, stderr)
		}
		if !strings.Contains(stdout, "2026.11.02-rc.1") || !strings.Contains(stdout, "Prerelease") {
			t.Errorf("version output lacks the prerelease marker:\n%s", stdout)
		}
	})
}

// TestChannelHelpExamplesNameTheRealCommand keeps the help examples runnable: the
// group lives under `settings`, so an example without it fails when pasted.
func TestChannelHelpExamplesNameTheRealCommand(t *testing.T) {
	f := channelServer(t)
	for _, sub := range []string{"show", "set"} {
		code, stdout, stderr := run(t, f, goodKey, "settings", "channels", sub, "--help")
		if code != ExitOK {
			t.Fatalf("%s --help: exit %d: %s", sub, code, stderr)
		}
		if strings.Contains(stdout, "mcharbor channels") {
			t.Errorf("%s help shows a command that does not exist:\n%s", sub, stdout)
		}
	}
}
