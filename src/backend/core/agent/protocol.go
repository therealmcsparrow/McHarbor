// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// SYNC: This file must match src/agent/protocol.go.
// Run `make check-protocol` to verify sync.

package agent

// WebSocket message types exchanged between server and agent.
const (
	// Auth handshake
	MsgAuth       = "auth"        // Agent→Server: token + hostname + Docker version
	MsgAuthResult = "auth_result" // Server→Agent: accept/reject with envID

	// Keepalive
	MsgPing = "ping" // Bidirectional
	MsgPong = "pong" // Bidirectional

	// HTTP proxy (Docker API calls)
	MsgHTTPRequest      = "http_request"       // Server→Agent: proxied Docker API call
	MsgHTTPRequestStart = "http_request_start" // Server→Agent: proxied Docker API call with streamed request body
	MsgHTTPRequestChunk = "http_request_chunk" // Server→Agent: request body chunk
	MsgHTTPRequestEnd   = "http_request_end"   // Server→Agent: end of request body stream
	MsgHTTPResponse     = "http_response"      // Agent→Server: full response

	// Streaming responses (logs, stats, exec)
	MsgHTTPResponseStart = "http_response_start" // Agent→Server: streaming header
	MsgHTTPResponseChunk = "http_response_chunk" // Agent→Server: binary data chunk
	MsgHTTPResponseEnd   = "http_response_end"   // Agent→Server: end of stream

	// Cancellation
	MsgHTTPCancel = "http_cancel" // Server→Agent: cancel in-flight request

	// Exec session (terminal over agent)
	MsgExecStart  = "exec_start"  // Server→Agent: start exec attach
	MsgExecInput  = "exec_input"  // Server→Agent: stdin data
	MsgExecOutput = "exec_output" // Agent→Server: stdout data
	MsgExecResize = "exec_resize" // Server→Agent: terminal resize
	MsgExecEnd    = "exec_end"    // Bidirectional: exec session ended
)

// WSMessage is the envelope for all WebSocket messages.
type WSMessage struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"` // Request ID for multiplexing
	// Payload fields (only populated per message type)
	Auth         *AuthPayload       `json:"auth,omitempty"`
	AuthResult   *AuthResultPayload `json:"authResult,omitempty"`
	HTTPRequest  *WSHTTPRequest     `json:"httpRequest,omitempty"`
	HTTPResponse *WSHTTPResponse    `json:"httpResponse,omitempty"`
	StreamStart  *WSStreamStart     `json:"streamStart,omitempty"`
	StreamChunk  *WSStreamChunk     `json:"streamChunk,omitempty"`
	ExecStart    *ExecStartPayload  `json:"execStart,omitempty"`
	ExecResize   *ExecResizePayload `json:"execResize,omitempty"`
}

// ExecStartPayload is sent by the server to start an exec attach on the agent.
type ExecStartPayload struct {
	ExecID string `json:"execId"`
}

// ExecResizePayload is sent by the server to resize the exec terminal.
type ExecResizePayload struct {
	ExecID string `json:"execId"`
	Cols   uint   `json:"cols"`
	Rows   uint   `json:"rows"`
}

// AuthPayload is sent by the agent during the handshake.
type AuthPayload struct {
	Token         string `json:"token"`
	Hostname      string `json:"hostname"`
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	AgentVersion  string `json:"agentVersion"`
	DockerVersion string `json:"dockerVersion"`
}

// AuthResultPayload is sent by the server after validating the auth.
type AuthResultPayload struct {
	Success bool   `json:"success"`
	EnvID   string `json:"envId,omitempty"`
	Error   string `json:"error,omitempty"`
}

// WSHTTPRequest represents a Docker API HTTP request sent over WebSocket.
type WSHTTPRequest struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Query   string            `json:"query,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    []byte            `json:"body,omitempty"` // raw bytes
}

// WSHTTPResponse represents a full HTTP response sent back by the agent.
type WSHTTPResponse struct {
	StatusCode int               `json:"statusCode"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       []byte            `json:"body,omitempty"` // raw bytes
}

// WSStreamStart signals the beginning of a streaming response.
type WSStreamStart struct {
	StatusCode int               `json:"statusCode"`
	Headers    map[string]string `json:"headers,omitempty"`
}

// WSStreamChunk carries a chunk of streaming response data.
type WSStreamChunk struct {
	Data []byte `json:"data"`
}
