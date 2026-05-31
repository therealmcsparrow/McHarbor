// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package middleware

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
)

var timeNow = time.Now

// Logger logs HTTP requests with structured logging.
func Logger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := timeNow()
			sw := &statusWriter{ResponseWriter: w, status: 200}
			next.ServeHTTP(sw, r)
			logger.Info("http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.status,
				"duration_ms", timeNow().Sub(start).Milliseconds(),
				"ip", clientIP(r),
			)
		})
	}
}

// Recovery catches panics and returns 500.
func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					logger.Error("panic recovered",
						"error", err,
						"stack", string(debug.Stack()),
						"method", r.Method,
						"path", r.URL.Path,
					)
					lang := i18n.FromContext(r.Context())
					msg := i18n.T(lang, i18n.ErrPanicRecovery)
					body, _ := json.Marshal(map[string]any{"success": false, "error": msg, "code": string(i18n.ErrPanicRecovery)}) // safe: simple map literal
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(body)
				}
			}()
			next.ServeHTTP(sw(w), r)
		})
	}
}

// RateLimit is a simple token-bucket rate limiter per IP.
// For auth endpoints, limit to 10 requests per minute.
func RateLimit(requestsPerMinute int) func(http.Handler) http.Handler {
	// Simple in-memory rate limiter
	type bucket struct {
		tokens    int
		lastReset time.Time
	}
	buckets := make(map[string]*bucket)
	var mu sync.Mutex

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := remoteIP(r)
			now := timeNow()

			mu.Lock()
			b, ok := buckets[ip]
			if !ok || now.Sub(b.lastReset) > time.Minute {
				buckets[ip] = &bucket{tokens: requestsPerMinute - 1, lastReset: now}
				mu.Unlock()
				next.ServeHTTP(w, r)
				return
			}

			if b.tokens <= 0 {
				mu.Unlock()
				lang := i18n.FromContext(r.Context())
				msg := i18n.T(lang, i18n.ErrRateLimitExceed)
				body, _ := json.Marshal(map[string]any{"success": false, "error": msg, "code": string(i18n.ErrRateLimitExceed)}) // safe: simple map literal
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write(body)
				return
			}

			b.tokens--
			mu.Unlock()
			next.ServeHTTP(w, r)
		})
	}
}

// statusWriter wraps ResponseWriter to capture the status code.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Unwrap exposes the underlying writer to http.ResponseController.
func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// Flush implements http.Flusher so SSE streaming works through the middleware.
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack implements http.Hijacker so WebSocket upgrades work through the middleware.
func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := w.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not implement http.Hijacker")
}

func sw(w http.ResponseWriter) http.ResponseWriter {
	if _, ok := w.(*statusWriter); ok {
		return w
	}
	return &statusWriter{ResponseWriter: w, status: 200}
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP
		for i, c := range xff {
			if c == ',' {
				return strings.TrimSpace(xff[:i])
			}
		}
		return strings.TrimSpace(xff)
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	return remoteIP(r)
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

// ClientIP extracts the client IP from the request.
func ClientIP(r *http.Request) string {
	return clientIP(r)
}

// ForceHTTPS redirects HTTP requests to HTTPS.
// Checks X-Forwarded-Proto for reverse proxy setups and r.TLS for direct connections.
// Skips /api/health to allow healthchecks over HTTP.
func ForceHTTPS(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip health endpoint for healthchecks
			if strings.HasPrefix(r.URL.Path, "/api/health") {
				next.ServeHTTP(w, r)
				return
			}

			// Check X-Forwarded-Proto (reverse proxy)
			proto := r.Header.Get("X-Forwarded-Proto")
			if proto != "" && proto != "https" {
				target := "https://" + r.Host + r.RequestURI
				logger.Debug("force HTTPS redirect", "from", r.URL.String(), "to", target)
				http.Redirect(w, r, target, http.StatusTemporaryRedirect)
				return
			}

			// Check direct TLS connection (no proxy)
			if proto == "" && r.TLS == nil {
				target := "https://" + r.Host + r.RequestURI
				logger.Debug("force HTTPS redirect", "from", r.URL.String(), "to", target)
				http.Redirect(w, r, target, http.StatusTemporaryRedirect)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders adds baseline browser hardening headers.
func SecurityHeaders() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			headers := w.Header()
			headers.Set("X-Frame-Options", "DENY")
			headers.Set("X-Content-Type-Options", "nosniff")
			headers.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			headers.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			next.ServeHTTP(w, r)
		})
	}
}

// MaxJSONBodySize caps JSON request bodies to prevent oversized payload abuse.
func MaxJSONBodySize(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch:
			default:
				next.ServeHTTP(w, r)
				return
			}

			contentType := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
			if contentType == "" || (!strings.HasPrefix(contentType, "application/json") && !strings.Contains(contentType, "+json")) {
				next.ServeHTTP(w, r)
				return
			}

			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
