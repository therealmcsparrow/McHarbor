// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package config

import (
	"os"
	"testing"
)

func TestContentSecurityPolicyDefaultsWhenUnset(t *testing.T) {
	t.Setenv(EnvContentSecurityPolicy, "placeholder")
	if err := os.Unsetenv(EnvContentSecurityPolicy); err != nil {
		t.Fatalf("unset: %v", err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ContentSecurityPolicy != DefaultContentSecurityPolicy {
		t.Fatalf("CSP = %q, want the default", cfg.ContentSecurityPolicy)
	}
	if cfg.CSPReportOnly {
		t.Fatal("report-only must default to false")
	}
}

func TestContentSecurityPolicyEnvOverride(t *testing.T) {
	t.Setenv(EnvContentSecurityPolicy, "  default-src 'none'  ")
	t.Setenv("CONTENT_SECURITY_POLICY_REPORT_ONLY", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ContentSecurityPolicy != "default-src 'none'" {
		t.Fatalf("CSP = %q, want the trimmed override", cfg.ContentSecurityPolicy)
	}
	if !cfg.CSPReportOnly {
		t.Fatal("report-only must follow the environment")
	}
}

func TestContentSecurityPolicyExplicitEmptyDisables(t *testing.T) {
	t.Setenv(EnvContentSecurityPolicy, "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ContentSecurityPolicy != "" {
		t.Fatalf("CSP = %q, want empty so the header is disabled", cfg.ContentSecurityPolicy)
	}
}
