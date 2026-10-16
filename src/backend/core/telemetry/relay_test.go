// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package telemetry

import (
	"context"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	collectorpb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

const (
	relayTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	relayParent  = "00f067aa0ba902b7"
	relayProxy   = "1111111111111111"
	relayDaemon  = "2222222222222222"
)

func agentSpansFixture() []RelayedSpan {
	now := time.Now()
	return []RelayedSpan{
		{TraceID: relayTraceID, SpanID: relayProxy, ParentSpanID: relayParent, Name: "agent.proxy GET", Kind: "server",
			StartUnixNano: now.Add(-20 * time.Millisecond).UnixNano(), EndUnixNano: now.UnixNano()},
		{TraceID: relayTraceID, SpanID: relayDaemon, ParentSpanID: relayProxy, Name: "docker GET", Kind: "client",
			StartUnixNano: now.Add(-15 * time.Millisecond).UnixNano(), EndUnixNano: now.Add(-5 * time.Millisecond).UnixNano(),
			Attributes: map[string]string{"http.response.status_code": "500"}, Error: "daemon broke"},
		{TraceID: "not-hex", SpanID: relayDaemon, Name: "malformed", StartUnixNano: 1, EndUnixNano: 2},
	}
}

func TestToResourceSpansKeepsParentageAndDropsMalformed(t *testing.T) {
	rs := toResourceSpans("env-1", agentSpansFixture())
	if rs == nil {
		t.Fatal("no resource spans")
	}
	spans := rs.GetScopeSpans()[0].GetSpans()
	if len(spans) != 2 {
		t.Fatalf("spans = %d, want 2 (the malformed one dropped)", len(spans))
	}
	if hex.EncodeToString(spans[1].GetParentSpanId()) != relayProxy {
		t.Fatal("daemon span lost its parent")
	}
	if spans[0].GetKind() != tracepb.Span_SPAN_KIND_SERVER || spans[1].GetKind() != tracepb.Span_SPAN_KIND_CLIENT {
		t.Fatal("span kinds not mapped")
	}
	if spans[1].GetStatus().GetCode() != tracepb.Status_STATUS_CODE_ERROR {
		t.Fatal("error status not carried")
	}
}

func TestRelayIsANoOpWithTracingOff(t *testing.T) {
	before := agentSpansRelayedTotal()
	RelayAgentSpans("env-off", agentSpansFixture())
	if agentSpansRelayedTotal() != before {
		t.Fatal("spans were counted with no relay running")
	}
}

func TestAgentSpansReachTheCollector(t *testing.T) {
	var (
		mu       sync.Mutex
		received []*tracepb.ResourceSpans
	)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		var req collectorpb.ExportTraceServiceRequest
		if err := proto.Unmarshal(body, &req); err != nil {
			t.Error(err)
			return
		}
		mu.Lock()
		received = append(received, req.GetResourceSpans()...)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(collector.Close)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector.URL)

	shutdown, err := SetupTracing(context.Background(), collector.URL)
	if err != nil {
		t.Fatal(err)
	}
	RelayAgentSpans("env-relay", agentSpansFixture())

	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		n := len(received)
		mu.Unlock()
		if n > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	var agentSpans int
	for _, rs := range received {
		service := ""
		for _, kv := range rs.GetResource().GetAttributes() {
			if kv.GetKey() == "service.name" {
				service = kv.GetValue().GetStringValue()
			}
		}
		if service == "mcharbor-agent" {
			for _, ss := range rs.GetScopeSpans() {
				agentSpans += len(ss.GetSpans())
			}
		}
	}
	if agentSpans != 2 {
		t.Fatalf("collector received %d agent spans, want 2", agentSpans)
	}
	if TracingEnabled() {
		t.Fatal("shutdown left tracing enabled")
	}
}

func agentSpansRelayedTotal() float64 {
	families, err := Registry.Gather()
	if err != nil {
		return -1
	}
	total := 0.0
	for _, f := range families {
		if f.GetName() == "mcharbor_agent_spans_relayed_total" {
			for _, m := range f.GetMetric() {
				total += m.GetCounter().GetValue()
			}
		}
	}
	return total
}
