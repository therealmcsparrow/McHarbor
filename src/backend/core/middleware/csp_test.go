// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func serveCSP(t *testing.T, policy string, reportOnly bool) http.Header {
	t.Helper()
	rec := httptest.NewRecorder()
	ContentSecurityPolicy(policy, reportOnly)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	return rec.Header()
}

func TestContentSecurityPolicyEnforcing(t *testing.T) {
	headers := serveCSP(t, "default-src 'self'", false)
	if got := headers.Get(HeaderContentSecurityPolicy); got != "default-src 'self'" {
		t.Fatalf("CSP = %q, want the configured policy", got)
	}
	if got := headers.Get(HeaderContentSecurityPolicyReportOnly); got != "" {
		t.Fatalf("report-only header must be absent when enforcing, got %q", got)
	}
}

func TestContentSecurityPolicyEmptyDisables(t *testing.T) {
	for _, policy := range []string{"", "   "} {
		for _, reportOnly := range []bool{false, true} {
			headers := serveCSP(t, policy, reportOnly)
			if values := headers.Values(HeaderContentSecurityPolicy); len(values) != 0 {
				t.Fatalf("policy %q: CSP header must be absent, got %q", policy, values)
			}
			if values := headers.Values(HeaderContentSecurityPolicyReportOnly); len(values) != 0 {
				t.Fatalf("policy %q: report-only CSP header must be absent, got %q", policy, values)
			}
		}
	}
}

func TestContentSecurityPolicyReportOnly(t *testing.T) {
	headers := serveCSP(t, "default-src 'self'", true)
	if got := headers.Get(HeaderContentSecurityPolicyReportOnly); got != "default-src 'self'" {
		t.Fatalf("report-only CSP = %q, want the configured policy", got)
	}
	if got := headers.Get(HeaderContentSecurityPolicy); got != "" {
		t.Fatalf("enforcing header must be absent in report-only mode, got %q", got)
	}
}

func TestReplaceContentSecurityPolicy(t *testing.T) {
	h := http.Header{}
	ReplaceContentSecurityPolicy(h, "form-action *")
	if h.Get(HeaderContentSecurityPolicy) != "" || h.Get(HeaderContentSecurityPolicyReportOnly) != "" {
		t.Fatalf("a disabled policy must stay disabled, got %#v", h)
	}

	h.Set(HeaderContentSecurityPolicyReportOnly, "default-src 'self'")
	ReplaceContentSecurityPolicy(h, "form-action *")
	if got := h.Get(HeaderContentSecurityPolicyReportOnly); got != "form-action *" {
		t.Fatalf("report-only policy = %q, want the replacement", got)
	}
	if got := h.Get(HeaderContentSecurityPolicy); got != "" {
		t.Fatalf("replacement must keep report-only mode, got enforcing %q", got)
	}
}
