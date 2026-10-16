// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package middleware

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/httpx"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/requestid"
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
				"request_id", requestid.From(r.Context()),
			)
		})
	}
}

// writeErrorEnvelope writes the standard failure envelope; encoding or write failures are logged because the response is already committed.
func writeErrorEnvelope(ctx context.Context, w http.ResponseWriter, status int, msg, code string) {
	body, err := json.Marshal(map[string]any{"success": false, "error": msg, "code": code, "requestId": requestid.From(ctx)})
	if err != nil {
		slog.ErrorContext(ctx, "encoding error envelope failed", "error", err)
		body = []byte(`{"success":false}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(body); err != nil {
		slog.WarnContext(ctx, "writing error envelope failed", "error", err)
	}
}

// Recovery catches panics and returns 500.
func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer recoverPanic(logger, w, r)
			next.ServeHTTP(sw(w), r)
		})
	}
}

// recoverPanic must be the deferred call itself, because recover only stops a panic when its caller is the deferred function.
func recoverPanic(logger *slog.Logger, w http.ResponseWriter, r *http.Request) {
	err := recover()
	if err == nil {
		return
	}
	ctx := r.Context()
	logger.ErrorContext(ctx, "panic recovered",
		"error", err,
		"stack", string(debug.Stack()),
		"method", r.Method,
		"path", r.URL.Path,
	)
	msg := i18n.T(i18n.FromContext(ctx), i18n.ErrPanicRecovery)
	writeErrorEnvelope(ctx, w, http.StatusInternalServerError, msg, string(i18n.ErrPanicRecovery))
}

// rateLimitMaxBuckets caps the per-client bucket map. Behind a trusted proxy
// every distinct origin gets a bucket, so without a cap a long-lived process
// accumulates one entry per address it has ever seen.
const rateLimitMaxBuckets = 10000

// rateLimitWindow is the period a bucket's allowance covers.
const rateLimitWindow = time.Minute

type rateBucket struct {
	tokens    int
	lastReset time.Time
}

// rateLimiter holds the per-client token buckets behind RateLimit. Stale
// buckets are swept inline under mu when the map reaches maxBuckets, so no
// background goroutine needs a lifecycle.
type rateLimiter struct {
	limit      int
	maxBuckets int
	mu         sync.Mutex
	buckets    map[string]*rateBucket
}

func newRateLimiter(limit, maxBuckets int) *rateLimiter {
	return &rateLimiter{limit: limit, maxBuckets: maxBuckets, buckets: make(map[string]*rateBucket)}
}

// allow spends one token from key's bucket and reports whether the request may
// proceed.
func (l *rateLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[key]
	if !ok || now.Sub(b.lastReset) > rateLimitWindow {
		if !ok && len(l.buckets) >= l.maxBuckets {
			l.sweep(now)
		}
		l.buckets[key] = &rateBucket{tokens: l.limit - 1, lastReset: now}
		return true
	}
	if b.tokens <= 0 {
		return false
	}
	b.tokens--
	return true
}

// sweep drops every bucket whose window has passed. When every bucket is still
// live it evicts the oldest, so the map never exceeds maxBuckets; the evicted
// client merely gets a fresh allowance, which is the cheaper failure.
func (l *rateLimiter) sweep(now time.Time) {
	for key, b := range l.buckets {
		if now.Sub(b.lastReset) > rateLimitWindow {
			delete(l.buckets, key)
		}
	}
	for len(l.buckets) >= l.maxBuckets {
		oldestKey := ""
		var oldest time.Time
		for key, b := range l.buckets {
			if oldestKey == "" || b.lastReset.Before(oldest) {
				oldestKey, oldest = key, b.lastReset
			}
		}
		delete(l.buckets, oldestKey)
	}
}

// size reports the number of tracked buckets.
func (l *rateLimiter) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

// Limiter is RateLimit's per-key budget for a caller that only knows the key after it has
// looked at the request, such as the node-to-node routes keying on the verified node id.
type Limiter struct{ l *rateLimiter }

// NewLimiter returns a Limiter allowing requestsPerMinute requests per key per window.
func NewLimiter(requestsPerMinute int) *Limiter {
	return &Limiter{l: newRateLimiter(requestsPerMinute, rateLimitMaxBuckets)}
}

// Allow spends one of key's tokens and reports whether the request may proceed.
func (l *Limiter) Allow(key string) bool { return l.l.allow(key, timeNow()) }

// RateLimit throttles requests per client, identified by clientKey. A nil
// clientKey, or one that returns "", falls back to the socket peer, which is
// correct only when nothing sits in front of the server.
func RateLimit(requestsPerMinute int, clientKey func(*http.Request) string) func(http.Handler) http.Handler {
	return rateLimitWith(newRateLimiter(requestsPerMinute, rateLimitMaxBuckets), clientKey)
}

func rateLimitWith(limiter *rateLimiter, clientKey func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.allow(rateLimitKey(r, clientKey), timeNow()) {
				lang := i18n.FromContext(r.Context())
				msg := i18n.T(lang, i18n.ErrRateLimitExceed)
				writeErrorEnvelope(r.Context(), w, http.StatusTooManyRequests, msg, string(i18n.ErrRateLimitExceed))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// rateLimitKey never returns "": an unattributable request is keyed on its
// peer rather than sharing one bucket with every other unattributable request.
func rateLimitKey(r *http.Request, clientKey func(*http.Request) string) string {
	if clientKey != nil {
		if key := clientKey(r); key != "" {
			return key
		}
	}
	return remoteIP(r)
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
	return httpx.ClientIP(r)
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

// ClientIP is a log-line convenience; a security decision (throttling, lockout)
// must use the AppDeps.ResolveClientAddress resolver and its attribution source.
func ClientIP(r *http.Request) string {
	return clientIP(r)
}

// ForceHTTPS redirects HTTP requests to HTTPS.
// Checks X-Forwarded-Proto for reverse proxy setups and r.TLS for direct connections.
// Skips /api/health to allow healthchecks over HTTP.
//
// `httpsHost` is the canonical "host[:port]" the redirect should
// point at. When empty, the middleware falls back to r.Host — fine
// for proxies that rewrite the Host header, but wrong when the
// redirect listener binds a different port (e.g. :8080 → :8705).
// Pass the main HTTPS listener's address to point the redirect at
// the right place.
func ForceHTTPS(logger *slog.Logger, httpsHost string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip health endpoint for healthchecks
			if strings.HasPrefix(r.URL.Path, "/api/health") {
				next.ServeHTTP(w, r)
				return
			}

			targetHost := httpsHost
			if targetHost == "" {
				targetHost = r.Host
			}

			// Check X-Forwarded-Proto (reverse proxy)
			proto := r.Header.Get("X-Forwarded-Proto")
			if proto != "" && proto != "https" {
				target := "https://" + targetHost + r.RequestURI
				logger.Debug("force HTTPS redirect", "from", r.URL.String(), "to", target)
				http.Redirect(w, r, target, http.StatusTemporaryRedirect)
				return
			}

			// Check direct TLS connection (no proxy)
			if proto == "" && r.TLS == nil {
				target := "https://" + targetHost + r.RequestURI
				logger.Debug("force HTTPS redirect", "from", r.URL.String(), "to", target)
				http.Redirect(w, r, target, http.StatusTemporaryRedirect)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders adds baseline browser hardening headers.
//
// `hstsMaxAge` is in seconds (the HSTS spec unit). When set to a
// positive value, the Strict-Transport-Security header is emitted
// with `max-age=<n>; includeSubDomains`. The header is opt-in
// (default off) because enabling HSTS on the bare self-signed cert
// would lock out operators whose only path back to the recovery
// redirect listener is HTTP, e.g. after a cert upload mistake.
func SecurityHeaders(hstsMaxAge int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			headers := w.Header()
			headers.Set("X-Frame-Options", "DENY")
			headers.Set("X-Content-Type-Options", "nosniff")
			headers.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			headers.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			if hstsMaxAge > 0 {
				headers.Set("Strict-Transport-Security", fmt.Sprintf("max-age=%d; includeSubDomains", hstsMaxAge))
			}
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
