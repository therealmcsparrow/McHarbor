// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCrossSiteGuard(t *testing.T) {
	allowed := []string{"https://console.example.com"}
	h := crossSiteGuard(allowed)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	tests := []struct {
		name    string
		method  string
		path    string
		host    string
		headers map[string]string
		want    int
	}{
		{"sibling host same-site POST refused", "POST", "/api/containers", "app.example.com",
			map[string]string{"Sec-Fetch-Site": "same-site", "Origin": "https://evil.example.com"}, 403},
		{"cross-site POST refused", "DELETE", "/api/containers/x", "app.example.com",
			map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.test"}, 403},
		{"cross-site POST with no Origin refused", "POST", "/api/x", "app.example.com",
			map[string]string{"Sec-Fetch-Site": "cross-site"}, 403},
		{"no Sec-Fetch-Site, foreign Origin refused", "PUT", "/api/x", "app.example.com",
			map[string]string{"Origin": "https://evil.test"}, 403},
		{"null Origin refused", "POST", "/api/x", "app.example.com",
			map[string]string{"Origin": "null"}, 403},
		{"CLI with no headers passes", "POST", "/api/x", "app.example.com", nil, 204},
		{"bearer client with no browser headers passes", "POST", "/api/x", "app.example.com",
			map[string]string{"Authorization": "Bearer abc"}, 204},
		{"same-origin passes", "POST", "/api/x", "app.example.com",
			map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": "https://app.example.com"}, 204},
		{"user-initiated none passes", "POST", "/api/x", "app.example.com",
			map[string]string{"Sec-Fetch-Site": "none"}, 204},
		{"configured origin cross-site passes", "POST", "/api/x", "app.example.com",
			map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://console.example.com"}, 204},
		{"proxy-rewritten Host with same-origin fetch metadata passes", "POST", "/api/x", "backend:8705",
			map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": "https://public.example.com"}, 204},
		{"plain-http matching Origin without fetch metadata passes", "POST", "/api/x", "192.168.1.5:8705",
			map[string]string{"Origin": "http://192.168.1.5:8705"}, 204},
		{"proxy forwarded proto with matching host passes", "POST", "/api/x", "app.example.com",
			map[string]string{"Origin": "https://app.example.com", "X-Forwarded-Proto": "https"}, 204},
		{"safe GET cross-site passes", "GET", "/api/x", "app.example.com",
			map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.test"}, 204},
		{"SAML ACS cross-site POST from IdP passes", "POST", "/api/identity-providers/abc123/acs", "app.example.com",
			map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://idp.example.org"}, 204},
		{"ACS lookalike path refused", "POST", "/api/identity-providers/abc123/acs/extra", "app.example.com",
			map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.test"}, 403},
		{"inbound webhook from a server passes", "POST", "/api/workflows/webhooks/tok", "app.example.com", nil, 204},
		{"agent transfer PUT from an agent passes", "PUT", "/api/container-backups/internal/agent-archives/t1", "app.example.com", nil, 204},
		{"node-to-node POST passes", "POST", "/api/internal/node/agent-rpc/env1", "app.example.com", nil, 204},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.Host = tt.host
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}
