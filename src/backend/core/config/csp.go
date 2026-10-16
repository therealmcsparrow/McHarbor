// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package config

import (
	"os"
	"strings"
)

// EnvContentSecurityPolicy names the variable that overrides or disables the CSP.
const EnvContentSecurityPolicy = "CONTENT_SECURITY_POLICY"

// DefaultContentSecurityPolicy is the policy the host UI is verified against. The two
// relaxations — inline styles for Radix/Recharts/xterm/sonner, and https: images for
// remote app-catalog and plugin icons — are documented in .documentation/security/.
const DefaultContentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob: https:; " +
	"font-src 'self'; " +
	"connect-src 'self'; " +
	"frame-ancestors 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self'; " +
	"object-src 'none'"

// resolveContentSecurityPolicy applies the default only when the variable is unset.
// env/v11 substitutes envDefault for an empty value too, which would make the documented
// "set it empty to disable" escape hatch impossible, so the default is applied here.
func (c *Config) resolveContentSecurityPolicy() {
	raw, set := os.LookupEnv(EnvContentSecurityPolicy)
	if !set {
		c.ContentSecurityPolicy = DefaultContentSecurityPolicy
		return
	}
	c.ContentSecurityPolicy = strings.TrimSpace(raw)
}
