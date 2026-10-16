// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// capsFixturePath is the accept/refuse set the host's plugincaps tests read too.
// Both modules sit under src/, so the same relative path reaches it from each.
var capsFixturePath = filepath.Join("..", "..", "..", "contracts", "plugincaps", "cases.yaml")

type capsSharedCases struct {
	Manifest string `yaml:"manifest"`
	Cases    []struct {
		Name         string `yaml:"name"`
		Error        string `yaml:"error"`
		Capabilities string `yaml:"capabilities"`
	} `yaml:"cases"`
}

func capsFindings(t *testing.T, slug, manifestYAML string) []pluginFinding {
	t.Helper()
	var m pluginManifest
	if err := yaml.Unmarshal([]byte(manifestYAML), &m); err != nil {
		t.Fatalf("manifest does not parse: %v", err)
	}
	var out []pluginFinding
	validateCapabilities(slug, m, func(severity, check, detail string) {
		out = append(out, pluginFinding{Slug: slug, Severity: severity, Check: check, Detail: detail})
	})
	return out
}

func indentCaps(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = "    " + l
	}
	return strings.Join(lines, "\n") + "\n"
}

// TestCapabilitiesSharedCases runs the fixture set the host implementation runs, so
// a rule changed on one side and not the other fails one of the two suites.
func TestCapabilitiesSharedCases(t *testing.T) {
	raw, err := os.ReadFile(capsFixturePath)
	if err != nil {
		t.Fatalf("shared fixtures unreadable: %v", err)
	}
	var set capsSharedCases
	if err := yaml.Unmarshal(raw, &set); err != nil {
		t.Fatal(err)
	}
	if len(set.Cases) < 139 {
		t.Fatalf("shared fixtures shrank to %d cases; the parity guard only grows", len(set.Cases))
	}
	for _, c := range set.Cases {
		t.Run(c.Name, func(t *testing.T) {
			got := capsFindings(t, "addon-demo", set.Manifest+indentCaps(c.Capabilities))
			if c.Error == "" {
				if len(got) != 0 {
					t.Fatalf("want a clean manifest, got %s", joinDetails(got))
				}
				return
			}
			if len(got) == 0 {
				t.Fatalf("want a finding containing %q, got none", c.Error)
			}
			if !strings.Contains(joinDetails(got), c.Error) {
				t.Fatalf("want a finding containing %q, got %s", c.Error, joinDetails(got))
			}
			for _, f := range got {
				if f.Severity != "error" || f.Check != "capabilities" {
					t.Fatalf("a refused declaration must be a capabilities error, got %+v", f)
				}
			}
		})
	}
}

// TestCapabilitiesInTreeManifestsPass is the contract over the shipped plugins: every
// plugin.yaml declares only capabilities the host will accept.
func TestCapabilitiesInTreeManifestsPass(t *testing.T) {
	root := filepath.Join("..", "..", "..", "plugins", "Plugins")
	dirs, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		t.Skipf("plugin source tree not present at %s", root)
	}
	if err != nil {
		t.Fatal(err)
	}
	declared := 0
	for _, d := range dirs {
		raw, err := os.ReadFile(filepath.Join(root, d.Name(), "plugin.yaml"))
		if err != nil {
			continue
		}
		var m pluginManifest
		if err := yaml.Unmarshal(raw, &m); err != nil {
			t.Errorf("%s: plugin.yaml does not parse: %v", d.Name(), err)
			continue
		}
		declared += len(m.Contribs.Capabilities)
		if got := capsFindings(t, d.Name(), string(raw)); len(got) != 0 {
			t.Errorf("%s: %s", d.Name(), joinDetails(got))
		}
	}
	if declared == 0 {
		t.Error("no in-tree plugin declares a capability; the test is not reading the manifests it thinks it is")
	}
}

// TestValidatePluginReportsARefusedCapability proves the check is wired into the
// command, not only callable.
func TestValidatePluginReportsARefusedCapability(t *testing.T) {
	repo := pluginFixture(t, "comm-fixture")
	manifest := filepath.Join(repo, "src", "plugins", "Plugins", "comm-fixture", "plugin.yaml")
	body, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, []byte("  capabilities:\n    - kind: runbook-provider\n      domain: \"VM Stuff\"\n")...)
	if err := os.WriteFile(manifest, body, 0o644); err != nil {
		t.Fatal(err)
	}
	errs := errorsFor(t, repo, "comm-fixture")
	if !strings.Contains(joinDetails(errs), "contributes.capabilities[0] (runbook-provider)") {
		t.Fatalf("want the refused capability reported, got %s", joinDetails(errs))
	}
}
