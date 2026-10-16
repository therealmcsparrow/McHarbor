// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package telemetry

import (
	"context"
	"encoding/hex"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// relayQueueSize bounds how many agent span batches wait for upload; a slow or
// absent collector costs spans, never memory or a stalled agent read loop.
const relayQueueSize = 512

var agentSpansRelayed = newCounterVec("agent_spans_relayed_total",
	"Spans measured by agents and relayed to the OTLP collector, by outcome (exported, failed, dropped).", "outcome")

// RelayedSpan is one span an agent measured, mirroring the agent protocol's
// TraceSpan so this package does not depend on core/agent.
type RelayedSpan struct {
	TraceID       string
	SpanID        string
	ParentSpanID  string
	Name          string
	Kind          string
	StartUnixNano int64
	EndUnixNano   int64
	Attributes    map[string]string
	Error         string
}

type spanRelay struct {
	client otlptrace.Client
	queue  chan *tracepb.ResourceSpans
	done   chan struct{}
	wg     sync.WaitGroup
}

var activeRelay atomic.Pointer[spanRelay]

func startRelay(ctx context.Context) (*spanRelay, error) {
	client := otlptracehttp.NewClient()
	if err := client.Start(ctx); err != nil {
		return nil, err
	}
	r := &spanRelay{client: client, queue: make(chan *tracepb.ResourceSpans, relayQueueSize), done: make(chan struct{})}
	r.wg.Add(1)
	go r.run(context.WithoutCancel(ctx))
	activeRelay.Store(r)
	return r, nil
}

func (r *spanRelay) run(parent context.Context) {
	defer r.wg.Done()
	for {
		select {
		case <-r.done:
			return
		case rs := <-r.queue:
			ctx, cancel := context.WithTimeout(parent, 10*time.Second)
			err := r.client.UploadTraces(ctx, []*tracepb.ResourceSpans{rs})
			cancel()
			n := float64(countSpans(rs))
			if err != nil {
				agentSpansRelayed.WithLabelValues("failed").Add(n)
			} else {
				agentSpansRelayed.WithLabelValues("exported").Add(n)
			}
		}
	}
}

func (r *spanRelay) stop(ctx context.Context) error {
	activeRelay.CompareAndSwap(r, nil)
	close(r.done)
	r.wg.Wait()
	return r.client.Stop(ctx)
}

// RelayAgentSpans queues spans an agent measured for export under the service
// name mcharbor-agent. It does nothing when tracing is off and never blocks.
func RelayAgentSpans(envID string, spans []RelayedSpan) {
	r := activeRelay.Load()
	if r == nil || len(spans) == 0 {
		return
	}
	rs := toResourceSpans(envID, spans)
	if rs == nil {
		return
	}
	select {
	case r.queue <- rs:
	default:
		agentSpansRelayed.WithLabelValues("dropped").Add(float64(countSpans(rs)))
	}
}

func countSpans(rs *tracepb.ResourceSpans) int {
	n := 0
	for _, ss := range rs.GetScopeSpans() {
		n += len(ss.GetSpans())
	}
	return n
}

// toResourceSpans converts agent spans to OTLP, dropping any whose identifiers
// are malformed rather than exporting a span the collector would reject.
func toResourceSpans(envID string, spans []RelayedSpan) *tracepb.ResourceSpans {
	out := make([]*tracepb.Span, 0, len(spans))
	for _, s := range spans {
		traceID, err := hex.DecodeString(s.TraceID)
		if err != nil || len(traceID) != 16 {
			continue
		}
		spanID, err := hex.DecodeString(s.SpanID)
		if err != nil || len(spanID) != 8 {
			continue
		}
		var parentID []byte
		if s.ParentSpanID != "" {
			parentID, err = hex.DecodeString(s.ParentSpanID)
			if err != nil || len(parentID) != 8 {
				continue
			}
		}
		if s.StartUnixNano <= 0 || s.EndUnixNano < s.StartUnixNano {
			continue
		}
		kind := tracepb.Span_SPAN_KIND_INTERNAL
		switch s.Kind {
		case "server":
			kind = tracepb.Span_SPAN_KIND_SERVER
		case "client":
			kind = tracepb.Span_SPAN_KIND_CLIENT
		}
		attrs := make([]*commonpb.KeyValue, 0, len(s.Attributes))
		for k, v := range s.Attributes {
			attrs = append(attrs, stringKV(k, v))
		}
		span := &tracepb.Span{
			TraceId:           traceID,
			SpanId:            spanID,
			ParentSpanId:      parentID,
			Name:              s.Name,
			Kind:              kind,
			StartTimeUnixNano: uint64(s.StartUnixNano),
			EndTimeUnixNano:   uint64(s.EndUnixNano),
			Attributes:        attrs,
		}
		if s.Error != "" {
			span.Status = &tracepb.Status{Code: tracepb.Status_STATUS_CODE_ERROR, Message: s.Error}
		}
		out = append(out, span)
	}
	if len(out) == 0 {
		return nil
	}
	return &tracepb.ResourceSpans{
		Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{
			stringKV("service.name", "mcharbor-agent"),
			stringKV("mcharbor.environment.id", envID),
		}},
		ScopeSpans: []*tracepb.ScopeSpans{{
			Scope: &commonpb.InstrumentationScope{Name: tracerName + "/agent"},
			Spans: out,
		}},
	}
}

func stringKV(k, v string) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: v}}}
}
