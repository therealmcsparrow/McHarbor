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
const agentRequestStreamingMinVersion = "1.3.3"
const TransferKindProbe = "probe"

// ExecSession tracks an active exec terminal session over the agent WebSocket.
type ExecSession struct {
	OutputCh chan []byte
	DoneCh   chan struct{}
}

// RunCompose sends a compose command to the agent and waits for completion.
func (t *AgentTransport) RunCompose(ctx context.Context, payload ComposePayload) (*ComposePayload, error) {
	reqID := xid.New().String()
	ch := make(chan *WSMessage, 1)

	t.mu.Lock()
	t.composeWaiters[reqID] = ch
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		delete(t.composeWaiters, reqID)
		t.mu.Unlock()
	}()

	if err := t.conn.WriteJSON(WSMessage{
		Type:    MsgComposeRun,
		ID:      reqID,
		Compose: &payload,
	}); err != nil {
		return nil, fmt.Errorf("sending compose command to agent: %w", err)
	}

	select {
	case <-ctx.Done():
		if err := t.conn.WriteJSON(WSMessage{Type: MsgComposeCancel, ID: reqID}); err != nil {
			t.logger.Debug("agent compose cancel failed", "id", reqID, "error", err)
		}
		return nil, ctx.Err()
	case msg := <-ch:
		if msg == nil || msg.Compose == nil {
			return nil, fmt.Errorf("empty compose response from agent")
		}
		return msg.Compose, nil
	case <-t.done:
		return nil, fmt.Errorf("agent transport closed")
	}
}

// HostStatsSupported reports whether the connected agent understands the
// host_stats_request message. Agents older than 1.6.0 do not.
func (t *AgentTransport) HostStatsSupported() bool {
	return t.conn.SupportsFeature("1.6.0")
}

// PruneSupported reports whether the connected agent understands the
// prune_request message. Agents older than 1.7.0 do not.
func (t *AgentTransport) PruneSupported() bool {
	return t.conn.SupportsFeature("1.7.0")
}

// HostTerminalSupported reports whether the connected agent understands
// the host_terminal_* messages. Agents older than 1.8.0 do not.
func (t *AgentTransport) HostTerminalSupported() bool {
	return t.conn.SupportsFeature("1.8.0")
}

// WriteJSON forwards a JSON message to the connected agent.
func (t *AgentTransport) WriteJSON(v interface{}) error {
	return t.conn.WriteJSON(v)
}

// HostLogsSupported reports whether the connected agent understands the
// host_log_* messages. Agents older than 1.8.0 do not.
func (t *AgentTransport) HostLogsSupported() bool {
	return t.conn.SupportsFeature("1.8.0")
}

// HostTerminalChannel exposes the in-flight host terminal waiter map for
// server code that bridges browser WebSockets to agent-backed shells.
func (t *AgentTransport) HostTerminalChannel() *HostTerminalChannel {
	return &HostTerminalChannel{waiters: t.hostTerminalWaiters, mu: t.mu}
}

// HostTerminalChannel is a thread-safe wrapper around the waiter map used
// for host terminal sessions.
type HostTerminalChannel struct {
	mu      sync.Mutex
	waiters map[string]chan *WSMessage
}

// Set registers a waiter for the given request id.
func (h *HostTerminalChannel) Set(reqID string, ch chan *WSMessage) {
	h.mu.Lock()
	h.waiters[reqID] = ch
	h.mu.Unlock()
}

// Pop removes and returns the waiter for the given request id.
func (h *HostTerminalChannel) Pop(reqID string) chan *WSMessage {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch, ok := h.waiters[reqID]
	if ok {
		delete(h.waiters, reqID)
	}
	return ch
}

// HostStats asks the agent to read /proc on the remote host and returns the
// resulting HostStatsPayload. Returns ok=false when the agent is not
// connected or does not support the feature.
func (t *AgentTransport) HostStats(ctx context.Context) (HostStatsPayload, bool, error) {
	if !t.HostStatsSupported() {
		return HostStatsPayload{}, false, fmt.Errorf("agent does not support host stats (requires 1.7.0+)")
	}
	reqID := xid.New().String()
	ch := make(chan *WSMessage, 1)

	t.mu.Lock()
	t.hostStatsWaiters[reqID] = ch
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		delete(t.hostStatsWaiters, reqID)
		t.mu.Unlock()
	}()

	if err := t.conn.WriteJSON(WSMessage{Type: MsgHostStatsRequest, ID: reqID}); err != nil {
		return HostStatsPayload{}, false, fmt.Errorf("sending host stats request to agent: %w", err)
	}

	select {
	case <-ctx.Done():
		return HostStatsPayload{}, false, ctx.Err()
	case msg := <-ch:
		if msg == nil || msg.HostStats == nil {
			return HostStatsPayload{}, false, fmt.Errorf("empty host stats response from agent")
		}
		return *msg.HostStats, msg.HostStats.Supported, nil
	case <-t.done:
		return HostStatsPayload{}, false, fmt.Errorf("agent transport closed")
	}
}

// Prune asks the agent to run a Docker prune operation locally and returns
// the result. Returns an error when the agent is not connected, does not
// support the feature, or the prune itself failed.
func (t *AgentTransport) Prune(ctx context.Context, payload PrunePayload) (*PrunePayload, error) {
	if !t.PruneSupported() {
		return nil, fmt.Errorf("agent does not support prune (requires 1.7.0+)")
	}
	reqID := xid.New().String()
	ch := make(chan *WSMessage, 1)

	t.mu.Lock()
	t.pruneWaiters[reqID] = ch
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		delete(t.pruneWaiters, reqID)
		t.mu.Unlock()
	}()

	if err := t.conn.WriteJSON(WSMessage{
		Type:  MsgPruneRequest,
		ID:    reqID,
		Prune: &payload,
	}); err != nil {
		return nil, fmt.Errorf("sending prune request to agent: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-ch:
		if msg == nil || msg.Prune == nil {
			return nil, fmt.Errorf("empty prune response from agent")
		}
		if !msg.Prune.Success {
			return msg.Prune, fmt.Errorf("%s", strings.TrimSpace(msg.Prune.Error))
		}
		return msg.Prune, nil
	case <-t.done:
		return nil, fmt.Errorf("agent transport closed")
	}
}

// HostLogs asks the agent to read host logs and returns the result as a
// stream of lines plus any non-fatal notices. Requires agent 1.8.0+.
func (t *AgentTransport) HostLogs(ctx context.Context, payload HostLogRequestPayload) (*HostLogStream, error) {
	if !t.HostLogsSupported() {
		return nil, fmt.Errorf("agent does not support host logs (requires 1.8.0+)")
	}
	reqID := xid.New().String()
	ch := make(chan *WSMessage, 256)

	t.mu.Lock()
	t.hostLogWaiters[reqID] = ch
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		delete(t.hostLogWaiters, reqID)
		t.mu.Unlock()
	}()

	if err := t.conn.WriteJSON(WSMessage{
		Type:            MsgHostLogRequest,
		ID:              reqID,
		HostLogRequest:  &payload,
	}); err != nil {
		return nil, fmt.Errorf("sending host log request to agent: %w", err)
	}

	stream := &HostLogStream{Lines: make(chan string, 256)}
	stream.errCh = make(chan error, 1)
	go func() {
		defer close(stream.Lines)
		for {
			select {
			case <-ctx.Done():
				stream.errCh <- ctx.Err()
				return
			case <-t.done:
				stream.errCh <- fmt.Errorf("agent transport closed")
				return
			case msg, ok := <-ch:
				if !ok {
					stream.errCh <- fmt.Errorf("host log channel closed")
					return
				}
				if msg.HostLogChunk != nil {
					select {
					case stream.Lines <- msg.HostLogChunk.Line:
					case <-ctx.Done():
						stream.errCh <- ctx.Err()
						return
					}
				}
				if msg.HostLogEnd != nil {
					stream.Notices = msg.HostLogEnd.Notices
					return
				}
			}
		}
	}()
	return stream, nil
}

// HostLogStream carries a stream of host log lines plus a list of non-fatal
// notices collected during the read.
type HostLogStream struct {
	Lines   chan string
	Notices []string
	errCh   chan error
}

// Err returns the terminal error of a host log stream (after Lines is
// closed). nil when the stream completed successfully.
func (s *HostLogStream) Err() error {
	select {
	case err := <-s.errCh:
		return err
	default:
		return nil
	}
}

// HostTerminalSendInput forwards stdin data to an active PTY-backed shell.
// The data is prefixed with the exec id header so the agent can route it
// to the correct session.
func (t *AgentTransport) HostTerminalSendInput(execID string, data []byte) error {
	if !t.HostTerminalSupported() {
		return fmt.Errorf("agent does not support host terminal (requires 1.8.0+)")
	}
	prefix := []byte("MCHARBOR_EXEC_ID=" + execID + "\n")
	combined := make([]byte, 0, len(prefix)+len(data))
	combined = append(combined, prefix...)
	combined = append(combined, data...)
	return t.conn.WriteJSON(WSMessage{
		Type:        MsgHostTerminalInput,
		StreamChunk: &WSStreamChunk{Data: combined},
	})
}

// HostTerminalSendResize forwards a TTY resize for an active shell.
func (t *AgentTransport) HostTerminalSendResize(execID string, cols, rows uint) error {
	if !t.HostTerminalSupported() {
		return fmt.Errorf("agent does not support host terminal (requires 1.8.0+)")
	}
	return t.conn.WriteJSON(WSMessage{
		Type:             MsgHostTerminalResize,
		HostTerminalRes:  &HostTerminalResizePayload{ExecID: execID, Cols: cols, Rows: rows},
	})
}

// HostTerminalSendEnd closes an active shell session.
func (t *AgentTransport) HostTerminalSendEnd(execID string) error {
	if !t.HostTerminalSupported() {
		return fmt.Errorf("agent does not support host terminal (requires 1.8.0+)")
	}
	return t.conn.WriteJSON(WSMessage{
		Type:             MsgHostTerminalEnd,
		HostTerminalEnd:  &HostTerminalEndPayload{ExecID: execID},
	})
}

// AgentTransport implements http.RoundTripper by proxying HTTP requests
// over a WebSocket connection to a remote agent.
type AgentTransport struct {
	conn                *AgentConnection
	db                  *sql.DB
	pending             map[string]*pendingReq
	execSessions        map[string]*ExecSession
	composeWaiters      map[string]chan *WSMessage
	hostStatsWaiters    map[string]chan *WSMessage
	pruneWaiters        map[string]chan *WSMessage
hostLogWaiters      map[string]chan *WSMessage
		hostTerminalWaiters map[string]chan *WSMessage
		transferWaiters     map[string]chan *WSMessage
	transferDiagnostics map[string]*TransferAuthDiagnostic
	transferReceivers   map[string]*TransferReceiverMarker
	mu                  sync.Mutex
	logger              *slog.Logger
	done                chan struct{}
}

// NewAgentTransport creates a new transport for the given agent connection.
func NewAgentTransport(conn *AgentConnection, db *sql.DB, logger *slog.Logger) *AgentTransport {
	return &AgentTransport{
		conn:                conn,
		db:                  db,
		pending:             make(map[string]*pendingReq),
		execSessions:        make(map[string]*ExecSession),
		composeWaiters:      make(map[string]chan *WSMessage),
		hostStatsWaiters:    make(map[string]chan *WSMessage),
		pruneWaiters:        make(map[string]chan *WSMessage),
		hostLogWaiters:      make(map[string]chan *WSMessage),
		hostTerminalWaiters: make(map[string]chan *WSMessage),
		transferWaiters:     make(map[string]chan *WSMessage),
		transferDiagnostics: make(map[string]*TransferAuthDiagnostic),
		transferReceivers:   make(map[string]*TransferReceiverMarker),
		logger:              logger,
		done:                make(chan struct{}),
	}
}

func (t *AgentTransport) registerTransferWaiter(transferID string) (chan *WSMessage, func()) {
	ch := make(chan *WSMessage, 64)
	t.mu.Lock()
	t.transferWaiters[transferID] = ch
	t.mu.Unlock()
	cleanup := func() {
		t.mu.Lock()
		delete(t.transferWaiters, transferID)
		t.mu.Unlock()
	}
	return ch, cleanup
}

// PrepareTransfer asks a target agent to open a one-use direct upload receiver.
func (t *AgentTransport) PrepareTransfer(ctx context.Context, transferID, token string) (string, error) {
	url, _, err := t.prepareTransfer(ctx, transferID, token, "", "", "")
	return url, err
}

// PrepareArchiveTransfer asks a target agent to open a one-use container archive receiver.
func (t *AgentTransport) PrepareArchiveTransfer(ctx context.Context, transferID, token, containerID, targetPath string) (string, error) {
	url, _, err := t.prepareTransfer(ctx, transferID, token, "archive", containerID, targetPath)
	return url, err
}

// PrepareProbe asks a target agent to open a one-use direct transfer probe receiver.
func (t *AgentTransport) PrepareProbe(ctx context.Context, transferID, token string) (string, *TransferReceiverMarker, error) {
	return t.prepareTransfer(ctx, transferID, token, TransferKindProbe, "", "")
}

func (t *AgentTransport) prepareTransfer(ctx context.Context, transferID, token, kind, containerID, targetPath string) (string, *TransferReceiverMarker, error) {
	ch, cleanup := t.registerTransferWaiter(transferID)
	defer cleanup()

	msg := WSMessage{
		Type: MsgTransferPrepare,
		Transfer: &TransferPayload{
			TransferID:  transferID,
			Kind:        kind,
			Token:       token,
			ContainerID: containerID,
			TargetPath:  targetPath,
		},
	}
	if err := t.conn.WriteJSON(msg); err != nil {
		return "", nil, fmt.Errorf("sending direct transfer prepare: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			t.cancelTransfer(transferID)
			return "", nil, ctx.Err()
		case msg := <-ch:
			if msg == nil || msg.Transfer == nil {
				continue
			}
			if msg.Type != MsgTransferReady && msg.Type != MsgTransferResult {
				continue
			}
			if !msg.Transfer.Success {
				if msg.Transfer.Error != "" {
					return "", nil, fmt.Errorf("direct transfer prepare failed: %s", msg.Transfer.Error)
				}
				return "", nil, fmt.Errorf("direct transfer prepare failed")
			}
			if strings.TrimSpace(msg.Transfer.URL) == "" {
				return "", nil, fmt.Errorf("direct transfer prepare returned no upload URL")
			}
			return msg.Transfer.URL, msg.Transfer.Receiver, nil
		case <-t.done:
			return "", nil, fmt.Errorf("agent transport closed")
		}
	}
}

// StartImageTransfer asks a source agent to stream an image directly to a target upload URL.
func (t *AgentTransport) StartImageTransfer(ctx context.Context, transferID, imageRef, uploadURL, token string, onProgress func(int64)) error {
	ch, cleanup := t.registerTransferWaiter(transferID)
	defer cleanup()

	msg := WSMessage{
		Type: MsgTransferImage,
		Transfer: &TransferPayload{
			TransferID: transferID,
			ImageRef:   imageRef,
			URL:        uploadURL,
			Token:      token,
		},
	}
	if err := t.conn.WriteJSON(msg); err != nil {
		return fmt.Errorf("sending direct image transfer command: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			t.cancelTransfer(transferID)
			return ctx.Err()
		case msg := <-ch:
			if msg == nil || msg.Transfer == nil {
				continue
			}
			switch msg.Type {
			case MsgTransferProgress:
				if onProgress != nil {
					onProgress(msg.Transfer.Bytes)
				}
			case MsgTransferResult:
				if msg.Transfer.Success {
					return nil
				}
				if msg.Transfer.Error != "" {
					return fmt.Errorf("direct image transfer failed: %s", msg.Transfer.Error)
				}
				return fmt.Errorf("direct image transfer failed")
			}
		case <-t.done:
			return fmt.Errorf("agent transport closed")
		}
	}
}

// StartImagePullTransfer asks an agent to pull an image archive from McHarbor and load it locally.
func (t *AgentTransport) StartImagePullTransfer(ctx context.Context, transferID, archiveURL, token string, onProgress func(int64, string)) error {
	return t.startPullTransfer(ctx, TransferPayload{
		TransferID: transferID,
		Kind:       "image",
		URL:        archiveURL,
		Token:      token,
	}, onProgress)
}

// StartRestoreTransfer asks an agent to pull a restore archive from McHarbor and apply it locally.
func (t *AgentTransport) StartRestoreTransfer(ctx context.Context, transferID, containerID, targetPath, archiveURL, token string, size int64, onProgress func(int64, string)) error {
	return t.startPullTransfer(ctx, TransferPayload{
		TransferID:  transferID,
		ContainerID: containerID,
		TargetPath:  targetPath,
		URL:         archiveURL,
		Token:       token,
		Size:        size,
	}, onProgress)
}

func (t *AgentTransport) startPullTransfer(ctx context.Context, payload TransferPayload, onProgress func(int64, string)) error {
	ch, cleanup := t.registerTransferWaiter(payload.TransferID)
	defer cleanup()

	msg := WSMessage{
		Type:     MsgTransferRestore,
		Transfer: &payload,
	}
	if err := t.conn.WriteJSON(msg); err != nil {
		return fmt.Errorf("sending restore transfer command: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			t.cancelTransfer(payload.TransferID)
			return ctx.Err()
		case msg := <-ch:
			if msg == nil || msg.Transfer == nil {
				continue
			}
			switch msg.Type {
			case MsgTransferProgress:
				if onProgress != nil {
					onProgress(msg.Transfer.Bytes, msg.Transfer.Stage)
				}
			case MsgTransferResult:
				if msg.Transfer.Success {
					return nil
				}
				if msg.Transfer.Error != "" {
					return fmt.Errorf("restore transfer failed: %s", msg.Transfer.Error)
				}
				return fmt.Errorf("restore transfer failed")
			}
		case <-t.done:
			return fmt.Errorf("agent transport closed")
		}
	}
}

// StartBackupRun asks an agent to create an encrypted backup archive locally and upload it.
func (t *AgentTransport) StartBackupRun(ctx context.Context, payload BackupPayload, onProgress func(BackupPayload)) (*BackupPayload, error) {
	return t.startBackupOperation(ctx, MsgBackupRun, payload, onProgress)
}

// StartBackupRestore asks an agent to download an encrypted archive and restore entries locally.
func (t *AgentTransport) StartBackupRestore(ctx context.Context, payload BackupPayload, onProgress func(BackupPayload)) (*BackupPayload, error) {
	return t.startBackupOperation(ctx, MsgBackupRestore, payload, onProgress)
}

func (t *AgentTransport) startBackupOperation(ctx context.Context, msgType string, payload BackupPayload, onProgress func(BackupPayload)) (*BackupPayload, error) {
	ch, cleanup := t.registerTransferWaiter(payload.TransferID)
	defer cleanup()

	if err := t.conn.WriteJSON(WSMessage{Type: msgType, Backup: &payload}); err != nil {
		return nil, fmt.Errorf("sending backup command to agent: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			t.cancelTransfer(payload.TransferID)
			return nil, ctx.Err()
		case msg := <-ch:
			if msg == nil || msg.Backup == nil {
				continue
			}
			switch msg.Type {
			case MsgTransferProgress:
				if onProgress != nil {
					onProgress(*msg.Backup)
				}
			case MsgTransferResult:
				if msg.Backup.Success {
					return msg.Backup, nil
				}
				if msg.Backup.Error != "" {
					return nil, fmt.Errorf("agent backup operation failed: %s", msg.Backup.Error)
				}
				return nil, fmt.Errorf("agent backup operation failed")
			}
		case <-t.done:
			return nil, fmt.Errorf("agent transport closed")
		}
	}
}

// StartArchiveTransfer asks a source agent to stream a container archive directly to a target upload URL.
func (t *AgentTransport) StartArchiveTransfer(ctx context.Context, transferID, containerID, sourcePath, uploadURL, token string, onProgress func(int64)) error {
	ch, cleanup := t.registerTransferWaiter(transferID)
	defer cleanup()

	msg := WSMessage{
		Type: MsgTransferArchive,
		Transfer: &TransferPayload{
			TransferID:  transferID,
			ContainerID: containerID,
			SourcePath:  sourcePath,
			URL:         uploadURL,
			Token:       token,
		},
	}
	if err := t.conn.WriteJSON(msg); err != nil {
		return fmt.Errorf("sending direct archive transfer command: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			t.cancelTransfer(transferID)
			return ctx.Err()
		case msg := <-ch:
			if msg == nil || msg.Transfer == nil {
				continue
			}
			switch msg.Type {
			case MsgTransferProgress:
				if onProgress != nil {
					onProgress(msg.Transfer.Bytes)
				}
			case MsgTransferResult:
				if msg.Transfer.Success {
					return nil
				}
				if msg.Transfer.Error != "" {
					return fmt.Errorf("direct archive transfer failed: %s", msg.Transfer.Error)
				}
				return fmt.Errorf("direct archive transfer failed")
			}
		case <-t.done:
			return fmt.Errorf("agent transport closed")
		}
	}
}

// StartTransferProbe asks a source agent to POST a lightweight probe to a target agent URL.
func (t *AgentTransport) StartTransferProbe(ctx context.Context, transferID, uploadURL, token string) (int, string, error) {
	ch, cleanup := t.registerTransferWaiter(transferID)
	defer cleanup()

	msg := WSMessage{
		Type: MsgTransferProbe,
		Transfer: &TransferPayload{
			TransferID: transferID,
			URL:        uploadURL,
			Token:      token,
		},
	}
	if err := t.conn.WriteJSON(msg); err != nil {
		return 0, "", fmt.Errorf("sending direct transfer probe command: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			t.cancelTransfer(transferID)
			return 0, "", ctx.Err()
		case msg := <-ch:
			if msg == nil || msg.Transfer == nil || msg.Type != MsgTransferResult {
				continue
			}
			responderAgentMarker := strings.TrimSpace(msg.Transfer.ResponderAgentMarker)
			if msg.Transfer.Success {
				return msg.Transfer.StatusCode, responderAgentMarker, nil
			}
			if msg.Transfer.Error != "" {
				return msg.Transfer.StatusCode, responderAgentMarker, fmt.Errorf("direct transfer probe failed: %s", msg.Transfer.Error)
			}
			return msg.Transfer.StatusCode, responderAgentMarker, fmt.Errorf("direct transfer probe failed")
		case <-t.done:
			return 0, "", fmt.Errorf("agent transport closed")
		}
	}
}

func (t *AgentTransport) CancelTransfer(transferID string) {
	t.cancelTransfer(transferID)
}

// WaitTransferDiagnostic waits briefly for token-safe receiver diagnostics from an agent.
func (t *AgentTransport) WaitTransferDiagnostic(ctx context.Context, transferID string, timeout time.Duration) *TransferAuthDiagnostic {
	if strings.TrimSpace(transferID) == "" {
		return nil
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		t.mu.Lock()
		diagnostic := t.transferDiagnostics[transferID]
		if diagnostic != nil {
			delete(t.transferDiagnostics, transferID)
		}
		t.mu.Unlock()
		if diagnostic != nil {
			return diagnostic
		}

		select {
		case <-ctx.Done():
			return nil
		case <-deadline.C:
			return nil
		case <-ticker.C:
		case <-t.done:
			return nil
		}
	}
}

func (t *AgentTransport) cancelTransfer(transferID string) {
	if strings.TrimSpace(transferID) == "" {
		return
	}
	if err := t.conn.WriteJSON(WSMessage{
		Type: MsgTransferCancel,
		Transfer: &TransferPayload{
			TransferID: transferID,
		},
	}); err != nil {
		t.logger.Debug("agent direct transfer cancel failed", "transferId", transferID, "error", err)
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
	if strings.HasSuffix(req.URL.Path, "/images/load") {
		return true
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

		case MsgComposeResult:
			t.mu.Lock()
			ch := t.composeWaiters[msg.ID]
			t.mu.Unlock()
			if ch != nil {
				select {
				case ch <- &msg:
				default:
					t.logger.Warn("agent compose result dropped", "id", msg.ID)
				}
			}

		case MsgHostStatsResponse:
			if msg.HostStats == nil {
				continue
			}
			t.mu.Lock()
			ch := t.hostStatsWaiters[msg.ID]
			t.mu.Unlock()
			if ch != nil {
				select {
				case ch <- &msg:
				default:
					t.logger.Warn("agent host stats response dropped", "id", msg.ID)
				}
			}

		case MsgPruneResponse:
			if msg.Prune == nil {
				continue
			}
			t.mu.Lock()
			ch := t.pruneWaiters[msg.ID]
			t.mu.Unlock()
			if ch != nil {
				select {
				case ch <- &msg:
				default:
					t.logger.Warn("agent prune response dropped", "id", msg.ID)
				}
			}

		case MsgHostLogChunk, MsgHostLogEnd:
			if msg.HostLogChunk == nil && msg.HostLogEnd == nil {
				continue
			}
			t.mu.Lock()
			ch := t.hostLogWaiters[msg.ID]
			t.mu.Unlock()
			if ch != nil {
				select {
				case ch <- &msg:
				default:
					t.logger.Warn("agent host log chunk dropped", "id", msg.ID)
				}
			}

		case MsgHostTerminalOutput, MsgHostTerminalEnd:
			if msg.StreamChunk == nil && msg.Type != MsgHostTerminalEnd {
				continue
			}
			t.mu.Lock()
			ch := t.hostTerminalWaiters[msg.ID]
			t.mu.Unlock()
			if ch != nil {
				// Use a buffered send so we never block the read loop.
				select {
				case ch <- &msg:
				default:
					t.logger.Warn("agent host terminal message dropped", "id", msg.ID, "type", msg.Type)
				}
			}

		case MsgTransferReady, MsgTransferProgress, MsgTransferResult:
			if msg.Transfer == nil && msg.Backup == nil {
				continue
			}
			if msg.Backup != nil {
				t.mu.Lock()
				ch := t.transferWaiters[msg.Backup.TransferID]
				t.mu.Unlock()
				if ch != nil {
					select {
					case ch <- &msg:
					default:
						t.logger.Warn("agent backup message dropped", "transferId", msg.Backup.TransferID, "type", msg.Type)
					}
				}
				continue
			}
			diagnosticOnly := msg.Type == MsgTransferResult && msg.Transfer.Diagnostic != nil
			t.mu.Lock()
			if msg.Transfer.Diagnostic != nil {
				t.transferDiagnostics[msg.Transfer.TransferID] = msg.Transfer.Diagnostic
			}
			if msg.Transfer.Receiver != nil {
				t.transferReceivers[msg.Transfer.TransferID] = msg.Transfer.Receiver
			}
			ch := t.transferWaiters[msg.Transfer.TransferID]
			t.mu.Unlock()
			if diagnosticOnly {
				continue
			}
			if ch != nil {
				select {
				case ch <- &msg:
				default:
					t.logger.Warn("agent direct transfer message dropped", "transferId", msg.Transfer.TransferID, "type", msg.Type)
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
