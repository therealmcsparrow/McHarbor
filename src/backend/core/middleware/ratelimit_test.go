// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/httpx"
)

func resolverKey(t *testing.T, trusted ...string) func(*http.Request) string {
	t.Helper()
	resolver, err := httpx.NewClientAddressResolver(trusted)
	if err != nil {
		t.Fatal(err)
	}
	return func(r *http.Request) string { return resolver.Resolve(r).ClientIP }
}

func limitedHandler(key func(*http.Request) string) http.Handler {
	return RateLimit(1, key)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
}

func requestFrom(peer, xff string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req.RemoteAddr = peer
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	return req
}

func serveStatus(h http.Handler, req *http.Request) int {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestRateLimitSeparatesClientsBehindTrustedProxy(t *testing.T) {
	setTestTime(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	h := limitedHandler(resolverKey(t, "10.0.0.0/8"))

	if got := serveStatus(h, requestFrom("10.0.0.2:5000", "203.0.113.1")); got != http.StatusNoContent {
		t.Fatalf("first client first request = %d", got)
	}
	if got := serveStatus(h, requestFrom("10.0.0.2:5001", "203.0.113.2")); got != http.StatusNoContent {
		t.Fatalf("second client behind the same proxy was throttled by the first client's bucket: %d", got)
	}
	if got := serveStatus(h, requestFrom("10.0.0.2:5002", "203.0.113.1")); got != http.StatusTooManyRequests {
		t.Fatalf("first client second request = %d, want 429", got)
	}
}

func TestRateLimitIgnoresForgedForwardingFromUntrustedPeer(t *testing.T) {
	setTestTime(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	h := limitedHandler(resolverKey(t, "10.0.0.0/8"))

	if got := serveStatus(h, requestFrom("198.51.100.7:5000", "203.0.113.1")); got != http.StatusNoContent {
		t.Fatalf("first request = %d", got)
	}
	if got := serveStatus(h, requestFrom("198.51.100.7:5001", "203.0.113.99")); got != http.StatusTooManyRequests {
		t.Fatalf("rotating a forged X-Forwarded-For bought a fresh bucket: %d", got)
	}
}

func TestRateLimitEmptyKeyFallsBackToPeer(t *testing.T) {
	setTestTime(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	h := limitedHandler(func(*http.Request) string { return "" })

	if got := serveStatus(h, requestFrom("192.0.2.1:1", "")); got != http.StatusNoContent {
		t.Fatalf("first peer = %d", got)
	}
	if got := serveStatus(h, requestFrom("192.0.2.2:1", "")); got != http.StatusNoContent {
		t.Fatalf("an empty key pooled two peers into one shared bucket: %d", got)
	}
	if got := serveStatus(h, requestFrom("192.0.2.1:2", "")); got != http.StatusTooManyRequests {
		t.Fatalf("first peer second request = %d, want 429", got)
	}
}

func TestRateLimiterSweepBoundsBuckets(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	limiter := newRateLimiter(10, 50)

	for i := 0; i < 50; i++ {
		limiter.allow(fmt.Sprintf("198.51.100.%d", i), start)
	}
	later := start.Add(2 * time.Minute)
	limiter.allow("203.0.113.1", later)
	if got := limiter.size(); got != 1 {
		t.Fatalf("stale buckets survived the sweep: size %d, want 1", got)
	}

	for i := 0; i < 500; i++ {
		limiter.allow(fmt.Sprintf("192.0.2.%d/%d", i%250, i), later)
	}
	if got := limiter.size(); got > 50 {
		t.Fatalf("bucket map grew past its cap under live load: size %d, want <= 50", got)
	}
}
