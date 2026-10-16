// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package nodelink

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/agent"
)

const (
	// streamDialTimeout bounds opening a stream link, greeting included.
	streamDialTimeout = 10 * time.Second
	// streamRedialBackoff keeps an unreachable owner from costing every caller a full
	// dial timeout; the next attempt after it dials again.
	streamRedialBackoff = 15 * time.Second
)

// remoteAgent is one virtual agent connection, relayed through the node that holds it.
type remoteAgent struct {
	conn  *agent.AgentConnection
	owner string
}

// RemoteAgent returns a connection to envID's agent relayed through the node that holds
// it, dialling that node the first time; the pool's Resolve uses it as its fallback.
//
// The connection is an ordinary agent.AgentConnection over a node-to-node WebSocket, so
// exec, the host terminal, the console, host logs and every request/reply message run
// through it unchanged. It is shared by every caller on this node and lives until the
// owner drops it, which it does the moment the agent leaves that node.
func (l *Link) RemoteAgent(envID string) (*agent.AgentConnection, bool) {
	return l.remoteAgent(context.Background(), envID)
}

// remoteAgent is RemoteAgent with the caller's context for the owner lookup; the shared dial itself is detached from it so one caller leaving cannot abort the dial the others wait on.
func (l *Link) remoteAgent(ctx context.Context, envID string) (*agent.AgentConnection, bool) {
	if l == nil || l.owners == nil || envID == "" {
		return nil, false
	}
	for {
		l.streamMu.Lock()
		if ra := l.remotes[envID]; ra != nil {
			select {
			case <-ra.conn.Transport.Done():
				delete(l.remotes, envID)
			default:
				l.streamMu.Unlock()
				return ra.conn, true
			}
		}
		if at, failed := l.failedAt[envID]; failed && time.Since(at) < streamRedialBackoff {
			l.streamMu.Unlock()
			return nil, false
		}
		if wait := l.dialing[envID]; wait != nil {
			l.streamMu.Unlock()
			select {
			case <-wait:
				continue
			case <-time.After(streamDialTimeout + time.Second):
				return nil, false
			}
		}
		wait := make(chan struct{})
		l.dialing[envID] = wait
		l.streamMu.Unlock()

		dialCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), streamDialTimeout)
		ra, err := l.dialAgentStream(dialCtx, envID)
		cancel()

		l.streamMu.Lock()
		delete(l.dialing, envID)
		close(wait)
		if err != nil {
			if !errors.Is(err, errNoRemoteOwner) {
				l.failedAt[envID] = time.Now()
			}
			l.streamMu.Unlock()
			if !errors.Is(err, errNoRemoteOwner) {
				l.logger.Warn("nodelink: opening an agent stream failed", "env", envID, "error", err)
			}
			return nil, false
		}
		delete(l.failedAt, envID)
		l.remotes[envID] = ra
		l.streamMu.Unlock()
		return ra.conn, true
	}
}

// errNoRemoteOwner means no other node holds the agent, which is not a routing failure.
var errNoRemoteOwner = fmt.Errorf("%w on another node", ErrNoOwner)

// dialAgentStream opens the stream link to the owning node and starts the virtual
// connection's read loop and keepalive, each of which ends with the link.
func (l *Link) dialAgentStream(ctx context.Context, envID string) (*remoteAgent, error) {
	owner, ok := l.RemoteOwner(ctx, envID)
	if !ok {
		return nil, errNoRemoteOwner
	}
	fail := func(err error) (*remoteAgent, error) {
		l.recordStreamProof(Proof{Status: ProofFailed, NodeID: owner.NodeID, EnvID: envID,
			At: time.Now().UTC(), Error: err.Error()})
		return nil, err
	}
	if !l.CanForward() {
		return fail(fmt.Errorf("%w: agent for env %s is held by node %s", ErrCannotForward, envID, owner.NodeID))
	}
	if err := l.checkPeerURL(owner.NodeID, owner.AdvertiseURL); err != nil {
		return fail(err)
	}
	target, err := streamURL(owner.AdvertiseURL, envID)
	if err != nil {
		return fail(err)
	}

	signed := &http.Request{Header: http.Header{}}
	if err := l.signer.Sign(signed, Claims{Hop: HopAgentStream, NodeID: l.NodeID(), Audience: owner.NodeID, EnvID: envID,
		Method: http.MethodGet, Path: streamClaimPath}, xid.New().String(), emptyDigest()); err != nil {
		return fail(fmt.Errorf("nodelink: signing the agent stream to node %s: %w", owner.NodeID, err))
	}
	dialer := websocket.Dialer{
		// No proxy, matching the HTTP hop: a signed node-to-node upgrade goes straight to
		// the peer's advertise URL and never through HTTP(S)_PROXY.
		Proxy:            nil,
		HandshakeTimeout: streamDialTimeout,
		ReadBufferSize:   32 * 1024,
		WriteBufferSize:  32 * 1024,
		TLSClientConfig:  l.tlsConfig(),
	}
	ws, resp, err := dialer.DialContext(ctx, target, signed.Header)
	if resp != nil {
		defer closeQuietly(resp.Body)
	}
	if err != nil {
		if resp != nil {
			return fail(fmt.Errorf("nodelink: node %s refused the agent stream: %s", owner.NodeID, tunnelRefusal(resp)))
		}
		return fail(fmt.Errorf("nodelink: opening the agent stream to node %s: %w", owner.NodeID, err))
	}

	var greeting hello
	if deadline, ok := ctx.Deadline(); ok {
		if err := ws.SetReadDeadline(deadline); err != nil {
			l.logger.Debug("nodelink: greeting deadline not set", "env", envID, "error", err)
		}
	}
	if err := ws.ReadJSON(&greeting); err != nil || greeting.Type != helloType {
		closeQuietly(ws)
		if err == nil {
			err = fmt.Errorf("unexpected first frame %q", greeting.Type)
		}
		return fail(fmt.Errorf("nodelink: node %s did not greet the agent stream: %w", owner.NodeID, err))
	}

	info := greeting.Agent
	conn := &agent.AgentConnection{
		EnvID: envID, Hostname: info.Hostname, OS: info.OS, Arch: info.Arch,
		Version: info.Version, Build: info.Build, RemoteIP: info.RemoteIP, DockerVer: info.DockerVer,
		TLSMode: info.TLSMode, ServerURL: info.ServerURL, TransferURL: info.TransferURL,
		TransferCandidates: info.TransferCandidates, Conn: ws,
	}
	transport := agent.NewAgentTransport(conn, l.owners.database, l.logger)
	transport.SetDisconnectReason(fmt.Sprintf("the link to node %s, which holds this agent, dropped", owner.NodeID))
	conn.Transport = transport
	ra := &remoteAgent{conn: conn, owner: owner.NodeID}

	go func() {
		err := transport.ReadLoopContext(ctx)
		l.forgetRemote(envID, ra)
		l.logger.Info("nodelink: agent stream closed", "env", envID, "node", owner.NodeID,
			"reason", transport.CloseReason(), "error", err)
	}()
	go keepStreamAlive(ws, transport)
	l.logger.Info("nodelink: agent sessions for this environment are relayed through its node",
		"env", envID, "node", owner.NodeID)
	return ra, nil
}

// keepStreamAlive pings the owning node so its read deadline, and this side's through the
// pong handler ReadLoop installs, only expire when the link is really gone.
func keepStreamAlive(ws *websocket.Conn, transport *agent.AgentTransport) {
	ticker := time.NewTicker(agent.KeepalivePeriod)
	defer ticker.Stop()
	for {
		select {
		case <-transport.Done():
			return
		case <-ticker.C:
			if err := ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				closeQuietly(ws)
				return
			}
		}
	}
}

// forgetRemote drops a virtual connection that has ended, unless a newer one replaced it.
func (l *Link) forgetRemote(envID string, ra *remoteAgent) {
	l.streamMu.Lock()
	defer l.streamMu.Unlock()
	if l.remotes[envID] == ra {
		delete(l.remotes, envID)
	}
}

// streamURL turns an advertised base URL into the owner's stream endpoint for envID.
func streamURL(advertise, envID string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(advertise), "/")
	if base == "" {
		return "", fmt.Errorf("nodelink: the owning node advertises no address; set MCHARBOR_ADVERTISE_URL on it")
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("nodelink: the owning node's advertised address %q is not a URL: %w", advertise, err)
	}
	switch parsed.Scheme {
	case "https":
		parsed.Scheme = "wss"
	case "http":
		parsed.Scheme = "ws"
	default:
		return "", fmt.Errorf("nodelink: the owning node's advertised address %q is not http or https", advertise)
	}
	return parsed.String() + "/api/internal/node/agent-stream/" + url.PathEscape(envID), nil
}

// CloseStreams ends every stream link this node holds in either role, for shutdown: the
// sessions on them end with a reason instead of waiting for a keepalive to expire.
func (l *Link) CloseStreams() {
	if l == nil {
		return
	}
	l.streamMu.Lock()
	remotes := make([]*remoteAgent, 0, len(l.remotes))
	for _, ra := range l.remotes {
		remotes = append(remotes, ra)
	}
	relays := make([]*relay, 0, len(l.relays))
	for rl := range l.relays {
		relays = append(relays, rl)
	}
	l.streamMu.Unlock()
	for _, ra := range remotes {
		frame := websocket.FormatCloseMessage(websocket.CloseGoingAway, "node "+l.NodeID()+" is shutting down")
		if err := ra.conn.Conn.WriteControl(websocket.CloseMessage, frame, time.Now().Add(time.Second)); err != nil {
			l.logger.Debug("nodelink: close frame to the agent stream not sent", "env", ra.conn.EnvID, "error", err)
		}
		closeQuietly(ra.conn.Conn)
	}
	for _, rl := range relays {
		rl.close(CloseShutdown, "node "+l.NodeID()+", which holds this agent, is shutting down")
	}
}

// StreamProof is the last verdict on relaying an agent session through another node.
func (l *Link) StreamProof() Proof {
	if l == nil {
		return Proof{Status: ProofUnproven}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.streamProof
}

// recordStreamProof keeps the newest stream verdict, with the same failure-first rule as
// recordProof.
func (l *Link) recordStreamProof(p Proof) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if p.Status == ProofProven && l.streamProof.Status == ProofFailed && l.streamProof.EnvID != p.EnvID &&
		time.Since(l.streamProof.At) < probeEvery {
		return
	}
	l.streamProof = p
}
