// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package middleware

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/httpx"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
)

func TestLoggerCapturesStatusAndClientIP(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := Logger(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	req.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.2")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if !strings.Contains(logs.String(), `"status":201`) || !strings.Contains(logs.String(), `"ip":"192.0.2.1"`) {
		t.Fatalf("logger output missing status/ip: %s", logs.String())
	}
}

func TestRecovery(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	t.Run("no panic", func(t *testing.T) {
		rec := httptest.NewRecorder()
		Recovery(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
		}
	})

	t.Run("panic", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/panic", nil)
		req = req.WithContext(i18n.WithLang(req.Context(), i18n.EN))

		Recovery(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			panic("boom")
		})).ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["code"] != string(i18n.ErrPanicRecovery) || body["success"] != false {
			t.Fatalf("panic response = %#v", body)
		}
	})
}

func TestRateLimit(t *testing.T) {
	restoreTime := setTestTime(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	defer restoreTime()

	calls := 0
	handler := RateLimit(1, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.10:1234"
	req = req.WithContext(i18n.WithLang(req.Context(), i18n.EN))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent || calls != 1 {
		t.Fatalf("first request status/calls = %d/%d", rec.Code, calls)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests || calls != 1 {
		t.Fatalf("second request status/calls = %d/%d", rec.Code, calls)
	}

	setCurrentTestTime(time.Date(2026, 1, 1, 0, 2, 0, 0, time.UTC))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent || calls != 2 {
		t.Fatalf("reset request status/calls = %d/%d", rec.Code, calls)
	}
}

func TestRateLimitAllowsExistingBucketWithTokens(t *testing.T) {
	restoreTime := setTestTime(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	defer restoreTime()

	calls := 0
	handler := RateLimit(2, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.11:1234"

	handler.ServeHTTP(httptest.NewRecorder(), req)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent || calls != 2 {
		t.Fatalf("second allowed request status/calls = %d/%d", rec.Code, calls)
	}
}

func TestStatusWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	w := &statusWriter{ResponseWriter: rec, status: http.StatusOK}
	w.WriteHeader(http.StatusAccepted)
	if w.status != http.StatusAccepted {
		t.Fatalf("statusWriter status = %d", w.status)
	}
	if w.Unwrap() != rec {
		t.Fatal("statusWriter did not unwrap to the underlying writer")
	}

	w.Flush()

	if got := sw(w); got != w {
		t.Fatal("sw should return existing statusWriter")
	}
	wrapped := sw(httptest.NewRecorder())
	if _, ok := wrapped.(*statusWriter); !ok {
		t.Fatalf("sw returned %T, want *statusWriter", wrapped)
	}

	conn, rw, err := w.Hijack()
	if conn != nil || rw != nil || err == nil {
		t.Fatalf("Hijack without support = %v %v %v", conn, rw, err)
	}
}

func TestStatusWriterHijackSuccess(t *testing.T) {
	client, server := net.Pipe()
	defer func() {
		if err := client.Close(); err != nil {
			t.Logf("closing client: %v", err)
		}
	}()
	defer func() {
		if err := server.Close(); err != nil {
			t.Logf("closing server: %v", err)
		}
	}()

	rw := bufio.NewReadWriter(bufio.NewReader(server), bufio.NewWriter(server))
	w := &statusWriter{ResponseWriter: hijackRecorder{conn: server, rw: rw}, status: http.StatusOK}

	conn, gotRW, err := w.Hijack()
	if err != nil {
		t.Fatalf("Hijack returned error: %v", err)
	}
	if conn != server || gotRW != rw {
		t.Fatal("Hijack did not return underlying values")
	}
}

func TestClientIP(t *testing.T) {
	proxy, err := httpx.NewClientAddressResolver([]string{"192.0.2.1"})
	if err != nil {
		t.Fatal(err)
	}
	httpx.SetDefaultResolver(proxy)
	t.Cleanup(func() { httpx.SetDefaultResolver(nil) })

	tests := []struct {
		name       string
		remoteAddr string
		xff        string
		xri        string
		want       string
	}{
		{name: "forwarded list", remoteAddr: "192.0.2.1:1234", xff: " 10.0.0.1, 10.0.0.2", want: "10.0.0.2"},
		{name: "forwarded single", remoteAddr: "192.0.2.1:1234", xff: " 10.0.0.3 ", want: "10.0.0.3"},
		{name: "real ip", remoteAddr: "192.0.2.1:1234", xri: " 10.0.0.4 ", want: "10.0.0.4"},
		{name: "untrusted peer forwarding ignored", remoteAddr: "198.51.100.7:1234", xff: "10.0.0.1", xri: "10.0.0.4", want: "198.51.100.7"},
		{name: "remote host port", remoteAddr: "192.0.2.1:1234", want: "192.0.2.1"},
		{name: "remote raw", remoteAddr: "192.0.2.1", want: "192.0.2.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			req.Header.Set("X-Forwarded-For", tt.xff)
			req.Header.Set("X-Real-IP", tt.xri)

			if got := ClientIP(req); got != tt.want {
				t.Fatalf("ClientIP = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestForceHTTPS(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	tests := []struct {
		name       string
		path       string
		proto      string
		tls        bool
		wantStatus int
	}{
		{name: "health bypass", path: "/api/health", wantStatus: http.StatusNoContent},
		{name: "forwarded http redirect", path: "/app?q=1", proto: "http", wantStatus: http.StatusTemporaryRedirect},
		{name: "direct http redirect", path: "/app?q=1", wantStatus: http.StatusTemporaryRedirect},
		{name: "forwarded https passes", path: "/app", proto: "https", wantStatus: http.StatusNoContent},
		{name: "tls passes", path: "/app", tls: true, wantStatus: http.StatusNoContent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://example.test"+tt.path, nil)
			if tt.proto != "" {
				req.Header.Set("X-Forwarded-Proto", tt.proto)
			}
			if tt.tls {
				req.TLS = &tls.ConnectionState{}
			}
			rec := httptest.NewRecorder()

			ForceHTTPS(logger, "")(next).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestSecurityHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	SecurityHeaders(0)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	headers := rec.Header()
	if headers.Get("X-Frame-Options") != "DENY" ||
		headers.Get("X-Content-Type-Options") != "nosniff" ||
		headers.Get("Referrer-Policy") != "strict-origin-when-cross-origin" ||
		headers.Get("Permissions-Policy") == "" {
		t.Fatalf("security headers not set: %#v", headers)
	}
	if hsts := headers.Get("Strict-Transport-Security"); hsts != "" {
		t.Fatalf("HSTS must be opt-in, got %q with max age 0", hsts)
	}

	rec = httptest.NewRecorder()
	SecurityHeaders(31536000)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if hsts := rec.Header().Get("Strict-Transport-Security"); hsts != "max-age=31536000; includeSubDomains" {
		t.Fatalf("HSTS = %q, want max-age=31536000; includeSubDomains", hsts)
	}
}

func TestMaxJSONBodySize(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		contentType string
		wantLimited bool
	}{
		{name: "get skipped", method: http.MethodGet, contentType: "application/json", wantLimited: false},
		{name: "post missing content type skipped", method: http.MethodPost, wantLimited: false},
		{name: "post text skipped", method: http.MethodPost, contentType: "text/plain", wantLimited: false},
		{name: "post json limited", method: http.MethodPost, contentType: "application/json", wantLimited: true},
		{name: "put json suffix limited", method: http.MethodPut, contentType: "application/vnd.api+json", wantLimited: true},
		{name: "patch json limited", method: http.MethodPatch, contentType: " APPLICATION/JSON ", wantLimited: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var readErr error
			handler := MaxJSONBodySize(2)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, readErr = r.Body.Read(make([]byte, 3))
				w.WriteHeader(http.StatusNoContent)
			}))
			req := httptest.NewRequest(tt.method, "/", strings.NewReader("abc"))
			req.Header.Set("Content-Type", tt.contentType)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if tt.wantLimited && readErr == nil {
				t.Fatal("expected limited body read error")
			}
			if !tt.wantLimited && readErr != nil {
				t.Fatalf("unexpected body read error: %v", readErr)
			}
		})
	}
}

func setTestTime(t *testing.T, value time.Time) func() {
	t.Helper()
	original := timeNow
	current := value
	timeNow = func() time.Time { return current }
	t.Cleanup(func() { timeNow = original })
	setCurrentTestTime = func(next time.Time) { current = next }
	return func() { timeNow = original }
}

var setCurrentTestTime = func(time.Time) {}

type hijackRecorder struct {
	conn net.Conn
	rw   *bufio.ReadWriter
}

func (h hijackRecorder) Header() http.Header { return http.Header{} }
func (h hijackRecorder) Write([]byte) (int, error) {
	return 0, nil
}
func (h hijackRecorder) WriteHeader(int) {}
func (h hijackRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return h.conn, h.rw, nil
}

var _ http.Hijacker = hijackRecorder{}
var _ context.Context
