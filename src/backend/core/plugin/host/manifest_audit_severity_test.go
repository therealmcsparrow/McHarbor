// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package host

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/plugin/contract"
)

// auditSeverityContract is src/contracts/pluginroutes/audit_severity.yaml, which the CLI's
// plugin dev validate reads too, so the two validators cannot disagree.
type auditSeverityContract struct {
	Allowed []string `yaml:"allowed"`
	Refused []string `yaml:"refused"`
}

func readAuditSeverityContract(t *testing.T) auditSeverityContract {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "contracts", "pluginroutes", "audit_severity.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var c auditSeverityContract
	if err := yaml.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func manifestWithSeverity(severity string) *contract.Manifest {
	m := &contract.Manifest{Schema: "mcharbor.plugin/v1", Slug: "addon-fixture", Name: "Fixture", Version: "2026.09.01", Author: "McSparrow"}
	m.Contribs.Routes = []contract.RouteDecl{{
		Method: "GET", Path: "/api/fixture/{id}/env", Requires: "fixture.manage",
		AuditAction: "plugin.fixture.env_revealed", AuditEntityType: "fixture", AuditSeverity: severity,
	}}
	return m
}

// TestManifestAuditSeverityIsTheAuditLogsSet: a route's audit_severity is accepted exactly when
// the audit log stores it, and any other spelling is refused before the plugin loads.
func TestManifestAuditSeverityIsTheAuditLogsSet(t *testing.T) {
	c := readAuditSeverityContract(t)
	allowed := slices.Clone(c.Allowed)
	stored := slices.Clone(audit.Severities)
	slices.Sort(allowed)
	slices.Sort(stored)
	if !slices.Equal(allowed, stored) {
		t.Fatalf("the contract's allowed severities %v drifted from audit.Severities %v", allowed, stored)
	}
	for _, s := range append([]string{""}, c.Allowed...) {
		if err := ValidateManifestSchema(manifestWithSeverity(s)); err != nil {
			t.Errorf("audit_severity %q refused: %v", s, err)
		}
	}
	for _, s := range c.Refused {
		err := ValidateManifestSchema(manifestWithSeverity(s))
		if err == nil || !strings.Contains(err.Error(), "audit_severity") {
			t.Errorf("audit_severity %q accepted (err %v); it must be refused", s, err)
		}
	}
}
