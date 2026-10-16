// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"context"
	"database/sql"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
	"github.com/therealmcsparrow/mcharbor/core/telemetry"
)

// transferRoute is a direct-transfer base URL proven to reach this agent from
// one particular source agent, and the moment that proof goes stale.
type transferRoute struct {
	base      string
	expiresAt time.Time
}

// AgentConnection represents a connected remote agent.
//
//nolint:revive // renaming the exported type would break every importer outside this package
type AgentConnection struct {
	EnvID    string
	Hostname string
	OS       string
	Arch     string
	Version  string
	// Build is the agent artifact's yyyyMMddHHmm stamp, empty for an agent too
	// old to report one.
	Build string
	// RemoteIP is the address this agent appeared to connect from, resolved
	// through the trusted-proxy rules rather than taken from a header.
	RemoteIP  string
	DockerVer string
	// TLSMode is how the agent secured its side of the control channel:
	// "pinned", "verified", "insecure", "plaintext", or "encrypted" when the
	// agent is too old to report one and only the request scheme is known.
	TLSMode string
	// ServerURL is the McHarbor address this agent actually dialled, taken from
	// its own connection request. It is the one address known to be reachable
	// from that host, which is what makes it the right base for anything the
	// agent has to fetch back from us.
	ServerURL string
	// TransferURL is the agent's primary direct transfer base URL, kept for
	// agents too old to advertise a candidate list.
	TransferURL string
	// TransferCandidates lists every base URL a peer agent may try, in
	// preference order, including the reflexive one the server derived.
	TransferCandidates []string
	// Capabilities is what the agent said it can serve on this host at
	// handshake, such as HostServiceCapability. Nil for an agent too old to
	// report any, and for a connection relayed from another node.
	Capabilities []string
	Conn         *websocket.Conn
	Transport    *AgentTransport
	mu           sync.Mutex

	// transferRoutes caches the winning candidate per source environment. It
	// is keyed by source because reachability is a property of the pair, not
	// of this agent alone.
	routeMu        sync.Mutex
	transferRoutes map[string]transferRoute

	// Keepalive observations, written by StartPingLoop (the send side) and by
	// AgentTransport.ReadLoop when a pong arrives (the receive side).
	//
	// Until this existed the pong was discarded outright, which meant a socket
	// that was open but no longer carrying traffic was indistinguishable from a
	// healthy one. The pair gives both liveness and a round-trip latency; the
	// health derivation in modules/agent reads it through AgentPool.Keepalive.
	kaMu       sync.Mutex
	lastPingAt time.Time
	lastPongAt time.Time
	latency    time.Duration
}

// Keepalive is one read of a connection's ping/pong observations.
//
// LastPong is zero when the agent has not answered a ping yet, which is the
// normal state for the first few seconds of a connection — callers must treat
// that as "not yet measured" rather than as a missed keepalive.
type Keepalive struct {
	LastPing time.Time
	LastPong time.Time
	Latency  time.Duration
}

// MarkPing records that a keepalive was just written to this connection.
func (ac *AgentConnection) MarkPing(at time.Time) {
	ac.kaMu.Lock()
	defer ac.kaMu.Unlock()
	ac.lastPingAt = at
}

// MarkPong records a keepalive reply and, when it can be paired with the ping
// that prompted it, the round-trip latency.
//
// A pong with no outstanding ping is still worth recording as liveness, so the
// timestamp is always stamped and only the latency is conditional.
func (ac *AgentConnection) MarkPong(at time.Time) {
	ac.kaMu.Lock()
	defer ac.kaMu.Unlock()
	ac.lastPongAt = at
	if !ac.lastPingAt.IsZero() && !at.Before(ac.lastPingAt) {
		ac.latency = at.Sub(ac.lastPingAt)
		telemetry.AgentKeepalive(ac.EnvID, ac.latency)
	}
}

// Keepalive returns a snapshot of this connection's ping/pong observations.
func (ac *AgentConnection) Keepalive() Keepalive {
	ac.kaMu.Lock()
	defer ac.kaMu.Unlock()
	return Keepalive{LastPing: ac.lastPingAt, LastPong: ac.lastPongAt, Latency: ac.latency}
}

// WriteMessage sends a WebSocket message with write locking.
func (ac *AgentConnection) WriteMessage(messageType int, data []byte) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	return ac.Conn.WriteMessage(messageType, data)
}

// WriteJSON sends a JSON WebSocket message with write locking.
func (ac *AgentConnection) WriteJSON(v interface{}) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	return ac.Conn.WriteJSON(v)
}

// AgentPool manages connected remote agents by environment ID.
//
//nolint:revive // renaming the exported type would break every importer outside this package
type AgentPool struct {
	mu     sync.RWMutex
	conns  map[string]*AgentConnection
	logger *slog.Logger

	// pingInterval is the keepalive period StartPingLoop is running at, kept so
	// TelemetryStats can judge staleness without reading settings from the DB.
	pingInterval atomic.Int64

	// remote resolves an agent another node holds to a virtual connection
	// relayed through that node; nil outside active/active.
	remote atomic.Pointer[RemoteResolver]
}

// RemoteResolver returns a connection to an agent held by another node, or false when no
// node holds it or the node holding it cannot be reached.
type RemoteResolver func(envID string) (*AgentConnection, bool)

// SetRemoteResolver installs how Resolve reaches an agent this node does not hold.
func (p *AgentPool) SetRemoteResolver(resolve RemoteResolver) {
	if resolve == nil {
		p.remote.Store(nil)
		return
	}
	p.remote.Store(&resolve)
}

// Resolve returns envID's agent connection wherever it is held: the local one when this
// node holds it, and otherwise a connection relayed through the node that does.
//
// Get stays local-only on purpose: the lease renewal, the status pages and the forwarding
// hop itself must not mistake a relayed connection for one this node holds.
func (p *AgentPool) Resolve(envID string) (*AgentConnection, bool) {
	if conn, ok := p.Get(envID); ok {
		return conn, true
	}
	resolve := p.remote.Load()
	if resolve == nil {
		return nil, false
	}
	return (*resolve)(envID)
}

// ResolveTransport is Resolve for callers that only need the transport.
func (p *AgentPool) ResolveTransport(envID string) *AgentTransport {
	conn, ok := p.Resolve(envID)
	if !ok || conn == nil {
		return nil
	}
	return conn.Transport
}

// NewAgentPool creates a new agent pool.
func NewAgentPool(logger *slog.Logger) *AgentPool {
	return &AgentPool{
		conns:  make(map[string]*AgentConnection),
		logger: logger,
	}
}

// Register adds an agent connection to the pool and reports whether it became
// the active connection for the environment.
func (p *AgentPool) Register(envID string, conn *AgentConnection) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Close existing connection if any.
	//
	// An equal version takes over rather than being turned away. The
	// connection already in the map may be half-open — a NAT eviction or a
	// router blip leaves a socket nothing has read from since, and without
	// keepalive that state used to persist for as long as the kernel kept
	// retransmitting. A connection that just completed the auth handshake is
	// strictly better evidence of a live agent than a silent one, so the
	// newcomer wins ties. Only a strictly older agent is refused, which is
	// what keeps a stale binary from stealing the slot back from the
	// upgraded one mid-self-update.
	if existing, ok := p.conns[envID]; ok {
		if compareAgentVersions(conn.Version, existing.Version) < 0 {
			p.closeConn(conn.Conn, envID)
			p.logger.Warn("older duplicate agent rejected; current connection remains active", "env", envID, "hostname", conn.Hostname, "agentVersion", conn.Version, "currentVersion", existing.Version)
			return false
		}
		p.logger.Info("replacing existing agent connection", "env", envID, "hostname", conn.Hostname, "agentVersion", conn.Version, "previousVersion", existing.Version)
		p.closeConn(existing.Conn, envID)
		telemetry.AgentDisconnected(envID)
	}
	p.conns[envID] = conn
	telemetry.AgentConnected(envID)
	p.logger.Info("agent registered", "env", envID, "hostname", conn.Hostname)
	return true
}

func compareAgentVersions(a, b string) int {
	a = strings.TrimPrefix(strings.TrimSpace(a), "v")
	b = strings.TrimPrefix(strings.TrimSpace(b), "v")
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")
	for i := 0; i < 3; i++ {
		aPart := agentVersionPart(aParts, i)
		bPart := agentVersionPart(bParts, i)
		if aPart > bPart {
			return 1
		}
		if aPart < bPart {
			return -1
		}
	}
	return 0
}

func agentVersionPart(parts []string, index int) int {
	if index >= len(parts) {
		return 0
	}
	value, err := strconv.Atoi(parts[index])
	if err != nil {
		return 0
	}
	return value
}

// Remove removes an agent connection from the pool.
func (p *AgentPool) Remove(envID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.conns[envID]; ok {
		telemetry.AgentDisconnected(envID)
	}
	delete(p.conns, envID)
	p.logger.Info("agent removed", "env", envID)
}

// RemoveIfCurrent removes an agent connection only if it still matches the
// connection currently registered for the environment.
func (p *AgentPool) RemoveIfCurrent(envID string, conn *AgentConnection) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if current, ok := p.conns[envID]; ok && current == conn {
		delete(p.conns, envID)
		telemetry.AgentDisconnected(envID)
		p.logger.Info("agent removed", "env", envID)
		return true
	}
	p.logger.Debug("agent remove skipped; newer connection is registered", "env", envID)
	return false
}

// Get returns an agent connection for the given environment ID.
func (p *AgentPool) Get(envID string) (*AgentConnection, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	conn, ok := p.conns[envID]
	return conn, ok
}

// Version returns the connected agent version for an environment.
func (p *AgentPool) Version(envID string) (string, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	conn, ok := p.conns[envID]
	if !ok {
		return "", false
	}
	return conn.Version, true
}

// Keepalive returns the keepalive snapshot for an environment's agent, and
// false when no agent is connected.
func (p *AgentPool) Keepalive(envID string) (Keepalive, bool) {
	p.mu.RLock()
	conn, ok := p.conns[envID]
	p.mu.RUnlock()
	if !ok {
		return Keepalive{}, false
	}
	return conn.Keepalive(), true
}

// IsConnected checks if an agent is connected for the given environment.
func (p *AgentPool) IsConnected(envID string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	_, ok := p.conns[envID]
	return ok
}

// Transport returns the agent transport for the given environment, or nil
// when no agent is connected.
func (p *AgentPool) Transport(envID string) *AgentTransport {
	p.mu.RLock()
	defer p.mu.RUnlock()
	conn, ok := p.conns[envID]
	if !ok {
		return nil
	}
	return conn.Transport
}

// SupportsFeature reports whether the connected agent version meets or
// exceeds the required minimum (semver-like). Used by the metrics module to
// decide whether to ask the agent for host stats or fall back to the
// "unavailable" state.
func (ac *AgentConnection) SupportsFeature(minimum string) bool {
	ac.mu.Lock()
	v := ac.Version
	ac.mu.Unlock()
	return agentAtLeast(v, minimum)
}

// VersionEquals reports whether the connected agent runs exactly this version.
func (ac *AgentConnection) VersionEquals(version string) bool {
	ac.mu.Lock()
	v := ac.Version
	ac.mu.Unlock()
	return strings.TrimSpace(v) == strings.TrimSpace(version)
}

// BuildAtLeast reports whether the agent's build stamp is at or after minimum.
//
// An agent that reports no build predates build stamping entirely, so it is
// older than any stamp a caller can name and the answer is false.
func (ac *AgentConnection) BuildAtLeast(minimum string) bool {
	ac.mu.Lock()
	build := strings.TrimSpace(ac.Build)
	ac.mu.Unlock()
	if build == "" || build == "dev" {
		return false
	}
	return build >= strings.TrimSpace(minimum)
}

// agentAtLeast compares two dotted-triple version strings and reports
// whether a is >= b. Equal-length components are compared numerically;
// missing components are treated as 0.
func agentAtLeast(a, minimum string) bool {
	a = strings.TrimPrefix(strings.TrimSpace(a), "v")
	minimum = strings.TrimPrefix(strings.TrimSpace(minimum), "v")
	aParts := strings.Split(a, ".")
	mParts := strings.Split(minimum, ".")
	for i := 0; i < 3; i++ {
		ap := versionPart(aParts, i)
		mp := versionPart(mParts, i)
		if ap > mp {
			return true
		}
		if ap < mp {
			return false
		}
	}
	return true
}

func versionPart(parts []string, idx int) int {
	if idx >= len(parts) {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(parts[idx]))
	if err != nil {
		return 0
	}
	return n
}

// TelemetryStats classifies every pooled connection by keepalive state from
// memory alone; stale uses the same two-missed-intervals rule as agent health.
func (p *AgentPool) TelemetryStats() telemetry.AgentPoolStats {
	interval := time.Duration(p.pingInterval.Load())
	now := time.Now()
	p.mu.RLock()
	defer p.mu.RUnlock()
	stats := telemetry.AgentPoolStats{Connected: len(p.conns)}
	for _, conn := range p.conns {
		ka := conn.Keepalive()
		switch {
		case ka.LastPong.IsZero():
			stats.Measuring++
		case interval > 0 && now.Sub(ka.LastPong) > 2*interval:
			stats.Stale++
		}
	}
	return stats
}

// List returns info about all connected agents.
func (p *AgentPool) List() []AgentConnection {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]AgentConnection, 0, len(p.conns))
	for _, conn := range p.conns {
		result = append(result, AgentConnection{
			EnvID:     conn.EnvID,
			Hostname:  conn.Hostname,
			OS:        conn.OS,
			Arch:      conn.Arch,
			Version:   conn.Version,
			Build:     conn.Build,
			RemoteIP:  conn.RemoteIP,
			DockerVer: conn.DockerVer,
			ServerURL: conn.ServerURL,
		})
	}
	return result
}

// StartPingLoop sends pings to all connected agents at the configured interval.
func (p *AgentPool) StartPingLoop(ctx context.Context, db *sql.DB) {
	agentSettings := coreSettings.ReadAgentSettingsContext(ctx, db)
	currentInterval := time.Duration(agentSettings.PingInterval) * time.Second
	p.pingInterval.Store(int64(currentInterval))
	ticker := time.NewTicker(currentInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Re-read settings and adjust ticker if interval changed
			agentSettings = coreSettings.ReadAgentSettingsContext(ctx, db)
			newInterval := time.Duration(agentSettings.PingInterval) * time.Second
			if newInterval != currentInterval {
				ticker.Reset(newInterval)
				currentInterval = newInterval
				p.pingInterval.Store(int64(newInterval))
				p.logger.Info("agent ping interval updated", "interval", newInterval)
			}

			p.mu.RLock()
			for envID, conn := range p.conns {
				msg := WSMessage{Type: MsgPing}
				// Stamp before the write, not after: the round trip being
				// measured starts when the frame leaves, and a slow WriteJSON
				// is itself part of the latency the health derivation cares
				// about.
				sentAt := time.Now()
				if err := conn.WriteJSON(msg); err != nil {
					p.logger.Warn("agent ping failed", "env", envID, "error", err)
					continue
				}
				conn.MarkPing(sentAt)
			}
			p.mu.RUnlock()
		}
	}
}

// closeConn closes a superseded or rejected agent socket; the peer is already being dropped, so a failure is only logged.
func (p *AgentPool) closeConn(c *websocket.Conn, envID string) {
	if err := c.Close(); err != nil {
		p.logger.Debug("closing agent websocket failed", "env", envID, "error", err)
	}
}
