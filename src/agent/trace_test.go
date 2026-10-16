// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"errors"
	"testing"
	"time"
)

const (
	testTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	testParent  = "00f067aa0ba902b7"
)

func TestParseTraceparentOnlyAcceptsSampledValidContexts(t *testing.T) {
	cases := map[string]bool{
		"00-" + testTraceID + "-" + testParent + "-01":              true,
		"00-" + testTraceID + "-" + testParent + "-00":              false,
		"00-00000000000000000000000000000000-" + testParent + "-01": false,
		"00-" + testTraceID + "-0000000000000000-01":                false,
		"ff-" + testTraceID + "-" + testParent + "-01":              false,
		"00-" + testTraceID + "-" + testParent + "-01-extra":        false,
		"01-" + testTraceID + "-" + testParent + "-01-extra":        true,
		"00-4BF92F3577B34DA6A3CE929D0E0E4736-" + testParent + "-01": false,
		"garbage": false,
	}
	for value, want := range cases {
		_, _, got := parseTraceparent(map[string]string{"traceparent": value})
		if got != want {
			t.Errorf("%q: sampled=%v, want %v", value, got, want)
		}
	}
}

func TestStartRequestTraceRewritesTheDaemonTraceparent(t *testing.T) {
	req := &WSHTTPRequest{Method: "GET", Path: "/containers/json", Headers: map[string]string{
		"Traceparent": "00-" + testTraceID + "-1111111111111111-01",
		"traceparent": "00-" + testTraceID + "-" + testParent + "-01",
	}}
	rt := startRequestTrace(req)
	if rt == nil {
		t.Fatal("a sampled request produced no trace")
	}
	if rt.parentSpanID != testParent {
		t.Fatalf("parent = %s, want the lower-case key's span %s", rt.parentSpanID, testParent)
	}
	if len(req.Headers) != 1 || req.Headers["Traceparent"] != "00-"+testTraceID+"-"+rt.daemonSpanID+"-01" {
		t.Fatalf("daemon headers = %v, want one traceparent naming the daemon span", req.Headers)
	}

	rt.markDaemonStart()
	rt.markDaemonEnd(500, errors.New("daemon broke"))
	spans := rt.spans(time.Now())
	if len(spans) != 2 {
		t.Fatalf("spans = %d, want 2", len(spans))
	}
	if spans[0].ParentSpanID != testParent || spans[1].ParentSpanID != spans[0].SpanID {
		t.Fatalf("parentage broken: %+v", spans)
	}
	if spans[1].Attributes["http.response.status_code"] != "500" || spans[1].Error == "" {
		t.Fatalf("daemon span lost its outcome: %+v", spans[1])
	}
}

func TestUntracedRequestIsLeftAlone(t *testing.T) {
	req := &WSHTTPRequest{Method: "GET", Path: "/_ping", Headers: map[string]string{"Accept": "*/*"}}
	if rt := startRequestTrace(req); rt != nil {
		t.Fatal("a request without traceparent was traced")
	}
	if len(req.Headers) != 1 {
		t.Fatalf("headers changed: %v", req.Headers)
	}
	var rt *requestTrace
	rt.markDaemonStart()
	rt.markDaemonEnd(200, nil)
	rt.finish(nil, "id", nil)
}
