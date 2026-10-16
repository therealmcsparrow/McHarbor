// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package middleware holds the HTTP middleware shared by every route, such as recovery, rate limiting and CSP headers.
package middleware

import (
	"net/http"
	"strings"
)

const (
	// HeaderContentSecurityPolicy is the enforcing CSP header.
	HeaderContentSecurityPolicy = "Content-Security-Policy"
	// HeaderContentSecurityPolicyReportOnly is the observe-only variant, which the
	// browser reports violations against without blocking anything.
	HeaderContentSecurityPolicyReportOnly = "Content-Security-Policy-Report-Only"
)

// ContentSecurityPolicy sets policy on every response, as the report-only header when
// reportOnly is true. An empty policy returns a pass-through, which is the operator's
// escape hatch when a plugin breaks under the policy.
func ContentSecurityPolicy(policy string, reportOnly bool) func(http.Handler) http.Handler {
	policy = strings.TrimSpace(policy)
	header := HeaderContentSecurityPolicy
	if reportOnly {
		header = HeaderContentSecurityPolicyReportOnly
	}
	return func(next http.Handler) http.Handler {
		if policy == "" {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set(header, policy)
			next.ServeHTTP(w, r)
		})
	}
}

// ReplaceContentSecurityPolicy swaps the policy on a response that already carries one,
// keeping its enforcing or report-only mode. A response with no CSP is left alone, so a
// handler can never re-enable a policy the operator disabled.
func ReplaceContentSecurityPolicy(h http.Header, policy string) {
	for _, name := range []string{HeaderContentSecurityPolicy, HeaderContentSecurityPolicyReportOnly} {
		if h.Get(name) != "" {
			h.Set(name, policy)
		}
	}
}
