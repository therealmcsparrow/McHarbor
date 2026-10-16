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
	"github.com/therealmcsparrow/mcharbor/core/telemetry"
)

// pendingReq tracks an in-flight HTTP request waiting for a response.
type pendingReq struct {
	respCh        chan *WSMessage
	stream        *StreamReader // non-nil for streaming responses
	discardStream bool
}

type cancelableStreamBody struct {
	io.ReadCloser
	once    sync.Once
	onClose func()
}

func (b *cancelableStreamBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(b.onClose)
	return err
}

const agentRequestChunkSize = 32 * 1024
const maxBufferedAgentRequestBytes = 8 * 1024 * 1024
const maxAgentMessageBytes = 17 * 1024 * 1024

// KeepalivePeriod is how often the server sends a WebSocket control ping, and
// KeepaliveTimeout is how long it will wait for any frame before declaring the
// agent gone. The gap between them is deliberate: two pings may be lost before
// the connection is torn down.
//
// These are protocol-level pings, not the app-level "ping"/"pong" JSON the
// pool sends for health sampling. gorilla answers a control ping automatically
// from inside its read call, so this detects a half-open socket even against
// agents built before the agent-side keepalive existed — and detecting it is
// what frees the pool slot for the reconnecting agent.
const (
	KeepalivePeriod  = 30 * time.Second
	KeepaliveTimeout = 90 * time.Second
)
const agentRequestStreamingMinVersion = "1.3.3"

// TransferKindProbe is the transfer kind that measures reachability without moving data.
const TransferKindProbe = "probe"

// ExecSession tracks an active exec terminal session over the agent WebSocket.
type ExecSession struct {
	OutputCh chan []byte
	DoneCh   chan struct{}

	mu     sync.Mutex
	reason string
}

// RunCompose sends a compose command to the agent and waits for completion.
func (t *AgentTransport) RunCompose(ctx context.Context, payload ComposePayload) (r0 *ComposePayload, err error) {
	observe := telemetry.AgentRPC(t.conn.EnvID, "compose")
	defer func() { observe(err) }()
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
	// The mutex is shared by pointer, not copied. Copying it handed every
	// caller its own lock over the same map, so the wrapper's Set/Get did
	// not serialise against the transport's own use of t.mu at all.
	return &HostTerminalChannel{waiters: t.hostTerminalWaiters, mu: &t.mu}
}

// HostTerminalChannel is a thread-safe wrapper around the waiter map used
// for host terminal sessions.
type HostTerminalChannel struct {
	mu      *sync.Mutex
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
func (t *AgentTransport) HostStats(ctx context.Context) (r0 HostStatsPayload, r1 bool, err error) {
	observe := telemetry.AgentRPC(t.conn.EnvID, "host_stats")
	defer func() { observe(err) }()
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
func (t *AgentTransport) Prune(ctx context.Context, payload PrunePayload) (r0 *PrunePayload, err error) {
	observe := telemetry.AgentRPC(t.conn.EnvID, "prune")
	defer func() { observe(err) }()
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

// AgentUpdateMinVersion is the first agent release that understands
// MsgAgentUpdateRequest. Older agents must be upgraded by hand once.
const AgentUpdateMinVersion = "2.2.0"

// AgentUpdateSupported reports whether the connected agent can update itself.
func (t *AgentTransport) AgentUpdateSupported() bool {
	return t.conn.SupportsFeature(AgentUpdateMinVersion)
}

// AgentUpdate asks the agent to replace itself with the requested version
// and waits for the agent to acknowledge that the replacement was started.
// A successful response means the agent has begun the swap; the current
// connection will drop shortly afterwards and the new agent reconnects.
func (t *AgentTransport) AgentUpdate(ctx context.Context, payload AgentUpdatePayload) (r0 *AgentUpdatePayload, err error) {
	observe := telemetry.AgentRPC(t.conn.EnvID, "agent_update")
	defer func() { observe(err) }()
	if !t.AgentUpdateSupported() {
		return nil, fmt.Errorf("agent does not support self-update (requires %s+)", AgentUpdateMinVersion)
	}
	reqID := xid.New().String()
	ch := make(chan *WSMessage, 1)

	t.mu.Lock()
	t.agentUpdateWaiters[reqID] = ch
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		delete(t.agentUpdateWaiters, reqID)
		t.mu.Unlock()
	}()

	if err := t.conn.WriteJSON(WSMessage{
		Type:        MsgAgentUpdateRequest,
		ID:          reqID,
		AgentUpdate: &payload,
	}); err != nil {
		return nil, fmt.Errorf("sending agent update request: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-ch:
		if msg == nil || msg.AgentUpdate == nil {
			return nil, fmt.Errorf("empty agent update response")
		}
		if !msg.AgentUpdate.Success {
			return msg.AgentUpdate, fmt.Errorf("%s", strings.TrimSpace(msg.AgentUpdate.Error))
		}
		return msg.AgentUpdate, nil
	case <-t.done:
		return nil, fmt.Errorf("agent transport closed")
	}
}

// HostLogSourcesSupported reports whether the agent can read the libvirt, agent
// and per-guest qemu logs, which arrived after the four OS sources.
//
// The build is checked as well as the version, for the reason spelled out on
// HostLogSourcesMinBuild.
func (t *AgentTransport) HostLogSourcesSupported() bool {
	if !t.conn.SupportsFeature(HostLogSourcesMinVersion) {
		return false
	}
	if !t.conn.VersionEquals(HostLogSourcesMinVersion) {
		return true
	}
	return t.conn.BuildAtLeast(HostLogSourcesMinBuild)
}

// HostLogs asks the agent to read host logs and returns the result as a
// stream of lines plus any non-fatal notices. Requires agent 1.8.0+.
func (t *AgentTransport) HostLogs(ctx context.Context, payload HostLogRequestPayload) (r0 *HostLogStream, err error) {
	observe := telemetry.AgentRPC(t.conn.EnvID, "host_logs")
	defer func() { observe(err) }()
	if !t.HostLogsSupported() {
		return nil, fmt.Errorf("agent does not support host logs (requires 1.8.0+)")
	}
	reqID := xid.New().String()
	ch := make(chan *WSMessage, 256)

	t.mu.Lock()
	t.hostLogWaiters[reqID] = ch
	t.mu.Unlock()
	// The waiter must outlive this function. Unlike RunCompose and the other
	// waiter-map callers, HostLogs returns before the agent has answered, so a
	// function-scoped defer would unregister the id before the first chunk
	// arrived and ReadLoop would drop the whole stream onto the floor.
	release := func() {
		t.mu.Lock()
		delete(t.hostLogWaiters, reqID)
		t.mu.Unlock()
	}

	if err := t.conn.WriteJSON(WSMessage{
		Type:           MsgHostLogRequest,
		ID:             reqID,
		HostLogRequest: &payload,
	}); err != nil {
		release()
		return nil, fmt.Errorf("sending host log request to agent: %w", err)
	}

	stream := &HostLogStream{Lines: make(chan string, 256)}
	stream.errCh = make(chan error, 1)
	go func() {
		defer release()
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
					stream.AvailableFiles = msg.HostLogEnd.AvailableFiles
					stream.Targets = msg.HostLogEnd.Targets
					return
				}
			}
		}
	}()
	return stream, nil
}

// HostUpdatesSupported reports whether the agent understands the host OS
// inventory and package-update messages.
//
// The build is checked as well as the version: every agent released this month
// reports 2026.09.01, so the version alone would wave through the ones built
// before these handlers existed, and those hang rather than refuse.
func (t *AgentTransport) HostUpdatesSupported() bool {
	if !t.conn.SupportsFeature(HostUpdatesMinVersion) {
		return false
	}
	if !t.conn.VersionEquals(HostUpdatesMinVersion) {
		return true
	}
	return t.conn.BuildAtLeast(HostUpdatesMinBuild)
}

// HostUpdateRecoverySupported reports whether the agent honours Skip
// unavailable packages and retries a failed download once. An older agent still
// upgrades, it just ignores both, so this informs the operator rather than gates.
func (t *AgentTransport) HostUpdateRecoverySupported() bool {
	if !t.conn.SupportsFeature(HostUpdateRecoveryMinVersion) {
		return false
	}
	if !t.conn.VersionEquals(HostUpdateRecoveryMinVersion) {
		return true
	}
	return t.conn.BuildAtLeast(HostUpdateRecoveryMinBuild)
}

// HostUpdateApplyOptions is how far one host upgrade may go; the agent maps
// each field onto the package manager it finds.
type HostUpdateApplyOptions struct {
	WithNewPackages bool
	SkipUnavailable bool
	RemoveUnused    bool
}

// HostOSInfo asks the agent to describe the operating system it runs on.
func (t *AgentTransport) HostOSInfo(ctx context.Context) (r0 *HostOSInfoPayload, err error) {
	observe := telemetry.AgentRPC(t.conn.EnvID, "host_os_info")
	defer func() { observe(err) }()
	if !t.HostUpdatesSupported() {
		return nil, fmt.Errorf("agent does not support host OS info (requires %s+)", HostUpdatesMinVersion)
	}
	reqID := xid.New().String()
	ch := make(chan *WSMessage, 4)

	t.mu.Lock()
	t.hostOSInfoWaiters[reqID] = ch
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		delete(t.hostOSInfoWaiters, reqID)
		t.mu.Unlock()
	}()

	if err := t.conn.WriteJSON(WSMessage{Type: MsgHostOSInfoRequest, ID: reqID}); err != nil {
		return nil, fmt.Errorf("sending host os info request to agent: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-ch:
		if msg.HostOSInfo == nil {
			return nil, fmt.Errorf("empty host os info response from agent")
		}
		return msg.HostOSInfo, nil
	case <-t.done:
		return nil, fmt.Errorf("agent transport closed")
	}
}

// HostUpdateCheck asks the agent which host packages could be upgraded.
func (t *AgentTransport) HostUpdateCheck(ctx context.Context) (r0 *HostUpdatePayload, err error) {
	observe := telemetry.AgentRPC(t.conn.EnvID, "host_update_check")
	defer func() { observe(err) }()
	if !t.HostUpdatesSupported() {
		return nil, fmt.Errorf("agent does not support host updates (requires %s+)", HostUpdatesMinVersion)
	}
	reqID := xid.New().String()
	ch := make(chan *WSMessage, 4)

	t.mu.Lock()
	t.hostUpdateWaiters[reqID] = ch
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		delete(t.hostUpdateWaiters, reqID)
		t.mu.Unlock()
	}()

	if err := t.conn.WriteJSON(WSMessage{Type: MsgHostUpdateCheck, ID: reqID}); err != nil {
		return nil, fmt.Errorf("sending host update check to agent: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-ch:
		if msg.HostUpdate == nil {
			return nil, fmt.Errorf("empty host update response from agent")
		}
		return msg.HostUpdate, nil
	case <-t.done:
		return nil, fmt.Errorf("agent transport closed")
	}
}

// HostUpdateStream carries an upgrade's output as the agent produces it, and
// the terminal result once it finishes.
type HostUpdateStream struct {
	Lines  chan string
	result chan *HostUpdatePayload
	errCh  chan error
}

// Result blocks until the upgrade ends and returns its outcome. Lines must be
// drained first, or the sender will still be blocked filling them.
func (s *HostUpdateStream) Result() (*HostUpdatePayload, error) {
	select {
	case err := <-s.errCh:
		return nil, err
	case payload := <-s.result:
		return payload, nil
	}
}

// HostUpdateApply starts the host upgrade and streams its output.
//
// Like HostLogs this returns before the agent has answered, so the waiter is
// released by the forwarding goroutine and never by a function-scoped defer —
// doing the latter unregisters the id before the first frame lands and ReadLoop
// silently drops the entire stream.
func (t *AgentTransport) HostUpdateApply(ctx context.Context, opts HostUpdateApplyOptions) (r0 *HostUpdateStream, err error) {
	observe := telemetry.AgentRPC(t.conn.EnvID, "host_update_apply")
	defer func() { observe(err) }()
	if !t.HostUpdatesSupported() {
		return nil, fmt.Errorf("agent does not support host updates (requires %s+)", HostUpdatesMinVersion)
	}
	reqID := xid.New().String()
	ch := make(chan *WSMessage, 256)

	t.mu.Lock()
	t.hostUpdateWaiters[reqID] = ch
	t.mu.Unlock()
	release := func() {
		t.mu.Lock()
		delete(t.hostUpdateWaiters, reqID)
		t.mu.Unlock()
	}

	request := WSMessage{
		Type:       MsgHostUpdateApply,
		ID:         reqID,
		HostUpdate: &HostUpdatePayload{WithNewPackages: opts.WithNewPackages, SkipUnavailable: opts.SkipUnavailable, RemoveUnused: opts.RemoveUnused},
	}
	if err := t.conn.WriteJSON(request); err != nil {
		release()
		return nil, fmt.Errorf("sending host update apply to agent: %w", err)
	}

	stream := &HostUpdateStream{
		Lines:  make(chan string, 256),
		result: make(chan *HostUpdatePayload, 1),
		errCh:  make(chan error, 1),
	}
	go func() {
		defer release()
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
					stream.errCh <- fmt.Errorf("host update channel closed")
					return
				}
				if msg.HostUpdateChunk != nil {
					select {
					case stream.Lines <- msg.HostUpdateChunk.Line:
					case <-ctx.Done():
						stream.errCh <- ctx.Err()
						return
					}
				}
				if msg.Type == MsgHostUpdateEnd {
					stream.result <- msg.HostUpdate
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
	// AvailableFiles answers a target-less qemu request; empty for every other
	// source.
	AvailableFiles []string
	// Targets names the guests a qemu request can read; empty for every other source.
	Targets []HostLogTarget
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
		Type:            MsgHostTerminalResize,
		HostTerminalRes: &HostTerminalResizePayload{ExecID: execID, Cols: cols, Rows: rows},
	})
}

// HostTerminalSendEnd closes an active shell session.
func (t *AgentTransport) HostTerminalSendEnd(execID string) error {
	if !t.HostTerminalSupported() {
		return fmt.Errorf("agent does not support host terminal (requires 1.8.0+)")
	}
	return t.conn.WriteJSON(WSMessage{
		Type:            MsgHostTerminalEnd,
		HostTerminalEnd: &HostTerminalEndPayload{ExecID: execID},
	})
}

// OfflineBatchSink persists one batch of rows an agent buffered while it was
// disconnected. Implementations must be idempotent on entry ids: the agent
// deletes a batch only after the ack, so a batch whose ack was lost in a
// disconnect is sent again on the next connection.
type OfflineBatchSink interface {
	IngestOfflineBatch(ctx context.Context, envID string, batch *OfflineBatchPayload) error
}

// StatsPushSink applies one pushed stats batch and returns the ack to send
// back: whether it was applied or a resend, the next interval, or stop.
type StatsPushSink interface {
	IngestStatsPush(ctx context.Context, envID string, batch *StatsPushPayload) StatsPushPayload
}

// AgentTransport implements http.RoundTripper by proxying HTTP requests
// over a WebSocket connection to a remote agent.
//
//nolint:revive // renaming the exported type would break every importer outside this package
type AgentTransport struct {
	conn             *AgentConnection
	offlineSink      OfflineBatchSink
	statsSink        StatsPushSink
	db               *sql.DB
	pending          map[string]*pendingReq
	execSessions     map[string]*ExecSession
	composeWaiters   map[string]chan *WSMessage
	hostStatsWaiters map[string]chan *WSMessage
	pruneWaiters     map[string]chan *WSMessage
	snapshotWaiters  map[string]chan *WSMessage
	// unhandledTypes remembers which message types the read loop has already
	// warned about, so a type the agent sends in a loop cannot flood the log.
	unhandledTypes sync.Map

	// computeWaiters serves the request/reply compute families: capability
	// probe, pools, networks, images, and the vm_*/ct_* command results.
	computeWaiters map[string]chan *WSMessage
	// consoleSessions serves guest console byte streams. Unlike the waiters
	// above these are long-lived, and the session must be registered BEFORE
	// the start request is written — see StartConsole for why.
	consoleSessions map[string]*ConsoleSession
	// computeStreams carries progress frames for long agent operations
	// (image fetch, clone, migration) to whoever is showing the progress
	// dialog.
	computeStreams      map[string]chan *WSMessage
	hostLogWaiters      map[string]chan *WSMessage
	hostOSInfoWaiters   map[string]chan *WSMessage
	hostUpdateWaiters   map[string]chan *WSMessage
	hostServiceWaiters  map[string]chan *WSMessage
	hostLinkWaiters     map[string]chan *WSMessage
	hostTerminalWaiters map[string]chan *WSMessage
	transferWaiters     map[string]chan *WSMessage
	agentUpdateWaiters  map[string]chan *WSMessage
	transferDiagnostics map[string]*TransferAuthDiagnostic
	transferReceivers   map[string]*TransferReceiverMarker
	// relayTaps routes frames for sessions another node runs through this
	// connection to that node's relay instead of to the maps above.
	relayTaps map[string]RelayTap
	// closeReason is set once, when the read loop ends; disconnectReason is
	// what it falls back to when the socket drops without a close frame.
	closeReason      string
	disconnectReason string
	mu               sync.Mutex
	logger           *slog.Logger
	done             chan struct{}
	doneOnce         sync.Once
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
		snapshotWaiters:     make(map[string]chan *WSMessage),
		computeWaiters:      make(map[string]chan *WSMessage),
		consoleSessions:     make(map[string]*ConsoleSession),
		computeStreams:      make(map[string]chan *WSMessage),
		hostLogWaiters:      make(map[string]chan *WSMessage),
		hostOSInfoWaiters:   make(map[string]chan *WSMessage),
		hostUpdateWaiters:   make(map[string]chan *WSMessage),
		hostServiceWaiters:  make(map[string]chan *WSMessage),
		hostLinkWaiters:     make(map[string]chan *WSMessage),
		hostTerminalWaiters: make(map[string]chan *WSMessage),
		transferWaiters:     make(map[string]chan *WSMessage),
		agentUpdateWaiters:  make(map[string]chan *WSMessage),
		transferDiagnostics: make(map[string]*TransferAuthDiagnostic),
		transferReceivers:   make(map[string]*TransferReceiverMarker),
		relayTaps:           make(map[string]RelayTap),
		logger:              logger,
		done:                make(chan struct{}),
	}
}

// SetOfflineSink installs the handler for offline_batch messages. Without one
// the transport still acks batches — as rejected — so the agent keeps its data
// instead of waiting on a timeout every reconnect.
func (t *AgentTransport) SetOfflineSink(sink OfflineBatchSink) {
	t.mu.Lock()
	t.offlineSink = sink
	t.mu.Unlock()
}

// SetStatsSink installs the handler for stats_push messages. Without one the
// transport answers every batch with stop, so an agent never pushes into a void.
func (t *AgentTransport) SetStatsSink(sink StatsPushSink) {
	t.mu.Lock()
	t.statsSink = sink
	t.mu.Unlock()
}

// StatsPushSupported reports whether the agent can push container stats. The
// build is checked as well as the version for the reason StatsPushMinBuild gives.
func (t *AgentTransport) StatsPushSupported() bool {
	if !t.conn.SupportsFeature(StatsPushMinVersion) {
		return false
	}
	if !t.conn.VersionEquals(StatsPushMinVersion) {
		return true
	}
	return t.conn.BuildAtLeast(StatsPushMinBuild)
}

// SubscribeStats asks the agent to push container stats every interval. Sending
// it again retunes a running pusher rather than starting a second one.
func (t *AgentTransport) SubscribeStats(interval time.Duration) error {
	if !t.StatsPushSupported() {
		return fmt.Errorf("agent does not support stats push (requires %s build %s+)", StatsPushMinVersion, StatsPushMinBuild)
	}
	return t.conn.WriteJSON(WSMessage{
		Type:      MsgStatsSubscribe,
		StatsPush: &StatsPushPayload{IntervalSeconds: int(interval / time.Second)},
	})
}

// handleStatsPush applies one pushed batch and acks it. The agent keeps exactly
// one batch in flight and waits for this ack, so batches arrive in order even
// though each is handled off the read loop.
func (t *AgentTransport) handleStatsPush(ctx context.Context, msg *WSMessage) {
	t.mu.Lock()
	sink := t.statsSink
	t.mu.Unlock()

	ack := StatsPushPayload{Session: msg.StatsPush.Session, Seq: msg.StatsPush.Seq}
	if sink == nil {
		ack.Stop = true
		ack.Error = "stats push not supported by this server"
	} else {
		ingestCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		ack = sink.IngestStatsPush(ingestCtx, t.conn.EnvID, msg.StatsPush)
		cancel()
	}
	if err := t.conn.WriteJSON(WSMessage{Type: MsgStatsPushAck, ID: msg.ID, StatsPush: &ack}); err != nil {
		t.logger.Warn("agent stats push ack not sent", "env", t.conn.EnvID, "seq", ack.Seq, "error", err)
	}
}

// handleOfflineBatch persists a replayed batch and acks it. Runs on its own
// goroutine so a slow SQLite write never stalls the read loop that every
// proxied Docker call depends on.
func (t *AgentTransport) handleOfflineBatch(ctx context.Context, msg *WSMessage) {
	t.mu.Lock()
	sink := t.offlineSink
	t.mu.Unlock()

	ack := OfflineBatchPayload{BatchID: msg.Offline.BatchID, Kind: msg.Offline.Kind}
	switch sink {
	case nil:
		ack.Error = "offline replay not supported by this server"
	default:
		ingestCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 60*time.Second)
		err := sink.IngestOfflineBatch(ingestCtx, t.conn.EnvID, msg.Offline)
		cancel()
		if err != nil {
			t.logger.Error("agent offline batch ingest failed",
				"env", t.conn.EnvID, "kind", msg.Offline.Kind, "batchId", msg.Offline.BatchID, "error", err)
			ack.Error = "ingest failed"
		} else {
			ack.Accepted = true
			rows := len(msg.Offline.Logs) + len(msg.Offline.Events) + len(msg.Offline.Metrics)
			t.logger.Info("agent offline batch ingested",
				"env", t.conn.EnvID, "kind", msg.Offline.Kind, "rows", rows, "remaining", msg.Offline.Remaining)
		}
		rows := len(msg.Offline.Logs) + len(msg.Offline.Events) + len(msg.Offline.Metrics)
		telemetry.AgentOfflineBatch(t.conn.EnvID, msg.Offline.Kind, ack.Accepted, rows, msg.Offline.Remaining, oldestOfflineEntry(msg.Offline))
	}
	if err := t.conn.WriteJSON(WSMessage{Type: MsgOfflineBatchAck, ID: msg.ID, Offline: &ack}); err != nil {
		t.logger.Warn("agent offline batch ack not sent", "env", t.conn.EnvID, "batchId", ack.BatchID, "error", err)
	}
}

// relayAgentSpans hands spans the agent measured to the telemetry relay, which
// drops them when tracing is off.
func relayAgentSpans(envID string, spans []TraceSpan) {
	if len(spans) == 0 || !telemetry.TracingEnabled() {
		return
	}
	out := make([]telemetry.RelayedSpan, len(spans))
	for i, s := range spans {
		out[i] = telemetry.RelayedSpan{
			TraceID: s.TraceID, SpanID: s.SpanID, ParentSpanID: s.ParentSpanID,
			Name: s.Name, Kind: s.Kind,
			StartUnixNano: s.StartUnixNano, EndUnixNano: s.EndUnixNano,
			Attributes: s.Attributes, Error: s.Error,
		}
	}
	telemetry.RelayAgentSpans(envID, out)
}

// oldestOfflineEntry returns the earliest parseable timestamp in a replay batch,
// which is how far behind the replay is running when the batch lands.
func oldestOfflineEntry(batch *OfflineBatchPayload) time.Time {
	var oldest time.Time
	consider := func(stamp string) {
		at, err := time.Parse(time.RFC3339, stamp)
		if err == nil && (oldest.IsZero() || at.Before(oldest)) {
			oldest = at
		}
	}
	for _, e := range batch.Logs {
		consider(e.LoggedAt)
	}
	for _, e := range batch.Events {
		consider(e.Timestamp)
	}
	for _, e := range batch.Metrics {
		consider(e.SampledAt)
	}
	return oldest
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

func (t *AgentTransport) prepareTransfer(ctx context.Context, transferID, token, kind, containerID, targetPath string) (r0 string, r1 *TransferReceiverMarker, err error) {
	observe := telemetry.AgentRPC(t.conn.EnvID, "transfer_prepare")
	defer func() { observe(err) }()
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

func (t *AgentTransport) startPullTransfer(ctx context.Context, payload TransferPayload, onProgress func(int64, string)) (err error) {
	observe := telemetry.AgentRPC(t.conn.EnvID, "transfer_pull")
	defer func() { observe(err) }()
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

func (t *AgentTransport) startBackupOperation(ctx context.Context, msgType string, payload BackupPayload, onProgress func(BackupPayload)) (r0 *BackupPayload, err error) {
	observe := telemetry.AgentRPC(t.conn.EnvID, msgType)
	defer func() { observe(err) }()
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
func (t *AgentTransport) StartArchiveTransfer(ctx context.Context, transferID, containerID, sourcePath, uploadURL, token string, onProgress func(int64)) (err error) {
	observe := telemetry.AgentRPC(t.conn.EnvID, "transfer_archive")
	defer func() { observe(err) }()
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
func (t *AgentTransport) StartTransferProbe(ctx context.Context, transferID, uploadURL, token string) (r0 int, r1 string, err error) {
	observe := telemetry.AgentRPC(t.conn.EnvID, "transfer_probe")
	defer func() { observe(err) }()
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

// CancelTransfer releases the transfer's bookkeeping on this agent; safe to call for an ID that already finished.
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
		s.end("")
	}
}

// RoundTrip implements http.RoundTripper. It serializes the HTTP request,
// sends it over WebSocket, and blocks until the agent responds.
func (t *AgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	callerCtx := req.Context()
	releaseRequestContext := func() {}
	if _, hasDeadline := callerCtx.Deadline(); !hasDeadline {
		agentSettings := coreSettings.ReadAgentSettingsContext(callerCtx, t.db)
		timeout := time.Duration(agentSettings.RequestTimeout) * time.Second
		ctx, cancel := context.WithTimeout(callerCtx, timeout)
		releaseRequestContext = cancel
		req = req.WithContext(ctx)
	}
	defer releaseRequestContext()

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

	// The span's traceparent rides in wsReq.Headers, which the agent copies onto
	// the request it makes to its Docker daemon, so the trace crosses the hop.
	spanCtx, endSpan := telemetry.StartAgentSpan(req.Context(), t.conn.EnvID, req.Method, req.URL.Path, wsReq.Headers)
	req = req.WithContext(spanCtx)
	observe := telemetry.AgentRequestStarted(t.conn.EnvID)
	finish := func(outcome string, err error) {
		observe(outcome)
		endSpan(err)
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
		t.cancelPendingHTTP(reqID, pending)
		finish(telemetry.AgentOutcomeSendError, err)
		return nil, err
	}

	// Wait for response or context cancellation
	select {
	case <-req.Context().Done():
		cleanup()
		t.logger.Debug("agent RoundTrip: context cancelled", "id", reqID, "path", wsReq.Path)
		// Send cancel message
		cancel := WSMessage{Type: MsgHTTPCancel, ID: reqID}
		t.writeBestEffort(cancel)
		outcome := telemetry.AgentOutcomeCancelled
		if req.Context().Err() == context.DeadlineExceeded {
			outcome = telemetry.AgentOutcomeTimeout
		}
		finish(outcome, req.Context().Err())
		return nil, req.Context().Err()
	case resp := <-pending.respCh:
		if resp == nil {
			cleanup()
			err := fmt.Errorf("agent connection closed")
			finish(telemetry.AgentOutcomeClosed, err)
			return nil, err
		}
		t.logger.Debug("agent RoundTrip: response received", "id", reqID, "path", wsReq.Path, "type", resp.Type)
		httpResp, err := t.buildResponse(resp, pending)
		if err != nil {
			cleanup()
			finish(telemetry.AgentOutcomeError, err)
			return nil, err
		}
		finish(telemetry.AgentOutcomeOK, nil)
		// For streaming responses, keep the pending entry alive so
		// ReadLoop can dispatch chunks. Cleanup happens when the
		// stream ends (MsgHTTPResponseEnd). For regular responses,
		// clean up immediately.
		if pending.stream == nil {
			cleanup()
		} else {
			stream := pending.stream
			httpResp.Body = &cancelableStreamBody{
				ReadCloser: httpResp.Body,
				onClose: func() {
					t.cancelPendingHTTP(reqID, pending)
				},
			}
			go func() {
				select {
				case <-callerCtx.Done():
					stream.Fail(callerCtx.Err())
					t.cancelPendingHTTP(reqID, pending)
				case <-stream.Done():
				case <-t.done:
					stream.Fail(fmt.Errorf("agent transport closed"))
					t.cancelPendingHTTP(reqID, pending)
				}
			}()
		}
		return httpResp, nil
	case <-t.done:
		cleanup()
		err := fmt.Errorf("agent transport closed")
		finish(telemetry.AgentOutcomeClosed, err)
		return nil, err
	}
}

func (t *AgentTransport) cancelPendingHTTP(reqID string, pending *pendingReq) {
	t.mu.Lock()
	current, ok := t.pending[reqID]
	if ok && current == pending {
		delete(t.pending, reqID)
	}
	t.mu.Unlock()
	if !ok || current != pending {
		return
	}
	if err := t.conn.WriteJSON(WSMessage{Type: MsgHTTPCancel, ID: reqID}); err != nil {
		t.logger.Debug("agent HTTP cancel failed", "id", reqID, "error", err)
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
		body, err := io.ReadAll(io.LimitReader(req.Body, maxBufferedAgentRequestBytes+1))
		if err != nil {
			return fmt.Errorf("reading request body: %w", err)
		}
		if len(body) > maxBufferedAgentRequestBytes {
			return fmt.Errorf("request body exceeds %d-byte buffered limit", maxBufferedAgentRequestBytes)
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
	return t.ReadLoopContext(context.Background())
}

// ReadLoopContext is ReadLoop with a caller context that offline-batch ingests derive from, without inheriting its cancellation.
func (t *AgentTransport) ReadLoopContext(ctx context.Context) (loopErr error) {
	t.conn.Conn.SetReadLimit(maxAgentMessageBytes)
	// Any frame at all proves the far end is alive, so every read pushes the
	// deadline out; the pong handler covers the case where control pings are
	// the only traffic on an idle connection.
	if err := t.conn.Conn.SetReadDeadline(time.Now().Add(KeepaliveTimeout)); err != nil {
		return err
	}
	t.conn.Conn.SetPongHandler(func(string) error {
		return t.conn.Conn.SetReadDeadline(time.Now().Add(KeepaliveTimeout))
	})
	defer func() {
		t.failPendingHTTPStreams(fmt.Errorf("agent transport closed"))
		t.finish(loopErr)
	}()

	for {
		_, data, err := t.conn.Conn.ReadMessage()
		if err != nil {
			return err
		}
		if err := t.conn.Conn.SetReadDeadline(time.Now().Add(KeepaliveTimeout)); err != nil {
			return err
		}

		var msg WSMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			t.logger.Warn("agent: invalid message", "error", err)
			telemetry.AgentDropped(t.conn.EnvID, "invalid_message", 1)
			continue
		}

		// A session another node runs through this connection. Checked before
		// local dispatch so its frames never land in this node's waiter maps;
		// keepalives carry no id and so always stay local.
		if tap := t.relayTapFor(&msg); tap != nil {
			tap.DeliverRelay(data, &msg)
			continue
		}

		switch msg.Type {
		case MsgPong:
			// Keepalive reply. Stamping it is the whole basis of agent health:
			// an open socket proves nothing on its own, and this is the only
			// evidence that the far end is still processing frames.
			t.conn.MarkPong(time.Now())
			continue

		case MsgPing:
			// Agent sent ping, reply with pong
			pong := WSMessage{Type: MsgPong}
			t.writeBestEffort(pong)
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
			} else {
				telemetry.AgentDropped(t.conn.EnvID, "orphan_response", 1)
			}

		case MsgTraceSpans:
			relayAgentSpans(t.conn.EnvID, msg.Spans)

		case MsgHTTPResponseChunk:
			t.mu.Lock()
			p, ok := t.pending[msg.ID]
			t.mu.Unlock()
			if ok && p.discardStream {
				continue
			}
			if ok && p.stream != nil && msg.StreamChunk != nil {
				if !p.stream.Push(msg.StreamChunk.Data) {
					telemetry.AgentDropped(t.conn.EnvID, "stream_overflow", 1)
					t.cancelPendingHTTP(msg.ID, p)
				}
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
				s.end("")
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

		case MsgOfflineBatch:
			if msg.Offline == nil {
				continue
			}
			batch := msg
			go t.handleOfflineBatch(ctx, &batch)

		case MsgStatsPush:
			if msg.StatsPush == nil {
				continue
			}
			batch := msg
			go t.handleStatsPush(ctx, &batch)

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

		case MsgAgentUpdateResponse:
			if msg.AgentUpdate == nil {
				continue
			}
			t.mu.Lock()
			ch := t.agentUpdateWaiters[msg.ID]
			t.mu.Unlock()
			if ch != nil {
				select {
				case ch <- &msg:
				default:
					t.logger.Warn("agent update response dropped", "id", msg.ID)
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

		// A migration reports itself while it runs. It answers no request id,
		// so it goes to the event sink rather than the waiter map — the reply
		// the caller is blocked on arrives separately, when the move is done.
		case MsgComputeMigrateProgress:
			t.dispatchComputeEvent(&msg)

		// Compute request/reply families. Every one of these carries the
		// request id it is answering, so they all route through one waiter map.
		case MsgComputeProbeResponse, MsgComputeConfigResponse, MsgComputeInstallResponse,
			MsgComputeMigrateResponse,
			MsgPoolDetectResponse, MsgPoolCreateResponse, MsgPoolStatsResponse,
			MsgPoolRemoveResponse, MsgPoolBrowseResponse, MsgPoolFileResponse,
			MsgNetDetectResponse, MsgNetEnsureResponse, MsgNetRemoveResponse,
			MsgImageFetchResult, MsgImageListResponse, MsgImageRemoveResult,
			MsgVMResult, MsgVMState, MsgVMStats, MsgVMDiskResult,
			MsgVMSnapshotResult, MsgVMCloneResult, MsgVMGuestInfo,
			MsgVMMigrateResult,
			MsgCTResult, MsgCTState, MsgCTStats, MsgCTProcList,
			MsgCTLimitsResult, MsgCTSnapshotResult, MsgCTCloneResult,
			MsgCTBackupResult:
			t.mu.Lock()
			ch := t.computeWaiters[msg.ID]
			t.mu.Unlock()
			if ch != nil {
				select {
				case ch <- &msg:
				default:
					t.logger.Warn("agent compute response dropped", "id", msg.ID, "type", msg.Type)
				}
			}

		// Progress frames for long operations. Dropping one is harmless — the
		// next frame supersedes it and the result message is authoritative — so
		// these never block the read loop.
		case MsgImageFetchProgress, MsgVMCloneProgress, MsgVMMigrateProgress,
			MsgCTCloneProgress:
			t.mu.Lock()
			ch := t.computeStreams[msg.ID]
			t.mu.Unlock()
			if ch != nil {
				select {
				case ch <- &msg:
				default:
				}
			}

		// Unsolicited guest events and DHCP leases. These carry no request id
		// and are pushed to the registered sink rather than to a waiter. Import
		// progress is here too: the agent sends it without the id of the
		// request it belongs to, keyed by import id instead.
		case MsgVMEvent, MsgNetLeaseEvent, MsgCTBackupProgress, MsgVMImportProgress:
			t.dispatchComputeEvent(&msg)

		case MsgConsoleOutput:
			t.mu.Lock()
			session, ok := t.consoleSessions[msg.ID]
			t.mu.Unlock()
			if ok && msg.Console != nil {
				// Blocking here would stall every other guest's traffic behind one
				// slow browser, so a viewer that cannot keep up loses frames. For
				// VNC that is a repaint; for a text console it is a scrollback
				// gap. Neither is worth wedging the transport for.
				select {
				case session.OutputCh <- msg.Console.Data:
				default:
					t.logger.Warn("guest console frame dropped", "session", msg.ID)
				}
			}

		case MsgConsoleEnd:
			t.mu.Lock()
			session, ok := t.consoleSessions[msg.ID]
			delete(t.consoleSessions, msg.ID)
			t.mu.Unlock()
			if ok {
				if msg.Console != nil && msg.Console.Error != "" {
					session.setError(msg.Console.Error)
				}
				session.close()
			}

		case MsgSnapshotDetectResponse, MsgSnapshotCapabilityResponse,
			MsgSnapshotCreateResponse, MsgSnapshotRemoveResponse:
			if msg.Snapshot == nil {
				continue
			}
			t.mu.Lock()
			ch := t.snapshotWaiters[msg.ID]
			t.mu.Unlock()
			if ch != nil {
				select {
				case ch <- &msg:
				default:
					t.logger.Warn("agent snapshot response dropped", "id", msg.ID)
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

		case MsgHostOSInfoResponse:
			if msg.HostOSInfo == nil {
				continue
			}
			t.mu.Lock()
			ch := t.hostOSInfoWaiters[msg.ID]
			t.mu.Unlock()
			if ch != nil {
				select {
				case ch <- &msg:
				default:
					t.logger.Warn("agent host os info dropped", "id", msg.ID)
				}
			}

		case MsgHostUpdateResult, MsgHostUpdateChunk, MsgHostUpdateEnd:
			if msg.HostUpdate == nil && msg.HostUpdateChunk == nil {
				continue
			}
			t.mu.Lock()
			ch := t.hostUpdateWaiters[msg.ID]
			t.mu.Unlock()
			if ch != nil {
				select {
				case ch <- &msg:
				default:
					t.logger.Warn("agent host update frame dropped", "id", msg.ID)
				}
			}

		case MsgHostServiceResponse:
			if msg.HostService == nil {
				continue
			}
			t.mu.Lock()
			ch := t.hostServiceWaiters[msg.ID]
			t.mu.Unlock()
			if ch != nil {
				select {
				case ch <- &msg:
				default:
					t.logger.Warn("agent host service response dropped", "id", msg.ID)
				}
			}

		case MsgHostLinkResponse:
			if msg.HostLink == nil {
				continue
			}
			t.mu.Lock()
			ch := t.hostLinkWaiters[msg.ID]
			t.mu.Unlock()
			if ch != nil {
				select {
				case ch <- &msg:
				default:
					t.logger.Warn("agent host link response dropped", "id", msg.ID)
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

		// Never silent. A reply type missing from the cases above is discarded
		// here and the caller blocked on it waits out its whole timeout for an
		// answer that already arrived — which is how pool browse and every pool
		// file operation spent 60s per attempt returning nothing.
		default:
			if _, seen := t.unhandledTypes.LoadOrStore(msg.Type, struct{}{}); !seen {
				t.logger.Warn("agent: no handler for this message type; dropping it", "type", msg.Type)
			}
		}
	}
}

func (t *AgentTransport) failPendingHTTPStreams(err error) {
	t.mu.Lock()
	telemetry.AgentDropped(t.conn.EnvID, "disconnect", len(t.pending))
	streams := make([]*StreamReader, 0, len(t.pending))
	for id, pending := range t.pending {
		delete(t.pending, id)
		if pending.stream != nil {
			streams = append(streams, pending.stream)
		} else {
			select {
			case pending.respCh <- nil:
			default:
			}
		}
	}
	t.mu.Unlock()
	for _, stream := range streams {
		stream.Fail(err)
	}
}

// Close shuts down the transport.
func (t *AgentTransport) Close() {
	t.doneOnce.Do(func() { close(t.done) })
}

// writeBestEffort sends msg and only logs a failure: the callers (a cancel notice, a pong) run on a socket the read loop is about to notice is dead.
func (t *AgentTransport) writeBestEffort(msg WSMessage) {
	if err := t.conn.WriteJSON(msg); err != nil {
		t.logger.Debug("best-effort agent write failed", "type", msg.Type, "error", err)
	}
}
