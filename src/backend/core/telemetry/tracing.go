// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package telemetry

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	appversion "github.com/therealmcsparrow/mcharbor/core/version"
)

const tracerName = "github.com/therealmcsparrow/mcharbor"

var tracingEnabled atomic.Bool

// TracingEnabled reports whether SetupTracing installed an exporter; when it
// did not, every span helper in this package returns before allocating.
func TracingEnabled() bool { return tracingEnabled.Load() }

// SetupTracing installs an OTLP/HTTP trace exporter when endpoint is set and
// does nothing otherwise, so an unconfigured instance creates no spans at all.
// The exporter reads the standard OTEL_EXPORTER_OTLP_* variables itself.
func SetupTracing(ctx context.Context, endpoint string) (func(context.Context) error, error) {
	noop := func(context.Context) error { return nil }
	if strings.TrimSpace(endpoint) == "" {
		return noop, nil
	}

	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return noop, fmt.Errorf("otlp trace exporter: %w", err)
	}
	res, err := resource.New(ctx,
		resource.WithAttributes(
			attribute.String("service.name", "mcharbor"),
			attribute.String("service.version", appversion.Current()),
		),
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return noop, fmt.Errorf("otel resource: %w", err)
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	relay, err := startRelay(ctx)
	if err != nil {
		if shutdownErr := provider.Shutdown(ctx); shutdownErr != nil {
			return noop, fmt.Errorf("otlp span relay: %w (and provider shutdown: %w)", err, shutdownErr)
		}
		return noop, fmt.Errorf("otlp span relay: %w", err)
	}
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	tracingEnabled.Store(true)
	return func(ctx context.Context) error {
		tracingEnabled.Store(false)
		relayErr := relay.stop(ctx)
		if err := provider.Shutdown(ctx); err != nil {
			return err
		}
		return relayErr
	}, nil
}

func tracer() trace.Tracer { return otel.Tracer(tracerName) }

// StartAgentSpan opens a client span for one hop through envID's agent and
// writes its trace context into headers, which travel inside the WebSocket
// envelope and are replayed onto the request the agent sends its Docker daemon.
func StartAgentSpan(ctx context.Context, envID, method, path string, headers map[string]string) (context.Context, func(err error)) {
	if !tracingEnabled.Load() {
		return ctx, func(error) {}
	}
	ctx, span := tracer().Start(ctx, "agent "+method,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("mcharbor.environment.id", envID),
			attribute.String("http.request.method", method),
			attribute.String("url.path", path),
		))
	if headers != nil {
		// The Docker SDK's own span context arrives here as "Traceparent"; left
		// beside the injected lower-case key, the agent would see two parents.
		for k := range headers {
			switch strings.ToLower(k) {
			case "traceparent", "tracestate", "baggage":
				delete(headers, k)
			}
		}
		otel.GetTextMapPropagator().Inject(ctx, propagation.MapCarrier(headers))
	}
	return ctx, func(err error) {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "agent round trip failed")
		}
		span.End()
	}
}
