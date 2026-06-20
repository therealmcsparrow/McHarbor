// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// SYNC: This file must match src/agent/protocol.go.
// Run `make check-protocol` to verify sync.

package agent

// WebSocket message types exchanged between server and agent.
const (
	// Auth handshake
	MsgAuth       = "auth"        // Agent->Server: token + hostname + Docker version
	MsgAuthResult = "auth_result" // Server->Agent: accept/reject with envID

	// Keepalive
	MsgPing = "ping" // Bidirectional
	MsgPong = "pong" // Bidirectional

	// HTTP proxy (Docker API calls)
	MsgHTTPRequest      = "http_request"       // Server->Agent: proxied Docker API call
	MsgHTTPRequestStart = "http_request_start" // Server->Agent: proxied Docker API call with streamed request body
	MsgHTTPRequestChunk = "http_request_chunk" // Server->Agent: request body chunk
	MsgHTTPRequestEnd   = "http_request_end"   // Server->Agent: end of request body stream
	MsgHTTPResponse     = "http_response"      // Agent->Server: full response

	// Streaming responses (logs, stats, exec)
	MsgHTTPResponseStart = "http_response_start" // Agent->Server: streaming header
	MsgHTTPResponseChunk = "http_response_chunk" // Agent->Server: binary data chunk
	MsgHTTPResponseEnd   = "http_response_end"   // Agent->Server: end of stream

	// Cancellation
	MsgHTTPCancel = "http_cancel" // Server->Agent: cancel in-flight request

	// Exec session (terminal over agent)
	MsgExecStart  = "exec_start"  // Server->Agent: start exec attach
	MsgExecInput  = "exec_input"  // Server->Agent: stdin data
	MsgExecOutput = "exec_output" // Agent->Server: stdout data
	MsgExecResize = "exec_resize" // Server->Agent: terminal resize
	MsgExecEnd    = "exec_end"    // Bidirectional: exec session ended

	// Compose stack commands
	MsgComposeRun    = "compose_run"    // Server->Agent: run docker compose in a staged project
	MsgComposeResult = "compose_result" // Agent->Server: compose command result
	MsgComposeCancel = "compose_cancel" // Server->Agent: cancel compose command

	// Direct agent-to-agent transfers
	MsgTransferPrepare  = "transfer_prepare"  // Server->Target agent: prepare upload receiver
	MsgTransferReady    = "transfer_ready"    // Target agent->Server: receiver status
	MsgTransferImage    = "transfer_image"    // Server->Source agent: stream Docker image to target
	MsgTransferArchive  = "transfer_archive"  // Server->Source agent: stream container archive to target
	MsgTransferProbe    = "transfer_probe"    // Server->Source agent: test direct reachability to target
	MsgTransferRestore  = "transfer_restore"  // Server->Agent: pull restore archive from McHarbor and apply locally
	MsgTransferProgress = "transfer_progress" // Source agent->Server: bytes sent directly
	MsgTransferResult   = "transfer_result"   // Agent->Server: direct transfer result
	MsgTransferCancel   = "transfer_cancel"   // Server->Agent: cancel direct transfer

	// Agent-side container backup and restore
	MsgBackupRun     = "backup_run"     // Server->Agent: create archive locally and upload it
	MsgBackupRestore = "backup_restore" // Server->Agent: download archive locally and restore entries

	// Host metrics and prune (agent reports /proc data and runs docker prune locally)
	MsgHostStatsRequest  = "host_stats_request"  // Server->Agent: collect host stats
	MsgHostStatsResponse = "host_stats_response" // Agent->Server: host stats snapshot
	MsgPruneRequest      = "prune_request"       // Server->Agent: run docker prune
	MsgPruneResponse     = "prune_response"      // Agent->Server: prune result

	// Host terminal (interactive shell on the remote host, streams over WS)
	MsgHostTerminalStart  = "host_terminal_start"  // Server->Agent: start a shell session
	MsgHostTerminalInput  = "host_terminal_input"  // Server->Agent: stdin chunk
	MsgHostTerminalOutput = "host_terminal_output" // Agent->Server: stdout chunk
	MsgHostTerminalResize = "host_terminal_resize" // Server->Agent: TTY resize
	MsgHostTerminalEnd    = "host_terminal_end"    // Bidirectional: session ended

	// Host log streaming (bounded journal/file snapshot over WS)
	MsgHostLogRequest = "host_log_request" // Server->Agent: read host logs
	MsgHostLogChunk   = "host_log_chunk"   // Agent->Server: log line chunk
	MsgHostLogEnd     = "host_log_end"     // Agent->Server: log stream complete

	// Host power actions removed for stability; see commit removing host power.
)

// WSMessage is the envelope for all WebSocket messages.
type WSMessage struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"` // Request ID for multiplexing
	// Payload fields (only populated per message type)
	Auth             *AuthPayload                `json:"auth,omitempty"`
	AuthResult       *AuthResultPayload          `json:"authResult,omitempty"`
	HTTPRequest      *WSHTTPRequest              `json:"httpRequest,omitempty"`
	HTTPResponse     *WSHTTPResponse             `json:"httpResponse,omitempty"`
	StreamStart      *WSStreamStart              `json:"streamStart,omitempty"`
	StreamChunk      *WSStreamChunk              `json:"streamChunk,omitempty"`
	ExecStart        *ExecStartPayload           `json:"execStart,omitempty"`
	ExecResize       *ExecResizePayload          `json:"execResize,omitempty"`
	Compose          *ComposePayload             `json:"compose,omitempty"`
	Transfer         *TransferPayload            `json:"transfer,omitempty"`
	Backup           *BackupPayload              `json:"backup,omitempty"`
	HostStats        *HostStatsPayload           `json:"hostStats,omitempty"`
	Prune            *PrunePayload               `json:"prune,omitempty"`
	HostTerminal     *HostTerminalStartPayload   `json:"hostTerminal,omitempty"`
	HostTerminalRes  *HostTerminalResizePayload  `json:"hostTerminalResize,omitempty"`
	HostTerminalEnd  *HostTerminalEndPayload     `json:"hostTerminalEnd,omitempty"`
	HostLogRequest   *HostLogRequestPayload      `json:"hostLogRequest,omitempty"`
	HostLogChunk     *HostLogChunkPayload        `json:"hostLogChunk,omitempty"`
	HostLogEnd       *HostLogEndPayload          `json:"hostLogEnd,omitempty"`
}

// ComposePayload carries an agent-side docker compose command.
type ComposePayload struct {
	ProjectName string            `json:"projectName,omitempty"`
	Files       map[string]string `json:"files,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	Args        []string          `json:"args,omitempty"`
	Success     bool              `json:"success,omitempty"`
	Output      string            `json:"output,omitempty"`
	Error       string            `json:"error,omitempty"`
}

// TransferPayload carries direct agent-to-agent transfer control metadata.
type TransferPayload struct {
	TransferID           string                  `json:"transferId"`
	Kind                 string                  `json:"kind,omitempty"`
	Token                string                  `json:"token,omitempty"`
	URL                  string                  `json:"url,omitempty"`
	ImageRef             string                  `json:"imageRef,omitempty"`
	ContainerID          string                  `json:"containerId,omitempty"`
	SourcePath           string                  `json:"sourcePath,omitempty"`
	TargetPath           string                  `json:"targetPath,omitempty"`
	Stage                string                  `json:"stage,omitempty"`
	Size                 int64                   `json:"size,omitempty"`
	Bytes                int64                   `json:"bytes,omitempty"`
	StatusCode           int                     `json:"statusCode,omitempty"`
	Success              bool                    `json:"success,omitempty"`
	Error                string                  `json:"error,omitempty"`
	Receiver             *TransferReceiverMarker `json:"receiver,omitempty"`
	ResponderAgentMarker string                  `json:"responderAgentMarker,omitempty"`
	Diagnostic           *TransferAuthDiagnostic `json:"diagnostic,omitempty"`
}

// BackupPayload carries agent-side backup and restore metadata.
type BackupPayload struct {
	TransferID          string                     `json:"transferId"`
	ContainerID         string                     `json:"containerId,omitempty"`
	ContainerName       string                     `json:"containerName,omitempty"`
	IncludeConfig       bool                       `json:"includeConfig,omitempty"`
	IncludeLogs         bool                       `json:"includeLogs,omitempty"`
	IncludeFilesystem   bool                       `json:"includeFilesystem,omitempty"`
	IncludeImage        bool                       `json:"includeImage,omitempty"`
	SelectedMounts      []string                   `json:"selectedMounts,omitempty"`
	RestoreItems        []string                   `json:"restoreItems,omitempty"`
	EncryptionKey       string                     `json:"encryptionKey,omitempty"`
	StorageDestinations []BackupStorageDestination `json:"storageDestinations,omitempty"`
	ArchiveURL          string                     `json:"archiveUrl,omitempty"`
	ArchiveToken        string                     `json:"archiveToken,omitempty"`
	ArchiveSize         int64                      `json:"archiveSize,omitempty"`
	Stage               string                     `json:"stage,omitempty"`
	StorageLocationID   string                     `json:"storageLocationId,omitempty"`
	Bytes               int64                      `json:"bytes,omitempty"`
	Size                int64                      `json:"size,omitempty"`
	Success             bool                       `json:"success,omitempty"`
	Error               string                     `json:"error,omitempty"`
	Restored            []string                   `json:"restored,omitempty"`
}

// BackupStorageDestination describes an operation-scoped upload target.
type BackupStorageDestination struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	LocationType string `json:"locationType"`
	UploadURL    string `json:"uploadUrl"`
	Token        string `json:"token"`
	RemotePath   string `json:"remotePath"`
}

// TransferReceiverMarker identifies a prepared receiver without exposing tokens.
type TransferReceiverMarker struct {
	TransferID       string `json:"transferId"`
	Kind             string `json:"kind"`
	ExpiresAt        string `json:"expiresAt"`
	TokenFingerprint string `json:"tokenFingerprint"`
	AgentMarker      string `json:"agentMarker,omitempty"`
}

// TransferAuthDiagnostic carries token-safe transfer receiver auth diagnostics.
type TransferAuthDiagnostic struct {
	ReceiverExists       bool   `json:"receiverExists"`
	ReceiverExpired      bool   `json:"receiverExpired"`
	ReceiverKind         string `json:"receiverKind,omitempty"`
	KindMatched          bool   `json:"kindMatched"`
	BearerPresent        bool   `json:"bearerPresent"`
	TokenMatched         bool   `json:"tokenMatched"`
	RemoteAddr           string `json:"remoteAddr,omitempty"`
	ResponderAgentMarker string `json:"responderAgentMarker,omitempty"`
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

// HostStatsPayload carries a host-stats snapshot from the agent. Fields
// mirror the Linux readHostStats + readRootFSUsage outputs in the server.
type HostStatsPayload struct {
	CPUPercent float64 `json:"cpuPercent"`
	MemUsed    int64   `json:"memUsed"`
	Load1      float64 `json:"load1"`
	Load5      float64 `json:"load5"`
	Load15     float64 `json:"load15"`
	Uptime     int64   `json:"uptime"`
	FSTotal    int64   `json:"fsTotal"`
	FSUsed     int64   `json:"fsUsed"`
	Supported  bool    `json:"supported"`
	Error      string  `json:"error,omitempty"`
}

// PrunePayload carries a prune request (Server->Agent) or result (Agent->Server).
type PrunePayload struct {
	Type           string `json:"type"`
	Volumes        bool   `json:"volumes,omitempty"`
	ItemsDeleted   int64  `json:"itemsDeleted,omitempty"`
	SpaceReclaimed int64  `json:"spaceReclaimed,omitempty"`
	Success        bool   `json:"success,omitempty"`
	Error          string `json:"error,omitempty"`
}

// HostTerminalStartPayload requests the agent to start an interactive shell.
// Cols/Rows default to 80x24 when omitted.
type HostTerminalStartPayload struct {
	Cols uint `json:"cols,omitempty"`
	Rows uint `json:"rows,omitempty"`
}

// HostTerminalResizePayload updates the TTY size for an active shell session.
type HostTerminalResizePayload struct {
	ExecID string `json:"execId"`
	Cols   uint   `json:"cols"`
	Rows   uint   `json:"rows"`
}

// HostTerminalEndPayload notifies the agent that the session is over.
type HostTerminalEndPayload struct {
	ExecID string `json:"execId"`
}

// HostLogRequestPayload describes a host-log read request.
type HostLogRequestPayload struct {
	Source string `json:"source"`
	Tail   int    `json:"tail"`
}

// HostLogChunkPayload carries a single line of host-log output streamed from
// the agent to the server.
type HostLogChunkPayload struct {
	Line string `json:"line"`
}

// HostLogEndPayload signals that the host-log stream is complete and carries
// any non-fatal notices collected during the read.
type HostLogEndPayload struct {
	Notices []string `json:"notices,omitempty"`
}

// AuthPayload is sent by the agent during the handshake.
type AuthPayload struct {
	Token         string `json:"token"`
	Hostname      string `json:"hostname"`
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	AgentVersion  string `json:"agentVersion"`
	DockerVersion string `json:"dockerVersion"`
	TransferURL   string `json:"transferUrl,omitempty"`
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
