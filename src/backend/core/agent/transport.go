// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/xid"

	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
)

// pendingReq tracks an in-flight HTTP request waiting for a response.
type pendingReq struct {
	respCh        chan *WSMessage
	stream        *StreamReader // non-nil for streaming responses
	discardStream bool
}

const agentRequestChunkSize = 32 * 1024
const agentRequestStreamingMinVersion = "1.3.1"

// ExecSession tracks an active exec terminal session over the agent WebSocket.
type ExecSession struct {
	OutputCh chan []byte
	DoneCh   chan struct{}
}

// AgentTransport implements http.RoundTripper by proxying HTTP requests
// over a WebSocket connection to a remote agent.
type AgentTransport struct {
	conn         *AgentConnection
	db           *sql.DB
	pending      map[string]*pendingReq
	execSessions map[string]*ExecSession
	mu           sync.Mutex
	logger       *slog.Logger
	done         chan struct{}
}

// NewAgentTransport creates a new transport for the given agent connection.
func NewAgentTransport(conn *AgentConnection, db *sql.DB, logger *slog.Logger) *AgentTransport {
	return &AgentTransport{
		conn:         conn,
		db:           db,
		pending:      make(map[string]*pendingReq),
		execSessions: make(map[string]*ExecSession),
		logger:       logger,
		done:         make(chan struct{}),
	}
}

// StartExecSession registers an exec session to receive output from the agent.
func (t *AgentTransport) StartExecSession(id string) *ExecSession {
	s := &ExecSession{
		OutputCh: make(chan []byte, 64),
		DoneCh:   make(chan struct{}),
	}
	t.mu.Lock()
	t.execSessions[id] = s
	t.mu.Unlock()
	return s
}

// StopExecSession removes and closes an exec session.
func (t *AgentTransport) StopExecSession(id string) {
	t.mu.Lock()
	s, ok := t.execSessions[id]
	delete(t.execSessions, id)
	t.mu.Unlock()
	if ok {
		select {
		case <-s.DoneCh:
		default:
			close(s.DoneCh)
		}
	}
}

// RoundTrip implements http.RoundTripper. It serializes the HTTP request,
// sends it over WebSocket, and blocks until the agent responds.
func (t *AgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Apply configurable timeout if request context has no deadline
	if _, hasDeadline := req.Context().Deadline(); !hasDeadline {
		agentSettings := coreSettings.ReadAgentSettings(t.db)
		timeout := time.Duration(agentSettings.RequestTimeout) * time.Second
		ctx, cancel := context.WithTimeout(req.Context(), timeout)
		defer cancel()
		req = req.WithContext(ctx)
	}

	reqID := xid.New().String()

	// Serialize request
	wsReq := &WSHTTPRequest{
		Method:  req.Method,
		Path:    req.URL.Path,
		Query:   req.URL.RawQuery,
		Headers: make(map[string]string),
	}

	for k, v := range req.Header {
		if len(v) > 0 {
			wsReq.Headers[k] = v[0]
		}
	}

	// Register pending request
	pending := &pendingReq{
		respCh:        make(chan *WSMessage, 1),
		discardStream: shouldDiscardResponseStream(wsReq),
	}
	t.mu.Lock()
	t.pending[reqID] = pending
	t.mu.Unlock()

	// cleanup removes the pending request from the map.
	// For non-streaming responses this is called via defer.
	// For streaming responses this is deferred to when the stream ends
	// (see ReadLoop MsgHTTPResponseEnd handler), so that chunks can
	// still be dispatched after RoundTrip returns.
	cleanup := func() {
		t.mu.Lock()
		delete(t.pending, reqID)
		t.mu.Unlock()
	}

	// Send request
	t.logger.Debug("agent RoundTrip: sending", "id", reqID, "method", wsReq.Method, "path", wsReq.Path)
	if err := t.sendRequest(req, reqID, wsReq); err != nil {
		cleanup()
		return nil, err
	}

	// Wait for response or context cancellation
	select {
	case <-req.Context().Done():
		cleanup()
		t.logger.Debug("agent RoundTrip: context cancelled", "id", reqID, "path", wsReq.Path)
		// Send cancel message
		cancel := WSMessage{Type: MsgHTTPCancel, ID: reqID}
		t.conn.WriteJSON(cancel)
		return nil, req.Context().Err()
	case resp := <-pending.respCh:
		if resp == nil {
			cleanup()
			return nil, fmt.Errorf("agent connection closed")
		}
		t.logger.Debug("agent RoundTrip: response received", "id", reqID, "path", wsReq.Path, "type", resp.Type)
		httpResp, err := t.buildResponse(resp, pending)
		if err != nil {
			cleanup()
			return nil, err
		}
		// For streaming responses, keep the pending entry alive so
		// ReadLoop can dispatch chunks. Cleanup happens when the
		// stream ends (MsgHTTPResponseEnd). For regular responses,
		// clean up immediately.
		if pending.stream == nil {
			cleanup()
		}
		return httpResp, nil
	case <-t.done:
		cleanup()
		return nil, fmt.Errorf("agent transport closed")
	}
}

func (t *AgentTransport) sendRequest(req *http.Request, reqID string, wsReq *WSHTTPRequest) error {
	if req.Body == nil {
		return t.conn.WriteJSON(WSMessage{
			Type:        MsgHTTPRequest,
			ID:          reqID,
			HTTPRequest: wsReq,
		})
	}

	streamBody := shouldStreamRequestBody(req)
	if streamBody && !agentSupportsRequestStreaming(t.conn.Version) {
		return fmt.Errorf("streaming docker request bodies require mcharbor-agent %s or newer, connected agent is %s", agentRequestStreamingMinVersion, t.conn.Version)
	}

	if !streamBody {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return fmt.Errorf("reading request body: %w", err)
		}
		wsReq.Body = body
		return t.conn.WriteJSON(WSMessage{
			Type:        MsgHTTPRequest,
			ID:          reqID,
			HTTPRequest: wsReq,
		})
	}

	if err := t.conn.WriteJSON(WSMessage{
		Type:        MsgHTTPRequestStart,
		ID:          reqID,
		HTTPRequest: wsReq,
	}); err != nil {
		return fmt.Errorf("starting streamed request to agent: %w", err)
	}

	buf := make([]byte, agentRequestChunkSize)
	for {
		n, readErr := req.Body.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			if err := t.conn.WriteJSON(WSMessage{
				Type:        MsgHTTPRequestChunk,
				ID:          reqID,
				StreamChunk: &WSStreamChunk{Data: chunk},
			}); err != nil {
				return fmt.Errorf("sending streamed request chunk to agent: %w", err)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("reading streamed request body: %w", readErr)
		}
		select {
		case <-req.Context().Done():
			return req.Context().Err()
		default:
		}
	}

	if err := t.conn.WriteJSON(WSMessage{Type: MsgHTTPRequestEnd, ID: reqID}); err != nil {
		return fmt.Errorf("ending streamed request to agent: %w", err)
	}
	return nil
}

func shouldStreamRequestBody(req *http.Request) bool {
	if req.Body == nil {
		return false
	}
	if req.ContentLength < 0 {
		return true
	}
	return req.ContentLength > 8<<20
}

func shouldDiscardResponseStream(req *WSHTTPRequest) bool {
	if req == nil {
		return false
	}
	return strings.HasSuffix(req.Path, "/images/load") && strings.Contains(req.Query, "quiet=1")
}

func agentSupportsRequestStreaming(version string) bool {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return false
	}
	return compareAgentProtocolVersion(parts, strings.Split(agentRequestStreamingMinVersion, ".")) >= 0
}

func compareAgentProtocolVersion(versionParts, minimumParts []string) int {
	for i := 0; i < 3; i++ {
		versionPart := parseAgentProtocolVersionPart(versionParts, i)
		minimumPart := parseAgentProtocolVersionPart(minimumParts, i)
		if versionPart > minimumPart {
			return 1
		}
		if versionPart < minimumPart {
			return -1
		}
	}
	return 0
}

func parseAgentProtocolVersionPart(parts []string, index int) int {
	if index >= len(parts) {
		return 0
	}
	value, err := strconv.Atoi(parts[index])
	if err != nil {
		return 0
	}
	return value
}

// buildResponse constructs an http.Response from the agent's WebSocket message.
func (t *AgentTransport) buildResponse(msg *WSMessage, pending *pendingReq) (*http.Response, error) {
	switch msg.Type {
	case MsgHTTPResponse:
		if msg.HTTPResponse == nil {
			return nil, fmt.Errorf("empty http response from agent")
		}
		resp := &http.Response{
			StatusCode: msg.HTTPResponse.StatusCode,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(msg.HTTPResponse.Body)),
		}
		for k, v := range msg.HTTPResponse.Headers {
			resp.Header.Set(k, v)
		}
		return resp, nil

	case MsgHTTPResponseStart:
		if msg.StreamStart == nil {
			return nil, fmt.Errorf("empty stream start from agent")
		}
		// StreamReader already created by ReadLoop before dispatching
		resp := &http.Response{
			StatusCode: msg.StreamStart.StatusCode,
			Header:     make(http.Header),
			Body:       pending.stream,
		}
		for k, v := range msg.StreamStart.Headers {
			resp.Header.Set(k, v)
		}
		return resp, nil

	default:
		return nil, fmt.Errorf("unexpected response type: %s", msg.Type)
	}
}

// ReadLoop reads WebSocket messages and dispatches them to pending requests.
// This blocks until the connection is closed or an error occurs.
func (t *AgentTransport) ReadLoop() error {
	defer close(t.done)

	for {
		_, data, err := t.conn.Conn.ReadMessage()
		if err != nil {
			return err
		}

		var msg WSMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			t.logger.Warn("agent: invalid message", "error", err)
			continue
		}

		switch msg.Type {
		case MsgPong:
			// Keepalive response, ignore
			continue

		case MsgPing:
			// Agent sent ping, reply with pong
			pong := WSMessage{Type: MsgPong}
			t.conn.WriteJSON(pong)
			continue

		case MsgHTTPResponse, MsgHTTPResponseStart:
			t.mu.Lock()
			p, ok := t.pending[msg.ID]
			// For streaming responses, create the StreamReader NOW (under lock)
			// so it's available when chunks arrive before RoundTrip processes the start.
			if ok && msg.Type == MsgHTTPResponseStart {
				p.stream = NewStreamReader()
			}
			t.mu.Unlock()
			if ok {
				p.respCh <- &msg
			}

		case MsgHTTPResponseChunk:
			t.mu.Lock()
			p, ok := t.pending[msg.ID]
			t.mu.Unlock()
			if ok && p.discardStream {
				continue
			}
			if ok && p.stream != nil && msg.StreamChunk != nil {
				p.stream.Push(msg.StreamChunk.Data)
			}

		case MsgHTTPResponseEnd:
			t.mu.Lock()
			p, ok := t.pending[msg.ID]
			delete(t.pending, msg.ID) // Clean up streaming pending entry
			t.mu.Unlock()
			if ok && p.stream != nil {
				p.stream.End()
			}

		case MsgExecOutput:
			t.mu.Lock()
			s, ok := t.execSessions[msg.ID]
			t.mu.Unlock()
			if ok && msg.StreamChunk != nil {
				select {
				case s.OutputCh <- msg.StreamChunk.Data:
				default:
					// Drop if channel full to avoid blocking ReadLoop
				}
			}

		case MsgExecEnd:
			t.mu.Lock()
			s, ok := t.execSessions[msg.ID]
			delete(t.execSessions, msg.ID)
			t.mu.Unlock()
			if ok {
				select {
				case <-s.DoneCh:
				default:
					close(s.DoneCh)
				}
			}
		}
	}
}

// Close shuts down the transport.
func (t *AgentTransport) Close() {
	select {
	case <-t.done:
	default:
		close(t.done)
	}
}
