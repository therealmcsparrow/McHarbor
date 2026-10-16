// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package telemetry

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/felixge/httpsnoop"
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// unmatchedRoute labels requests no route claimed, so a scanner probing random
// paths lands in one series instead of one per path.
const unmatchedRoute = "unmatched"

// Handler serves the registry in the Prometheus text exposition format.
func Handler() http.Handler {
	return promhttp.HandlerFor(NodeGatherer(Registry), promhttp.HandlerOpts{ErrorHandling: promhttp.ContinueOnError})
}

// HTTPMiddleware counts and times every request by its chi route pattern, never
// by raw path, and opens a server span when tracing is enabled. It must sit on
// the root chi router so the full pattern is known once the handler returns.
func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpInFlight.Inc()
		defer httpInFlight.Dec()

		var span trace.Span
		if tracingEnabled.Load() {
			ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
			ctx, span = tracer().Start(ctx, r.Method, trace.WithSpanKind(trace.SpanKindServer))
			r = r.WithContext(ctx)
		}

		m := httpsnoop.CaptureMetricsFn(w, func(ww http.ResponseWriter) { next.ServeHTTP(ww, r) })

		route := routePattern(r)
		httpRequests.WithLabelValues(route, r.Method, statusClass(m.Code)).Inc()
		if !isStream(r, w) {
			httpDuration.WithLabelValues(route, r.Method).Observe(m.Duration.Seconds())
		}

		if span != nil {
			span.SetName(r.Method + " " + route)
			span.SetAttributes(
				attribute.String("http.route", route),
				attribute.String("http.request.method", r.Method),
				attribute.Int("http.response.status_code", m.Code),
			)
			if m.Code >= http.StatusInternalServerError {
				span.SetStatus(codes.Error, http.StatusText(m.Code))
			}
			span.End()
		}
	})
}

func routePattern(r *http.Request) string {
	rctx := chi.RouteContext(r.Context())
	if rctx == nil {
		return unmatchedRoute
	}
	pattern := rctx.RoutePattern()
	if pattern == "" {
		return unmatchedRoute
	}
	return pattern
}

func statusClass(code int) string {
	if code < 100 || code > 599 {
		return "unknown"
	}
	return strconv.Itoa(code/100) + "xx"
}

func isStream(r *http.Request, w http.ResponseWriter) bool {
	if r.Header.Get("Upgrade") != "" {
		return true
	}
	return strings.HasPrefix(w.Header().Get("Content-Type"), "text/event-stream")
}
