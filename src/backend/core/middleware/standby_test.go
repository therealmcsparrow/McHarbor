// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestStandbyRefusesWritesAndAgents guards the standby contract: reads and the
// cluster controls pass, writes and agent sockets are refused until promotion.
func TestStandbyRefusesWritesAndAgents(t *testing.T) {
	h := Standby()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	code := func(method, path string) int {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		return rec.Code
	}
	if code(http.MethodPost, "/api/containers/x/stop") != http.StatusOK {
		t.Fatal("an active node must accept writes")
	}
	SetStandby(true)
	t.Cleanup(func() { SetStandby(false) })
	cases := []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/api/containers", http.StatusOK},
		{http.MethodPost, "/api/containers/x/stop", http.StatusServiceUnavailable},
		{http.MethodGet, "/api/agent/ws", http.StatusServiceUnavailable},
		{http.MethodPost, "/api/auth/login", http.StatusOK},
		{http.MethodPost, "/api/cluster/failover", http.StatusOK},
		{http.MethodGet, "/api/readyz", http.StatusOK},
	}
	for _, c := range cases {
		if got := code(c.method, c.path); got != c.want {
			t.Fatalf("%s %s = %d, want %d", c.method, c.path, got, c.want)
		}
	}
}
