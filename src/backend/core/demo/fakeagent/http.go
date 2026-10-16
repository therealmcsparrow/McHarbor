// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package fakeagent

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
)

const (
	chunkSize      = 32 << 10
	bufferedLimit  = 4 << 20
	streamingTypes = "application/vnd.docker.raw-stream application/vnd.docker.multiplexed-stream application/x-tar application/octet-stream"
)

type pendingRequest struct {
	cancel context.CancelFunc
	req    *coreagent.WSHTTPRequest
	body   bytes.Buffer
}

func (s *session) beginStreamedRequest(msg *coreagent.WSMessage) {
	if msg.HTTPRequest == nil {
		return
	}
	s.mu.Lock()
	s.requests[msg.ID] = &pendingRequest{cancel: func() {}, req: msg.HTTPRequest}
	s.mu.Unlock()
}

func (s *session) appendRequestChunk(msg *coreagent.WSMessage) {
	if msg.StreamChunk == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.requests[msg.ID]; ok {
		p.body.Write(msg.StreamChunk.Data)
	}
}

func (s *session) finishStreamedRequest(ctx context.Context, id string) {
	s.mu.Lock()
	p, ok := s.requests[id]
	if ok {
		delete(s.requests, id)
	}
	s.mu.Unlock()
	if ok {
		s.startHTTP(ctx, id, p.req, p.body.Bytes())
	}
}

func (s *session) cancelRequest(id string) {
	s.mu.Lock()
	p, ok := s.requests[id]
	if ok {
		delete(s.requests, id)
	}
	s.mu.Unlock()
	if ok {
		p.cancel()
	}
}

// startHTTP serves one proxied Docker request on its own goroutine, so a
// long-lived stream such as /events never blocks the read loop.
func (s *session) startHTTP(ctx context.Context, id string, req *coreagent.WSHTTPRequest, body []byte) {
	if req == nil {
		return
	}
	if body == nil {
		body = req.Body
	}
	ctx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.requests[id] = &pendingRequest{cancel: cancel, req: req}
	s.mu.Unlock()
	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.requests, id)
			s.mu.Unlock()
			cancel()
		}()
		target := req.Path
		if req.Query != "" {
			target += "?" + req.Query
		}
		hr, err := http.NewRequestWithContext(ctx, req.Method, target, io.NopCloser(bytes.NewReader(body)))
		if err != nil {
			s.reply(&coreagent.WSMessage{Type: coreagent.MsgHTTPResponse, ID: id, HTTPResponse: &coreagent.WSHTTPResponse{StatusCode: http.StatusBadGateway, Headers: map[string]string{"Content-Type": "application/json"}, Body: []byte(`{"message":"malformed request"}`)}})
			return
		}
		for k, v := range req.Headers {
			hr.Header.Set(k, v)
		}
		hr.ContentLength = int64(len(body))
		fw := &frameWriter{s: s, id: id, ctx: ctx, header: http.Header{}}
		s.agent.docker.ServeHTTP(fw, hr)
		fw.finish()
	}()
}

// frameWriter renders an http.Handler's response as agent frames: one
// http_response when the body is small and unflushed, a start/chunk/end
// stream otherwise, in the order the server's read loop requires.
type frameWriter struct {
	s         *session
	id        string
	ctx       context.Context //nolint:containedctx // http.ResponseWriter adapter; Write and Flush cannot take a ctx parameter
	header    http.Header
	status    int
	buf       bytes.Buffer
	streaming bool
	failed    bool
}

func (f *frameWriter) Header() http.Header { return f.header }

func (f *frameWriter) WriteHeader(status int) {
	if f.status != 0 {
		return
	}
	f.status = status
	ct := f.header.Get("Content-Type")
	if ct != "" && strings.Contains(streamingTypes, strings.Split(ct, ";")[0]) {
		f.startStream()
	}
}

func (f *frameWriter) headers() map[string]string {
	out := make(map[string]string, len(f.header))
	for k, v := range f.header {
		if len(v) > 0 {
			out[k] = v[0]
		}
	}
	return out
}

func (f *frameWriter) startStream() {
	if f.streaming {
		return
	}
	f.streaming = true
	f.emit(&coreagent.WSMessage{Type: coreagent.MsgHTTPResponseStart, ID: f.id, StreamStart: &coreagent.WSStreamStart{StatusCode: f.status, Headers: f.headers()}})
	if f.buf.Len() > 0 {
		data := append([]byte(nil), f.buf.Bytes()...)
		f.buf.Reset()
		f.chunks(data)
	}
}

func (f *frameWriter) emit(msg *coreagent.WSMessage) {
	if f.failed || f.ctx.Err() != nil {
		f.failed = true
		return
	}
	if err := f.s.send(msg); err != nil {
		f.failed = true
	}
}

func (f *frameWriter) chunks(data []byte) {
	for len(data) > 0 {
		n := min(len(data), chunkSize)
		f.emit(&coreagent.WSMessage{Type: coreagent.MsgHTTPResponseChunk, ID: f.id, StreamChunk: &coreagent.WSStreamChunk{Data: data[:n]}})
		data = data[n:]
	}
}

func (f *frameWriter) Write(p []byte) (int, error) {
	if f.status == 0 {
		f.WriteHeader(http.StatusOK)
	}
	if f.failed || f.ctx.Err() != nil {
		return 0, io.ErrClosedPipe
	}
	if f.streaming {
		f.chunks(append([]byte(nil), p...))
		return len(p), nil
	}
	f.buf.Write(p)
	if f.buf.Len() > bufferedLimit {
		f.startStream()
	}
	return len(p), nil
}

// Flush switches the response to streaming, which is how a handler that emits
// documents over time (events, stats, pull progress) gets them delivered.
func (f *frameWriter) Flush() {
	if f.status == 0 {
		f.WriteHeader(http.StatusOK)
	}
	f.startStream()
}

func (f *frameWriter) finish() {
	if f.ctx.Err() != nil {
		return
	}
	if f.status == 0 {
		f.status = http.StatusOK
	}
	if f.streaming {
		f.emit(&coreagent.WSMessage{Type: coreagent.MsgHTTPResponseEnd, ID: f.id})
		return
	}
	headers := f.headers()
	headers["Content-Length"] = strconv.Itoa(f.buf.Len())
	f.emit(&coreagent.WSMessage{Type: coreagent.MsgHTTPResponse, ID: f.id, HTTPResponse: &coreagent.WSHTTPResponse{StatusCode: f.status, Headers: headers, Body: f.buf.Bytes()}})
}
