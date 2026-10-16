// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package config

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func envVars(ws []PostureWarning) []string {
	out := make([]string, 0, len(ws))
	for _, w := range ws {
		out = append(out, w.EnvVar)
	}
	return out
}

func TestPostureWarningsNameTheirEnvVar(t *testing.T) {
	got := envVars(PostureWarnings(PostureFacts{
		AuthDisabled:      true,
		TLSEnabled:        true,
		LocallyIssuedCert: true,
	}))
	want := []string{"AUTH_DISABLE", "TLS_ENABLED", "HSTS_MAX_AGE", "ALLOWED_ORIGINS", "MCHARBOR_PLUGIN_REQUIRE_SIGNATURE"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("warnings = %v, want %v (AUTH_DISABLE first)", got, want)
	}
}

func TestPostureWarningsSilentForAHardenedHost(t *testing.T) {
	got := PostureWarnings(PostureFacts{
		TLSEnabled:             true,
		HSTSMaxAge:             31536000,
		AllowedOrigins:         "https://mcharbor.example.com",
		PluginRequireSignature: true,
	})
	if len(got) != 0 {
		t.Fatalf("hardened host produced warnings: %v", envVars(got))
	}
}

func TestPostureWarningsTLSChecksNeedTLS(t *testing.T) {
	got := envVars(PostureWarnings(PostureFacts{
		LocallyIssuedCert:      true,
		AllowedOrigins:         "https://mcharbor.example.com",
		PluginRequireSignature: true,
	}))
	if len(got) != 0 {
		t.Fatalf("TLS off behind a proxy should not warn about the certificate or HSTS, got %v", got)
	}
}

func TestLogPostureWritesOneLinePerWarning(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	LogPosture(logger, PostureWarnings(PostureFacts{}))
	out := buf.String()
	if n := strings.Count(out, "level=WARN"); n != 2 {
		t.Fatalf("got %d warn lines, want 2:\n%s", n, out)
	}
	if !strings.Contains(out, "env=ALLOWED_ORIGINS") || !strings.Contains(out, "env=MCHARBOR_PLUGIN_REQUIRE_SIGNATURE") {
		t.Fatalf("env var missing from log:\n%s", out)
	}
}
