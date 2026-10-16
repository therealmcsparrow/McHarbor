// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package config

import (
	"log/slog"
	"strings"
)

// PostureFacts is what the boot posture check needs to know about this
// process. Some of it is only settled after config load — the effective TLS
// state reads the database, the certificate issuer reads the disk — so the
// caller assembles it rather than the check reading Config alone.
type PostureFacts struct {
	AuthDisabled           bool
	TLSEnabled             bool
	LocallyIssuedCert      bool
	HSTSMaxAge             int
	AllowedOrigins         string
	PluginRequireSignature bool
}

// PostureWarning is one default that is defensible on a LAN and wrong on a
// public host. EnvVar names the variable that addresses it, so the log line
// tells the operator what to change rather than only what is wrong.
type PostureWarning struct {
	EnvVar  string
	Message string
}

// PostureWarnings lists every posture finding for f, most severe first. It
// changes nothing: every default stays as it is, and this only says so aloud.
func PostureWarnings(f PostureFacts) []PostureWarning {
	var out []PostureWarning
	if f.AuthDisabled {
		out = append(out, PostureWarning{EnvVar: "AUTH_DISABLE",
			Message: "authentication is disabled; anyone who can reach this port controls the Docker host"})
	}
	if f.TLSEnabled && f.LocallyIssuedCert {
		out = append(out, PostureWarning{EnvVar: "TLS_ENABLED",
			Message: "TLS is served with a certificate from McHarbor's local CA, which browsers do not trust; terminate TLS at a reverse proxy with a real certificate and set TLS_ENABLED=false, or upload a certificate"})
	}
	if f.TLSEnabled && f.HSTSMaxAge <= 0 {
		out = append(out, PostureWarning{EnvVar: "HSTS_MAX_AGE",
			Message: "HSTS is off; set HSTS_MAX_AGE=31536000 once the certificate is one browsers trust"})
	}
	if strings.TrimSpace(f.AllowedOrigins) == "" {
		out = append(out, PostureWarning{EnvVar: "ALLOWED_ORIGINS",
			Message: "ALLOWED_ORIGINS is empty, so only localhost origins are allowed; set it to the URL the UI is served from"})
	}
	if !f.PluginRequireSignature {
		out = append(out, PostureWarning{EnvVar: "MCHARBOR_PLUGIN_REQUIRE_SIGNATURE",
			Message: "unsigned plugins can be installed after an operator confirms; set MCHARBOR_PLUGIN_REQUIRE_SIGNATURE=true on an internet-facing host"})
	}
	return out
}

// LogPosture writes each warning once at warn level, with the env var as a
// structured field so a log search for the variable finds the finding.
func LogPosture(logger *slog.Logger, warnings []PostureWarning) {
	if logger == nil {
		return
	}
	for _, w := range warnings {
		logger.Warn("security posture: "+w.Message, "env", w.EnvVar)
	}
}
