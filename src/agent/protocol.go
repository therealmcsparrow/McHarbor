// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// SYNC: This file must match src/backend/core/agent/protocol.go.
// Run `make check-protocol` to verify sync.

package main

// WebSocket message types — must match server protocol.
const (
	MsgAuth              = "auth"
	MsgAuthResult        = "auth_result"
	MsgPing              = "ping"
	MsgPong              = "pong"
	MsgHTTPRequest       = "http_request"
	MsgHTTPResponse      = "http_response"
	MsgHTTPResponseStart = "http_response_start"
	MsgHTTPResponseChunk = "http_response_chunk"
	MsgHTTPResponseEnd   = "http_response_end"
	MsgHTTPCancel        = "http_cancel"

	// Exec session (terminal)
	MsgExecStart  = "exec_start"
	MsgExecInput  = "exec_input"
	MsgExecOutput = "exec_output"
	MsgExecResize = "exec_resize"
	MsgExecEnd    = "exec_end"
)

// WSMessage is the envelope for all WebSocket messages.
type WSMessage struct {
	Type         string              `json:"type"`
	ID           string              `json:"id,omitempty"`
	Auth         *AuthPayload        `json:"auth,omitempty"`
	AuthResult   *AuthResultPayload  `json:"authResult,omitempty"`
	HTTPRequest  *WSHTTPRequest      `json:"httpRequest,omitempty"`
	HTTPResponse *WSHTTPResponse     `json:"httpResponse,omitempty"`
	StreamStart  *WSStreamStart      `json:"streamStart,omitempty"`
	StreamChunk  *WSStreamChunk      `json:"streamChunk,omitempty"`
	ExecStart    *ExecStartPayload   `json:"execStart,omitempty"`
	ExecResize   *ExecResizePayload  `json:"execResize,omitempty"`
}

// ExecStartPayload is sent by the server to start an exec attach.
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
	Body    []byte            `json:"body,omitempty"`
}

// WSHTTPResponse represents a full HTTP response sent back by the agent.
type WSHTTPResponse struct {
	StatusCode int               `json:"statusCode"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       []byte            `json:"body,omitempty"`
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
