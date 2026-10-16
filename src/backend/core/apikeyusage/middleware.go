// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package apikeyusage

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// Middleware records one usage event per API-key request.
//
// It is a no-op for cookie sessions: resolveKeyID returns "" and the request
// runs without a wrapper at all, so the ordinary UI path pays nothing. Only
// Bearer requests get the observer.
//
// Mount it after the authentication middleware, since it needs the principal
// that middleware installs.
func Middleware(sink Sink, resolveKeyID KeyIDResolver, resolveAddr AddressResolverFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if sink == nil || resolveKeyID == nil {
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			keyID := resolveKeyID(r.Context())
			if keyID == "" {
				next.ServeHTTP(w, r)
				return
			}

			started := time.Now()
			observer := &responseObserver{ResponseWriter: w, status: http.StatusOK}

			// The event is emitted from a defer so a panic still produces a
			// row. Recovery sits outside this middleware and writes the 500
			// after we unwind, so the status is corrected here and the panic
			// re-raised for Recovery to handle exactly as before.
			defer func() {
				if rec := recover(); rec != nil {
					observer.status = http.StatusInternalServerError
					emit(sink, keyID, r, resolveAddr, observer.status, started)
					panic(rec) // panic-ok: re-raising the handler panic so Recovery still writes the 500
				}
				emit(sink, keyID, r, resolveAddr, observer.status, started)
			}()

			next.ServeHTTP(observer, r)
		})
	}
}

// emit assembles the event. Every field is copied out of the request here,
// on the request goroutine; nothing downstream holds the *http.Request.
func emit(sink Sink, keyID string, r *http.Request, resolveAddr AddressResolverFunc, status int, started time.Time) {
	var peerIP, clientIP, source string
	if resolveAddr != nil {
		peerIP, clientIP, source = resolveAddr(r)
	}
	if source == "" {
		source = SourcePeer
	}

	sink.Record(Event{
		APIKeyID:       keyID,
		OccurredAt:     started.UTC(),
		Method:         r.Method,
		RoutePattern:   RoutePatternOf(r),
		StatusCode:     status,
		Duration:       time.Since(started),
		Outcome:        OutcomeAccepted,
		EnvironmentID:  environmentOf(r),
		PeerIP:         peerIP,
		ClientIP:       clientIP,
		ClientIPSource: source,
		UserAgent:      SanitizeUserAgent(r.UserAgent()),
	})
}

// RoutePatternOf returns the chi route template for a request, with the /api
// prefix trimmed so stored routes read like the API surface they describe.
//
// Called after the handler returns, chi has filled in the pattern. Called
// before routing — an early rejection in the Bearer middleware — it has not,
// and the result is RouteUnresolved. The raw URL is never a fallback: it
// carries query values and unbounded cardinality into the table.
func RoutePatternOf(r *http.Request) string {
	if r == nil {
		return RouteUnresolved
	}
	rctx := chi.RouteContext(r.Context())
	if rctx == nil {
		return RouteUnresolved
	}
	return NormalizeRoutePattern(strings.TrimPrefix(rctx.RoutePattern(), "/api"))
}

// environmentOf reads the ?env= scope. Duplicated from response.ParseEnvID by
// one line rather than importing that package, so this one stays free of the
// i18n and response dependencies its position in the chain does not need.
func environmentOf(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	return r.URL.Query().Get("env")
}

// responseObserver captures the status code while preserving the optional
// interfaces a streaming handler needs.
//
// It mirrors core/middleware.statusWriter deliberately: SSE log streams call
// Flush, and the terminal's WebSocket upgrade calls Hijack. A wrapper that
// dropped either would turn a working feature into a silent failure, and
// Unwrap keeps http.ResponseController working for anything that reaches for
// the underlying writer directly.
//
// Like statusWriter it has no Write method, so writes reach the embedded
// ResponseWriter untouched and an implicit 200 stays 200.
type responseObserver struct {
	http.ResponseWriter
	status int
}

func (o *responseObserver) WriteHeader(code int) {
	o.status = code
	o.ResponseWriter.WriteHeader(code)
}

// Unwrap exposes the underlying writer to http.ResponseController.
func (o *responseObserver) Unwrap() http.ResponseWriter {
	return o.ResponseWriter
}

// Flush implements http.Flusher so SSE streaming survives the wrapper.
func (o *responseObserver) Flush() {
	if f, ok := o.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack implements http.Hijacker so WebSocket upgrades survive the wrapper.
func (o *responseObserver) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := o.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not implement http.Hijacker")
}
