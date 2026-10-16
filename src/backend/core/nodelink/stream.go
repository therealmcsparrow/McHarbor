// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package nodelink

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/therealmcsparrow/mcharbor/core/agent"
)

// The stream relay, and the contract every relayed path follows.
//
// A node that does not hold an environment's agent opens one signed WebSocket to the node
// that does and runs an ordinary agent.AgentTransport over it. The owning node relays
// frames verbatim in both directions, so exec, the host terminal, the guest console, host
// logs and every request/reply message work unchanged and the agent protocol stays frozen.
//
// Ordering. One socket per hop and one reader per socket, so a resize can never overtake
// the input written before it. A frame's ids are tapped before it is written to the agent,
// so the agent's first reply can never arrive ahead of its route.
//
// Backpressure. No hop may block the agent socket, because it multiplexes every session of
// the host. Each hop is bounded: the owner queues at most relayQueue frames toward the
// asking node and its writer blocks on the socket, not on memory. Past relayDataHeadroom,
// output frames are dropped exactly as a local session drops them when its reader is slow;
// a control frame that cannot be queued tears the link down rather than being lost.
//
// Close. A session the agent ends, ends. A link that drops ends every session on it with
// the reason in the close frame, and the owner sends the agent the end frame for each
// session the asking node never closed, so a vanished browser frees its process across
// both hops without waiting for a TCP timeout.

// Relay close codes, from the private-use range. The text of the close frame is the
// reason, and it reaches the operator unchanged.
const (
	CloseAgentGone = 4001
	CloseOverflow  = 4002
	CloseShutdown  = 4003
)

const (
	// relayQueue bounds the frames queued toward the asking node on one link.
	relayQueue = 512
	// relayDataHeadroom is where output frames start being dropped, leaving the rest of
	// the queue for the end and reply frames that must not be.
	relayDataHeadroom = relayQueue * 3 / 4
	// relayMaxTaps bounds the ids one link may route; the oldest reply ids go first.
	relayMaxTaps = 4096
	// relayWriteTimeout is how long an asking node may leave the owner's writer blocked
	// before the link is declared dead.
	relayWriteTimeout = 30 * time.Second
	// relayReadLimit matches the agent socket's own frame ceiling.
	relayReadLimit = 17 * 1024 * 1024
	// helloType names the one frame the node link adds: the owner's first frame, which
	// carries the agent's identity so the asking node applies the same feature gates.
	helloType = "node_link_hello"
	// streamClaimPath is the signed path of a stream request, so an agent-rpc signature
	// cannot open a stream and the other way round.
	streamClaimPath = "/agent-stream"
)

// hello is the owner's first frame on a stream link.
type hello struct {
	Type  string    `json:"type"`
	Node  string    `json:"node"`
	Agent agentInfo `json:"agent"`
}

// agentInfo is the part of an agent connection another node needs, with no socket.
type agentInfo struct {
	EnvID              string   `json:"envId"`
	Hostname           string   `json:"hostname"`
	OS                 string   `json:"os"`
	Arch               string   `json:"arch"`
	Version            string   `json:"version"`
	Build              string   `json:"build"`
	RemoteIP           string   `json:"remoteIp"`
	DockerVer          string   `json:"dockerVersion"`
	TLSMode            string   `json:"tlsMode"`
	ServerURL          string   `json:"serverUrl"`
	TransferURL        string   `json:"transferUrl"`
	TransferCandidates []string `json:"transferCandidates"`
}

// sessionKind names the relayed sessions that hold a process open on the agent, and so
// need an end frame when their link drops.
const (
	kindExec         = "exec"
	kindConsole      = "console"
	kindHostTerminal = "hostTerminal"
)

// tapState is one id the relay routes back to the asking node.
type tapState struct {
	kind   string
	execID string
	ended  bool
	seq    uint64
}

// relay is the owning node's half of one stream link.
type relay struct {
	link      *Link
	envID     string
	peer      string
	conn      *websocket.Conn
	transport *agent.AgentTransport
	out       chan []byte
	done      chan struct{}
	closeOnce sync.Once

	mu     sync.Mutex
	taps   map[string]*tapState
	seq    uint64
	closed bool

	dropped  atomic.Int64
	maxDepth atomic.Int64
}

var streamUpgrader = websocket.Upgrader{
	ReadBufferSize:  32 * 1024,
	WriteBufferSize: 32 * 1024,
	// A node carries no Origin, so an Origin header means a browser, which has no business
	// on a node-to-node relay; ServeAgentStream refuses it before reading the signature.
	CheckOrigin: func(r *http.Request) bool { return r.Header.Get("Origin") == "" }, // websocket origin: node-to-node relay refuses every browser Origin
}

// ServeAgentStream relays one node's agent sessions through this node's agent connection.
//
// Like ServeAgentRPC it refuses rather than chains: a node that does not hold the agent
// answers 409 before upgrading, so a stale lease is one readable failure.
func (l *Link) ServeAgentStream(w http.ResponseWriter, r *http.Request, envID string) {
	if refuseBrowser(w, r) {
		return
	}
	if l == nil || l.signer == nil {
		refuse(w, http.StatusServiceUnavailable, "this node has no shared secret and cannot verify a forwarded stream")
		return
	}
	if l.refusePlaintext(w, r, HopAgentStream) {
		return
	}
	claims, err := l.signer.Authenticate(r, HopAgentStream, envID, l.NodeID())
	if err != nil {
		l.refuseUnverified(w, r, HopAgentStream, err, "env", envID)
		return
	}
	if claims.Method != http.MethodGet || claims.Path != streamClaimPath {
		l.logRefusal(HopAgentStream, fmt.Errorf("%w: signed for %s %s, not a stream", ErrBadSignature, claims.Method, claims.Path),
			claims.NodeID, "env", envID)
		refuse(w, http.StatusUnauthorized, "the node signature is not valid")
		return
	}
	if !l.admitVerified(w, r, HopAgentStream, claims.NodeID) {
		return
	}
	if err := l.signer.AcceptEmpty(r); err != nil {
		l.logRefusal(HopAgentStream, err, claims.NodeID, "env", envID)
		refuse(w, http.StatusUnauthorized, "the node signature is not valid")
		return
	}
	transport := l.local.Transport(envID)
	if transport != nil {
		select {
		case <-transport.Done():
			// Still in the pool for the moment it takes the agent handler to notice;
			// relaying into it would only open a link that closes at once.
			transport = nil
		default:
		}
	}
	if transport == nil {
		refuse(w, http.StatusConflict, "this node does not hold the agent connection for "+envID)
		return
	}

	conn, err := streamUpgrader.Upgrade(w, r, nil)
	if err != nil {
		l.logger.Warn("nodelink: agent stream upgrade failed", "env", envID, "peer", claims.NodeID, "error", err)
		return
	}
	info := transport.ConnectionInfo()
	greeting := hello{Type: helloType, Node: l.NodeID(), Agent: agentInfo{
		EnvID: envID, Hostname: info.Hostname, OS: info.OS, Arch: info.Arch,
		Version: info.Version, Build: info.Build, RemoteIP: info.RemoteIP, DockerVer: info.DockerVer,
		TLSMode: info.TLSMode, ServerURL: info.ServerURL, TransferURL: info.TransferURL,
		TransferCandidates: info.TransferCandidates,
	}}
	greetErr := conn.SetWriteDeadline(time.Now().Add(relayWriteTimeout))
	if greetErr == nil {
		greetErr = conn.WriteJSON(greeting)
	}
	if greetErr != nil {
		l.logger.Warn("nodelink: agent stream greeting failed", "env", envID, "peer", claims.NodeID, "error", greetErr)
		closeQuietly(conn)
		return
	}

	rl := &relay{
		link: l, envID: envID, peer: claims.NodeID, conn: conn, transport: transport,
		out: make(chan []byte, relayQueue), done: make(chan struct{}), taps: map[string]*tapState{},
	}
	l.trackRelay(rl, true)
	l.logger.Info("nodelink: relaying agent sessions for another node", "env", envID, "peer", claims.NodeID)

	go rl.writeLoop()
	go rl.watch()
	readErr := rl.readLoop()
	rl.close(websocket.CloseNormalClosure, "")
	l.logger.Info("nodelink: agent stream relay ended", "env", envID, "peer", claims.NodeID,
		"dropped", rl.dropped.Load(), "error", readErr)
}

// readLoop writes every frame the asking node sends to the agent, after routing its ids.
func (rl *relay) readLoop() error {
	rl.conn.SetReadLimit(relayReadLimit)
	extend := func() error { return rl.conn.SetReadDeadline(time.Now().Add(agent.KeepaliveTimeout)) }
	if err := extend(); err != nil {
		return err
	}
	rl.conn.SetPingHandler(func(data string) error {
		if err := extend(); err != nil {
			return err
		}
		err := rl.conn.WriteControl(websocket.PongMessage, []byte(data), time.Now().Add(time.Second))
		if errors.Is(err, websocket.ErrCloseSent) {
			return nil
		}
		return err
	})
	for {
		_, data, err := rl.conn.ReadMessage()
		if err != nil {
			return err
		}
		if err := extend(); err != nil {
			return err
		}
		var msg agent.WSMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			rl.link.logger.Warn("nodelink: invalid relayed frame", "env", rl.envID, "peer", rl.peer, "error", err)
			continue
		}
		// Keepalives are per hop; the agent gets its own from this node's ping loop.
		if msg.Type == agent.MsgPing || msg.Type == agent.MsgPong {
			continue
		}
		if !rl.noteOutbound(&msg) {
			return errors.New("the relay is closing")
		}
		if err := rl.transport.WriteJSON(msg); err != nil {
			return fmt.Errorf("writing a relayed frame to the agent: %w", err)
		}
	}
}

// noteOutbound routes a frame's ids back to this relay before the agent can answer it, and
// remembers which sessions the asking node has already ended. It reports false once the
// relay is closing, so no id is routed to a relay that will never deliver it.
func (rl *relay) noteOutbound(msg *agent.WSMessage) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if rl.closed {
		return false
	}
	for _, key := range agent.RelayKeys(msg) {
		if _, ok := rl.taps[key]; ok {
			continue
		}
		if len(rl.taps) >= relayMaxTaps {
			rl.evictOldestLocked()
		}
		rl.seq++
		rl.taps[key] = &tapState{kind: sessionKindOf(msg.Type), seq: rl.seq}
		rl.transport.AddRelayTap(key, rl)
	}
	switch msg.Type {
	case agent.MsgExecEnd, agent.MsgConsoleEnd:
		if st, ok := rl.taps[msg.ID]; ok {
			st.ended = true
		}
	case agent.MsgHostTerminalEnd:
		if msg.HostTerminalEnd != nil {
			for _, st := range rl.taps {
				if st.kind == kindHostTerminal && st.execID == msg.HostTerminalEnd.ExecID {
					st.ended = true
				}
			}
		}
	}
	return true
}

// evictOldestLocked drops the oldest reply id. Sessions are never evicted: they end with
// an end frame, and losing one would strand a process on the agent.
func (rl *relay) evictOldestLocked() {
	oldestKey, oldestSeq := "", uint64(0)
	for key, st := range rl.taps {
		if st.kind != "" {
			continue
		}
		if oldestKey == "" || st.seq < oldestSeq {
			oldestKey, oldestSeq = key, st.seq
		}
	}
	if oldestKey != "" {
		delete(rl.taps, oldestKey)
		rl.transport.RemoveRelayTap(oldestKey, rl)
	}
}

// DeliverRelay queues an agent frame for the asking node. It runs on the agent's read
// loop, so it never blocks: see the backpressure contract at the top of this file.
func (rl *relay) DeliverRelay(raw []byte, msg *agent.WSMessage) {
	rl.noteInbound(msg)
	select {
	case <-rl.done:
		return
	default:
	}
	droppable := isOutputFrame(msg.Type)
	if droppable && len(rl.out) >= relayDataHeadroom {
		rl.dropped.Add(1)
		return
	}
	select {
	case rl.out <- raw:
		if depth := int64(len(rl.out)); depth > rl.maxDepth.Load() {
			rl.maxDepth.Store(depth)
		}
	default:
		rl.dropped.Add(1)
		if !droppable {
			// Not on the agent's read loop: close writes end frames to the agent.
			go rl.close(CloseOverflow, "the node relaying this agent could not keep up and dropped the link")
		}
	}
}

// noteInbound learns a host terminal's exec id from its first frame, and forgets an id
// whose reply has arrived.
func (rl *relay) noteInbound(msg *agent.WSMessage) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if msg.Type == agent.MsgHostTerminalOutput && msg.StreamChunk != nil {
		if st, ok := rl.taps[msg.ID]; ok && st.kind == kindHostTerminal && st.execID == "" {
			st.execID = hostTerminalExecID(msg.StreamChunk.Data)
		}
	}
	if !isFinalReply(msg.Type) {
		return
	}
	for _, key := range agent.RelayKeys(msg) {
		if _, ok := rl.taps[key]; ok {
			delete(rl.taps, key)
			rl.transport.RemoveRelayTap(key, rl)
		}
	}
}

// writeLoop sends queued frames to the asking node. Its blocking write is the hop's
// backpressure; the deadline is what turns a peer that stopped reading into a closed link.
func (rl *relay) writeLoop() {
	for {
		select {
		case <-rl.done:
			return
		case raw := <-rl.out:
			if err := rl.conn.SetWriteDeadline(time.Now().Add(relayWriteTimeout)); err != nil {
				rl.close(websocket.CloseAbnormalClosure, "")
				return
			}
			if err := rl.conn.WriteMessage(websocket.TextMessage, raw); err != nil {
				rl.close(websocket.CloseAbnormalClosure, "")
				return
			}
		}
	}
}

// watch ends the link when the agent leaves this node, naming that as the reason, which
// is how a session learns that its agent moved rather than that "the connection" broke.
func (rl *relay) watch() {
	select {
	case <-rl.done:
	case <-rl.transport.Done():
		reason := rl.transport.CloseReason()
		if reason == "" {
			reason = "the agent disconnected"
		}
		rl.close(CloseAgentGone, fmt.Sprintf("the agent for %s left node %s (%s); reopen to reach it where it reconnected",
			rl.envID, rl.link.NodeID(), reason))
	}
}

// close ends the link once: the reason goes to the asking node in the close frame, every
// id is released, and every session the asking node left open is ended on the agent.
func (rl *relay) close(code int, reason string) {
	rl.closeOnce.Do(func() {
		close(rl.done)
		if code != websocket.CloseAbnormalClosure {
			frame := websocket.FormatCloseMessage(code, truncateReason(reason))
			if err := rl.conn.WriteControl(websocket.CloseMessage, frame, time.Now().Add(time.Second)); err != nil {
				rl.link.logger.Debug("nodelink: close frame not sent", "error", err)
			}
		}
		closeQuietly(rl.conn)

		rl.mu.Lock()
		taps := rl.taps
		rl.taps = map[string]*tapState{}
		rl.closed = true
		rl.mu.Unlock()

		agentAlive := true
		select {
		case <-rl.transport.Done():
			agentAlive = false
		default:
		}
		for key, st := range taps {
			rl.transport.RemoveRelayTap(key, rl)
			if !agentAlive || st.ended {
				continue
			}
			if end, ok := endFrame(key, st); ok {
				if err := rl.transport.WriteJSON(end); err != nil {
					rl.link.logger.Warn("nodelink: ending a relayed session on the agent failed",
						"env", rl.envID, "session", key, "error", err)
				}
			}
		}
		rl.link.trackRelay(rl, false)
	})
}

// endFrame is the frame that frees a session's process on the agent.
func endFrame(key string, st *tapState) (agent.WSMessage, bool) {
	switch st.kind {
	case kindExec:
		return agent.WSMessage{Type: agent.MsgExecEnd, ID: key}, true
	case kindConsole:
		return agent.WSMessage{Type: agent.MsgConsoleEnd, ID: key,
			Console: &agent.ConsolePayload{SessionID: key}}, true
	case kindHostTerminal:
		if st.execID == "" {
			return agent.WSMessage{}, false
		}
		return agent.WSMessage{Type: agent.MsgHostTerminalEnd,
			HostTerminalEnd: &agent.HostTerminalEndPayload{ExecID: st.execID}}, true
	}
	return agent.WSMessage{}, false
}

func sessionKindOf(msgType string) string {
	switch msgType {
	case agent.MsgExecStart:
		return kindExec
	case agent.MsgConsoleStart:
		return kindConsole
	case agent.MsgHostTerminalStart:
		return kindHostTerminal
	}
	return ""
}

// isOutputFrame lists the frames a local session also drops when its reader is slow, so
// dropping them on the relay changes nothing the operator could not already see. The
// guest progress frames come from the plugin route ceiling, which is where they are declared.
func isOutputFrame(msgType string) bool {
	switch msgType {
	case agent.MsgExecOutput, agent.MsgConsoleOutput, agent.MsgHostTerminalOutput,
		agent.MsgHostLogChunk, agent.MsgHostUpdateChunk, agent.MsgTransferProgress:
		return true
	}
	return agent.IsPluginProgressFrame(msgType)
}

// isFinalReply lists the frames after which the agent sends nothing more under that id.
// A reply type missing here is not a bug, only an id held until the link closes.
func isFinalReply(msgType string) bool {
	switch msgType {
	case agent.MsgExecEnd, agent.MsgConsoleEnd, agent.MsgHostTerminalEnd, agent.MsgHostLogEnd,
		agent.MsgHostUpdateEnd, agent.MsgHTTPResponse, agent.MsgHTTPResponseEnd,
		agent.MsgComposeResult, agent.MsgPruneResponse, agent.MsgHostStatsResponse,
		agent.MsgHostOSInfoResponse, agent.MsgAgentUpdateResponse:
		return true
	}
	return false
}

// hostTerminalExecID reads the exec id header the agent puts on a shell's first frame.
func hostTerminalExecID(data []byte) string {
	const prefix = "MCHARBOR_EXEC_ID="
	if !bytes.HasPrefix(data, []byte(prefix)) {
		return ""
	}
	rest := data[len(prefix):]
	if i := bytes.IndexAny(rest, "\r\n\x00"); i >= 0 {
		return string(rest[:i])
	}
	return string(rest)
}

// truncateReason keeps a close reason inside the 123 bytes a close frame can carry.
func truncateReason(reason string) string {
	const limit = 120
	if len(reason) <= limit {
		return reason
	}
	return reason[:limit]
}

// trackRelay records or forgets a relay this node is running, so CloseStreams can end them.
func (l *Link) trackRelay(rl *relay, add bool) {
	l.streamMu.Lock()
	defer l.streamMu.Unlock()
	if add {
		l.relays[rl] = struct{}{}
		return
	}
	delete(l.relays, rl)
}

// RelayStats is what one owning-side relay has done, for diagnostics and tests.
type RelayStats struct {
	EnvID    string `json:"envId"`
	Peer     string `json:"peer"`
	Taps     int    `json:"taps"`
	Queued   int    `json:"queued"`
	MaxDepth int64  `json:"maxDepth"`
	Dropped  int64  `json:"dropped"`
}

// Relays reports every relay this node is running for other nodes.
func (l *Link) Relays() []RelayStats {
	if l == nil {
		return nil
	}
	l.streamMu.Lock()
	relays := make([]*relay, 0, len(l.relays))
	for rl := range l.relays {
		relays = append(relays, rl)
	}
	l.streamMu.Unlock()
	out := make([]RelayStats, 0, len(relays))
	for _, rl := range relays {
		rl.mu.Lock()
		taps := len(rl.taps)
		rl.mu.Unlock()
		out = append(out, RelayStats{EnvID: rl.envID, Peer: rl.peer, Taps: taps, Queued: len(rl.out),
			MaxDepth: rl.maxDepth.Load(), Dropped: rl.dropped.Load()})
	}
	return out
}
