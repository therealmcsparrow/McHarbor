// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// requestTrace measures one proxied request that arrived with a sampled W3C
// traceparent. The agent has no exporter: it usually sits behind NAT and cannot
// reach a collector, so the spans travel back to the server, which relays them.
type requestTrace struct {
	traceID      string
	parentSpanID string
	proxySpanID  string
	daemonSpanID string
	method       string
	path         string
	started      time.Time
	daemonStart  time.Time
	daemonEnd    time.Time
	status       int
	err          string
}

// startRequestTrace returns nil unless headers carry a sampled traceparent. When
// it does, the traceparent forwarded to the Docker daemon is rewritten to name the
// agent's daemon span, so a tracing-enabled daemon nests its own spans under it.
func startRequestTrace(req *WSHTTPRequest) *requestTrace {
	if req == nil || req.Headers == nil {
		return nil
	}
	traceID, parentID, ok := parseTraceparent(req.Headers)
	if !ok {
		return nil
	}
	rt := &requestTrace{
		traceID:      traceID,
		parentSpanID: parentID,
		proxySpanID:  newSpanID(),
		daemonSpanID: newSpanID(),
		method:       req.Method,
		path:         req.Path,
		started:      time.Now(),
	}
	for k := range req.Headers {
		if strings.EqualFold(k, "traceparent") {
			delete(req.Headers, k)
		}
	}
	req.Headers["Traceparent"] = "00-" + rt.traceID + "-" + rt.daemonSpanID + "-01"
	return rt
}

// parseTraceparent accepts version-00 traceparents, and later versions by their
// first four fields as the W3C spec requires; the lower-case key wins when both
// spellings are present because the server writes the agent span's context there.
func parseTraceparent(headers map[string]string) (traceID, parentID string, sampled bool) {
	value, ok := headers["traceparent"]
	if !ok {
		for k, v := range headers {
			if strings.EqualFold(k, "traceparent") {
				value, ok = v, true
				break
			}
		}
	}
	if !ok {
		return "", "", false
	}
	parts := strings.Split(strings.TrimSpace(value), "-")
	if len(parts) < 4 || len(parts[0]) != 2 || parts[0] == "ff" ||
		len(parts[1]) != 32 || len(parts[2]) != 16 || len(parts[3]) != 2 {
		return "", "", false
	}
	if parts[0] == "00" && len(parts) != 4 {
		return "", "", false
	}
	for _, p := range parts[:4] {
		if _, err := hex.DecodeString(p); err != nil || p != strings.ToLower(p) {
			return "", "", false
		}
	}
	if strings.Trim(parts[1], "0") == "" || strings.Trim(parts[2], "0") == "" {
		return "", "", false
	}
	flags, err := hex.DecodeString(parts[3])
	if err != nil || len(flags) == 0 || flags[0]&0x01 == 0 {
		return "", "", false
	}
	return parts[1], parts[2], true
}

func newSpanID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "0000000000000001"
	}
	return hex.EncodeToString(b[:])
}

func (rt *requestTrace) markDaemonStart() {
	if rt != nil {
		rt.daemonStart = time.Now()
	}
}

func (rt *requestTrace) markDaemonEnd(status int, err error) {
	if rt == nil {
		return
	}
	rt.daemonEnd = time.Now()
	rt.status = status
	if err != nil {
		rt.err = err.Error()
	}
}

func (rt *requestTrace) fail(err error) {
	if rt != nil && err != nil && rt.err == "" {
		rt.err = err.Error()
	}
}

// spans builds the agent.proxy span and, when the daemon was reached, its child.
func (rt *requestTrace) spans(end time.Time) []TraceSpan {
	attrs := map[string]string{"http.request.method": rt.method, "url.path": rt.path}
	proxy := TraceSpan{
		TraceID: rt.traceID, SpanID: rt.proxySpanID, ParentSpanID: rt.parentSpanID,
		Name: "agent.proxy " + rt.method, Kind: "server",
		StartUnixNano: rt.started.UnixNano(), EndUnixNano: end.UnixNano(),
		Attributes: attrs, Error: rt.err,
	}
	out := []TraceSpan{proxy}
	if !rt.daemonStart.IsZero() {
		daemonEnd := rt.daemonEnd
		if daemonEnd.IsZero() {
			daemonEnd = end
		}
		daemonAttrs := map[string]string{"http.request.method": rt.method, "url.path": rt.path}
		if rt.status > 0 {
			daemonAttrs["http.response.status_code"] = strconv.Itoa(rt.status)
		}
		out = append(out, TraceSpan{
			TraceID: rt.traceID, SpanID: rt.daemonSpanID, ParentSpanID: rt.proxySpanID,
			Name: "docker " + rt.method, Kind: "client",
			StartUnixNano: rt.daemonStart.UnixNano(), EndUnixNano: daemonEnd.UnixNano(),
			Attributes: daemonAttrs, Error: rt.err,
		})
	}
	return out
}

// finish sends the spans to the server. It takes writeMu exactly once and only
// around the write, and runs after the response itself has been written.
func (rt *requestTrace) finish(conn *websocket.Conn, id string, logger *slog.Logger) {
	if rt == nil || conn == nil {
		return
	}
	msg := WSMessage{Type: MsgTraceSpans, ID: id, Spans: rt.spans(time.Now())}
	writeMu.Lock()
	err := conn.WriteJSON(msg)
	writeMu.Unlock()
	if err != nil && logger != nil {
		logger.Debug("trace spans not sent", "id", id, "error", err)
	}
}
