// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

// Proxy handles forwarding Docker API requests to the local Docker socket.
type Proxy struct {
	dockerHost string
	httpClient *http.Client
	logger     *slog.Logger
	execMu     sync.RWMutex
	execConns  map[string]net.Conn
}

// NewProxy creates a new Docker API proxy.
func NewProxy(dockerHost string, logger *slog.Logger) *Proxy {
	transport := &http.Transport{}

	if strings.HasPrefix(dockerHost, "unix://") {
		socketPath := strings.TrimPrefix(dockerHost, "unix://")
		transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		}
	}

	return &Proxy{
		dockerHost: dockerHost,
		httpClient: &http.Client{Transport: transport},
		logger:     logger,
		execConns:  make(map[string]net.Conn),
	}
}

// DetectDockerVersion tries to get the Docker API version from the local daemon.
func (p *Proxy) DetectDockerVersion() string {
	req, err := http.NewRequest("GET", "http://docker/version", nil)
	if err != nil {
		return "unknown"
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "unknown"
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	// Simple extraction — the version JSON contains "ApiVersion":"X.XX"
	s := string(body)
	idx := strings.Index(s, `"ApiVersion":"`)
	if idx < 0 {
		return "unknown"
	}
	s = s[idx+len(`"ApiVersion":"`):]
	end := strings.Index(s, `"`)
	if end < 0 {
		return "unknown"
	}
	return s[:end]
}

// HandleRequest processes a proxied Docker API request and sends the response back.
func (p *Proxy) HandleRequest(ctx context.Context, conn *websocket.Conn, id string, wsReq *WSHTTPRequest) {
	// Build the real HTTP request
	urlStr := fmt.Sprintf("http://docker%s", wsReq.Path)
	if wsReq.Query != "" {
		urlStr += "?" + wsReq.Query
	}

	var bodyReader io.Reader
	if len(wsReq.Body) > 0 {
		bodyReader = strings.NewReader(string(wsReq.Body))
	}

	req, err := http.NewRequestWithContext(ctx, wsReq.Method, urlStr, bodyReader)
	if err != nil {
		p.sendErrorResponse(conn, id, http.StatusBadGateway, err)
		return
	}

	for k, v := range wsReq.Headers {
		req.Header.Set(k, v)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		p.sendErrorResponse(conn, id, http.StatusBadGateway, err)
		return
	}
	defer resp.Body.Close()

	// Check if this is a streaming response
	if isStreamingResponse(resp) {
		p.handleStreamingResponse(ctx, conn, id, resp)
		return
	}

	// Regular response — read full body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		p.sendErrorResponse(conn, id, http.StatusBadGateway, err)
		return
	}

	headers := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	msg := WSMessage{
		Type: MsgHTTPResponse,
		ID:   id,
		HTTPResponse: &WSHTTPResponse{
			StatusCode: resp.StatusCode,
			Headers:    headers,
			Body:       body,
		},
	}

	writeMu.Lock()
	conn.WriteJSON(msg)
	writeMu.Unlock()
}

// handleStreamingResponse sends a streaming response back in chunks.
func (p *Proxy) handleStreamingResponse(ctx context.Context, conn *websocket.Conn, id string, resp *http.Response) {
	headers := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	// Send stream start
	startMsg := WSMessage{
		Type: MsgHTTPResponseStart,
		ID:   id,
		StreamStart: &WSStreamStart{
			StatusCode: resp.StatusCode,
			Headers:    headers,
		},
	}
	writeMu.Lock()
	conn.WriteJSON(startMsg)
	writeMu.Unlock()

	// Stream chunks
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			break
		default:
		}

		n, err := resp.Body.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])

			chunkMsg := WSMessage{
				Type: MsgHTTPResponseChunk,
				ID:   id,
				StreamChunk: &WSStreamChunk{
					Data: chunk,
				},
			}
			writeMu.Lock()
			writeErr := conn.WriteJSON(chunkMsg)
			writeMu.Unlock()
			if writeErr != nil {
				return
			}
		}
		if err != nil {
			break
		}
	}

	// Send stream end
	endMsg := WSMessage{Type: MsgHTTPResponseEnd, ID: id}
	writeMu.Lock()
	conn.WriteJSON(endMsg)
	writeMu.Unlock()
}

// sendErrorResponse sends an error response for a failed request.
func (p *Proxy) sendErrorResponse(conn *websocket.Conn, id string, status int, err error) {
	p.logger.Error("proxy error", "id", id, "error", err)

	body := fmt.Sprintf(`{"message":"%s"}`, err.Error())
	msg := WSMessage{
		Type: MsgHTTPResponse,
		ID:   id,
		HTTPResponse: &WSHTTPResponse{
			StatusCode: status,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       []byte(body),
		},
	}
	writeMu.Lock()
	conn.WriteJSON(msg)
	writeMu.Unlock()
}

// isStreamingResponse checks if the response is a streaming/chunked response.
func isStreamingResponse(resp *http.Response) bool {
	ct := resp.Header.Get("Content-Type")

	// Go's http.Transport moves Transfer-Encoding from headers to
	// resp.TransferEncoding field, so check that instead of the header.
	for _, te := range resp.TransferEncoding {
		if strings.EqualFold(te, "chunked") {
			return true
		}
	}

	// Unknown content length usually means streaming
	if resp.ContentLength < 0 {
		return true
	}

	// Multiplexed streams
	if ct == "application/vnd.docker.raw-stream" || ct == "application/vnd.docker.multiplexed-stream" {
		return true
	}
	// Tar archives (container filesystem copy)
	if ct == "application/x-tar" || ct == "application/octet-stream" {
		return true
	}
	return false
}

// HandleExec starts an exec attach session and streams I/O over WebSocket.
func (p *Proxy) HandleExec(ctx context.Context, wsConn *websocket.Conn, sessionID, execID string) {
	rawConn, reader, err := p.rawExecAttach(execID)
	if err != nil {
		p.logger.Error("exec attach failed", "sessionID", sessionID, "error", err)
		endMsg := WSMessage{Type: MsgExecEnd, ID: sessionID}
		writeMu.Lock()
		wsConn.WriteJSON(endMsg)
		writeMu.Unlock()
		return
	}
	defer rawConn.Close()

	// Register connection for input forwarding
	p.execMu.Lock()
	p.execConns[sessionID] = rawConn
	p.execMu.Unlock()
	defer func() {
		p.execMu.Lock()
		delete(p.execConns, sessionID)
		p.execMu.Unlock()
	}()

	// Read Docker stdout and send as exec_output
	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			break
		default:
		}

		n, readErr := reader.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])

			msg := WSMessage{
				Type:        MsgExecOutput,
				ID:          sessionID,
				StreamChunk: &WSStreamChunk{Data: chunk},
			}
			writeMu.Lock()
			writeErr := wsConn.WriteJSON(msg)
			writeMu.Unlock()
			if writeErr != nil {
				return
			}
		}
		if readErr != nil {
			break
		}
	}

	// Send exec_end
	endMsg := WSMessage{Type: MsgExecEnd, ID: sessionID}
	writeMu.Lock()
	wsConn.WriteJSON(endMsg)
	writeMu.Unlock()
}

// WriteExecInput writes stdin data to an active exec session.
func (p *Proxy) WriteExecInput(sessionID string, data []byte) {
	p.execMu.RLock()
	conn, ok := p.execConns[sessionID]
	p.execMu.RUnlock()
	if ok {
		conn.Write(data)
	}
}

// ResizeExec resizes an exec session terminal via the Docker API.
func (p *Proxy) ResizeExec(execID string, cols, rows uint) {
	url := fmt.Sprintf("http://docker/exec/%s/resize?h=%d&w=%d", execID, rows, cols)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		p.logger.Warn("exec resize request failed", "execID", execID, "error", err)
		return
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		p.logger.Warn("exec resize failed", "execID", execID, "error", err)
		return
	}
	defer resp.Body.Close()
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		p.logger.Warn("exec resize response read failed", "execID", execID, "error", err)
	}
}

// CloseExec closes an active exec session.
func (p *Proxy) CloseExec(sessionID string) {
	p.execMu.Lock()
	conn, ok := p.execConns[sessionID]
	delete(p.execConns, sessionID)
	p.execMu.Unlock()
	if ok {
		conn.Close()
	}
}

// rawExecAttach performs a raw HTTP exec attach to the Docker socket.
func (p *Proxy) rawExecAttach(execID string) (net.Conn, *bufio.Reader, error) {
	var conn net.Conn
	var err error

	if strings.HasPrefix(p.dockerHost, "unix://") {
		socketPath := strings.TrimPrefix(p.dockerHost, "unix://")
		conn, err = net.Dial("unix", socketPath)
	} else if strings.HasPrefix(p.dockerHost, "tcp://") {
		addr := strings.TrimPrefix(p.dockerHost, "tcp://")
		conn, err = net.Dial("tcp", addr)
	} else {
		// Try unix socket at the raw path
		conn, err = net.Dial("unix", p.dockerHost)
	}

	if err != nil {
		return nil, nil, fmt.Errorf("dialing Docker: %w", err)
	}

	body := `{"Detach":false,"Tty":true}`
	req := fmt.Sprintf(
		"POST /exec/%s/start HTTP/1.1\r\n"+
			"Host: docker\r\n"+
			"Content-Type: application/json\r\n"+
			"Connection: Upgrade\r\n"+
			"Upgrade: tcp\r\n"+
			"Content-Length: %d\r\n"+
			"\r\n%s",
		execID, len(body), body,
	)

	if _, err = conn.Write([]byte(req)); err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("writing request: %w", err)
	}

	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusSwitchingProtocols {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		conn.Close()
		return nil, nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	return conn, reader, nil
}

// writeMu protects concurrent WebSocket writes from the proxy.
var writeMu sync.Mutex
