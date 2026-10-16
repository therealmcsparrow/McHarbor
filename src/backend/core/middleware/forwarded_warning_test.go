// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serveForwarded(t *testing.T, configured bool, headers []map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	calls := 0
	h := ForwardedHeaderWarning(logger, configured)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	for _, hs := range headers {
		req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
		for k, v := range hs {
			req.Header.Set(k, v)
		}
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
	if calls != len(headers) {
		t.Fatalf("next handler ran %d times, want %d", calls, len(headers))
	}
	return buf.String()
}

func TestForwardedHeaderWarningFiresOnce(t *testing.T) {
	out := serveForwarded(t, false, []map[string]string{
		{"X-Forwarded-For": "203.0.113.7"},
		{"X-Forwarded-For": "203.0.113.8"},
		{"Forwarded": "for=203.0.113.9"},
	})
	if n := strings.Count(out, "level=WARN"); n != 1 {
		t.Fatalf("got %d warnings, want exactly 1:\n%s", n, out)
	}
	if !strings.Contains(out, "env=MCHARBOR_TRUSTED_PROXY_CIDRS") {
		t.Fatalf("warning does not name its env var:\n%s", out)
	}
}

func TestForwardedHeaderWarningSilentWithoutHeaders(t *testing.T) {
	if out := serveForwarded(t, false, []map[string]string{{}, {"X-Request-Id": "1"}}); out != "" {
		t.Fatalf("warned without a forwarding header:\n%s", out)
	}
}

func TestForwardedHeaderWarningSilentWhenProxyTrusted(t *testing.T) {
	if out := serveForwarded(t, true, []map[string]string{{"X-Forwarded-For": "203.0.113.7"}}); out != "" {
		t.Fatalf("warned although a trusted proxy is configured:\n%s", out)
	}
}
