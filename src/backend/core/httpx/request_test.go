// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package httpx

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/config"
)

func TestRequestScheme(t *testing.T) {
	tests := []struct {
		name    string
		request *http.Request
		want    string
	}{
		{name: "nil", request: nil, want: "http"},
		{name: "plain", request: httptest.NewRequest(http.MethodGet, "http://example.test/", nil), want: "http"},
		{name: "tls", request: &http.Request{TLS: &tls.ConnectionState{}}, want: "https"},
		{name: "forwarded https", request: requestWithProto("https"), want: "https"},
		{name: "forwarded wss", request: requestWithProto("wss"), want: "https"},
		{name: "forwarded http", request: requestWithProto("http"), want: "http"},
		{name: "forwarded ws", request: requestWithProto("ws"), want: "http"},
		{name: "forwarded list", request: requestWithProto(" https, http"), want: "https"},
		{name: "invalid forwarded", request: requestWithProto("ftp"), want: "http"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RequestScheme(tt.request); got != tt.want {
				t.Fatalf("RequestScheme() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBaseURLs(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.test/path", nil)
	req.Header.Set("X-Forwarded-Proto", "https")

	if got := WebSocketScheme(req); got != "wss" {
		t.Fatalf("WebSocketScheme = %q, want wss", got)
	}
	if got := BaseURL(req); got != "https://example.test" {
		t.Fatalf("BaseURL = %q", got)
	}
	if got := WebSocketBaseURL(req); got != "wss://example.test" {
		t.Fatalf("WebSocketBaseURL = %q", got)
	}

	req.Header.Set("X-Forwarded-Proto", "http")
	if got := WebSocketScheme(req); got != "ws" {
		t.Fatalf("WebSocketScheme = %q, want ws", got)
	}
}

func TestShouldSetSecureCookie(t *testing.T) {
	if !ShouldSetSecureCookie(httptest.NewRequest(http.MethodGet, "http://example.test", nil), &config.Config{ForceSecureCookies: true}) {
		t.Fatal("ForceSecureCookies should force secure cookies")
	}
	if !ShouldSetSecureCookie(httptest.NewRequest(http.MethodGet, "https://example.test", nil), nil) {
		t.Fatal("https request should set secure cookies")
	}
	if ShouldSetSecureCookie(httptest.NewRequest(http.MethodGet, "http://example.test", nil), nil) {
		t.Fatal("http request should not set secure cookies")
	}
}

func TestIsAllowedWebSocketOrigin(t *testing.T) {
	tests := []struct {
		name               string
		target             string
		origin             string
		proto              string
		cfg                *config.Config
		allowMissingOrigin bool
		want               bool
	}{
		{name: "missing allowed", target: "http://example.test", allowMissingOrigin: true, want: true},
		{name: "missing denied", target: "http://example.test", allowMissingOrigin: false, want: false},
		{name: "invalid origin", target: "http://example.test", origin: "://bad", want: false},
		{name: "same origin", target: "http://example.test", origin: "http://example.test", want: true},
		{name: "same origin default port", target: "http://example.test", origin: "http://example.test:80", want: true},
		{name: "same origin forwarded https", target: "http://example.test", origin: "https://example.test", proto: "https", want: true},
		{name: "different origin no config", target: "http://example.test", origin: "http://other.test", want: false},
		{name: "configured origin", target: "http://example.test", origin: "https://allowed.test", cfg: &config.Config{AllowedOrigins: "bad-origin, https://allowed.test"}, want: true},
		{name: "configured origin default port", target: "http://example.test", origin: "https://allowed.test:443", cfg: &config.Config{AllowedOrigins: "https://allowed.test"}, want: true},
		{name: "configured origin mismatch", target: "http://example.test", origin: "https://denied.test", cfg: &config.Config{AllowedOrigins: "https://allowed.test"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.target, nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.proto != "" {
				req.Header.Set("X-Forwarded-Proto", tt.proto)
			}

			got := IsAllowedWebSocketOrigin(req, tt.cfg, tt.allowMissingOrigin)
			if got != tt.want {
				t.Fatalf("IsAllowedWebSocketOrigin = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizeHostPort(t *testing.T) {
	tests := []struct {
		scheme string
		host   string
		want   string
	}{
		{scheme: "http", host: "Example.Test", want: "example.test:80"},
		{scheme: "https", host: "example.test", want: "example.test:443"},
		{scheme: "ws", host: "example.test", want: "example.test:80"},
		{scheme: "wss", host: "example.test", want: "example.test:443"},
		{scheme: "ftp", host: "example.test", want: "example.test"},
		{scheme: "http", host: "", want: ""},
		{scheme: "http", host: "example.test:8080", want: "example.test:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.scheme+"/"+tt.host, func(t *testing.T) {
			if got := normalizeHostPort(tt.scheme, tt.host); got != tt.want {
				t.Fatalf("normalizeHostPort(%q, %q) = %q, want %q", tt.scheme, tt.host, got, tt.want)
			}
		})
	}
}

func requestWithProto(proto string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	req.Header.Set("X-Forwarded-Proto", proto)
	return req
}
