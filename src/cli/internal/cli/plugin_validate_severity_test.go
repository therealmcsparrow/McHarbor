// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestValidateChecksAuditSeverity: plugin dev validate refuses the audit_severity values the
// host's manifest validator refuses, read from the contract the backend test reads too.
func TestValidateChecksAuditSeverity(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "contracts", "pluginroutes", "audit_severity.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var contract struct {
		Allowed []string `yaml:"allowed"`
		Refused []string `yaml:"refused"`
	}
	if err := yaml.Unmarshal(raw, &contract); err != nil {
		t.Fatal(err)
	}
	got := slices.Clone(pluginAuditSeverities)
	want := slices.Clone(contract.Allowed)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("the CLI's audit severities drifted from the contract: CLI %v, contract %v", got, want)
	}
	errorsFor := func(severity string) []string {
		m := pluginManifest{Slug: "addon-fixture", Schema: "mcharbor.plugin/v1", Version: "2026.09.01", Mcharbor: ">=2026.09.01"}
		m.Contribs.Routes = []pluginRouteDecl{{Method: "GET", Path: "/api/fixture/{id}/env", Requires: "fixture.manage", AuditSeverity: severity}}
		var errs []string
		validateManifestShape("addon-fixture", m, func(sev, _, detail string) {
			if sev == "error" {
				errs = append(errs, detail)
			}
		})
		return errs
	}
	for _, s := range append([]string{""}, contract.Allowed...) {
		if errs := errorsFor(s); len(errs) != 0 {
			t.Errorf("audit_severity %q reported: %v", s, errs)
		}
	}
	for _, s := range contract.Refused {
		if errs := errorsFor(s); !strings.Contains(strings.Join(errs, "\n"), "audit_severity") {
			t.Errorf("audit_severity %q not reported, got %v", s, errs)
		}
	}
}
