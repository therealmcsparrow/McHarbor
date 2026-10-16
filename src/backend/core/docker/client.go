// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package docker

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/nodelink"
)

// CallTimeout bounds a single Docker SDK call.
//
// One constant rather than a hundred copies of `30 * time.Second`: the number
// is a policy about how long a daemon may take to answer one request — mostly
// agent-transport latency — and tuning it should be one edit, not a sweep.
// It does not belong on a stream (events, logs, an image export): those live
// as long as their caller does.
const CallTimeout = 30 * time.Second

// ClientPool manages Docker SDK clients per environment.
type ClientPool struct {
	mu        sync.RWMutex
	clients   map[string]*client.Client
	db        *sql.DB
	agentPool *agent.AgentPool
	nodeLink  *nodelink.Link
	logger    *slog.Logger
}

// SetNodeLink gives the pool the node-to-node link, so an agent held by another node is
// reachable from here. Injected rather than taken in NewClientPool because the link
// needs the database and the agent pool, which this pool is built alongside.
func (p *ClientPool) SetNodeLink(link *nodelink.Link) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.nodeLink = link
}

// NewClientPool creates a new Docker client pool.
func NewClientPool(db *sql.DB, agentPool *agent.AgentPool, logger *slog.Logger) *ClientPool {
	return &ClientPool{
		clients:   make(map[string]*client.Client),
		db:        db,
		agentPool: agentPool,
		logger:    logger,
	}
}

// Get returns a Docker client for the given environment ID; an empty ID returns the default/local client.
func (p *ClientPool) Get(envID string) (*client.Client, error) {
	return p.get(context.Background(), envID)
}

// GetContext is Get bound to the caller's context, so a cancelled request stops the environment lookup.
func (p *ClientPool) GetContext(ctx context.Context, envID string) (*client.Client, error) {
	return p.get(ctx, envID)
}

// get is Get with the caller's context threaded into the environment lookups so a cancelled request stops them.
func (p *ClientPool) get(ctx context.Context, envID string) (*client.Client, error) {
	// Resolve empty/default to the real environment ID so that
	// tryAgentClient receives it (not the literal "default").
	if envID == "" || envID == "default" {
		if resolved, err := p.resolveEnvironmentID(ctx, envID); err == nil {
			envID = resolved
		}
		// If no env found, envID stays empty — resolveConnection will auto-detect local socket
	}

	p.mu.RLock()
	if c, ok := p.clients[envID]; ok {
		p.mu.RUnlock()
		return c, nil
	}
	p.mu.RUnlock()

	// Create new client
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock
	if c, ok := p.clients[envID]; ok {
		return c, nil
	}

	// Check if this is an agent-connected environment
	if p.agentPool != nil {
		c, err := p.tryAgentClient(ctx, envID)
		if err == nil && c != nil {
			p.clients[envID] = c
			p.logger.Info("docker client created via agent", "env", envID)
			return c, nil
		}
		if err != nil {
			p.logger.Debug("tryAgentClient failed", "env", envID, "error", err)
		}
	}

	conn, err := p.resolveConnection(ctx, envID)
	if err != nil {
		return nil, err
	}

	c, err := conn.createClient()
	if err != nil {
		return nil, fmt.Errorf("creating Docker client for env %s: %w", envID, err)
	}

	p.clients[envID] = c
	p.logger.Info("docker client created", "env", envID, "host", conn.Host)
	return c, nil
}

// SetClient points envID at a client built elsewhere, bypassing connection
// resolution; it is the seam the fleet fixture uses to put an in-process daemon behind an environment.
func (p *ClientPool) SetClient(envID string, c *client.Client) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.clients[envID] = c
}

// ResolveEnvironmentID resolves the default environment alias to its stored ID.
func (p *ClientPool) ResolveEnvironmentID(envID string) (string, error) {
	return p.resolveEnvironmentID(context.Background(), envID)
}

// ResolveEnvironmentIDContext is ResolveEnvironmentID bound to the caller's context, so a cancelled request stops the lookup.
func (p *ClientPool) ResolveEnvironmentIDContext(ctx context.Context, envID string) (string, error) {
	return p.resolveEnvironmentID(ctx, envID)
}

// resolveEnvironmentID is ResolveEnvironmentID with the caller's context for the lookup.
func (p *ClientPool) resolveEnvironmentID(ctx context.Context, envID string) (string, error) {
	if envID == "" || envID == "default" {
		return p.resolveDefaultEnvID(ctx)
	}
	return envID, nil
}

// resolveDefaultEnvID returns the actual environment ID for the default environment.
func (p *ClientPool) resolveDefaultEnvID(ctx context.Context) (string, error) {
	var id string
	err := p.db.QueryRowContext(ctx, "SELECT id FROM environments WHERE is_default = 1 LIMIT 1").Scan(&id)
	if err == nil {
		return id, nil
	}
	err = p.db.QueryRowContext(ctx, "SELECT id FROM environments ORDER BY created_at ASC LIMIT 1").Scan(&id)
	if err == nil {
		return id, nil
	}
	return "", fmt.Errorf("no environments found")
}

// Remove removes a client from the pool and closes it.
func (p *ClientPool) Remove(envID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if c, ok := p.clients[envID]; ok {
		if err := c.Close(); err != nil {
			p.logger.Warn("closing docker client", "env", envID, "error", err)
		}
		delete(p.clients, envID)
	}
}

// Ping checks if a Docker client is reachable.
func (p *ClientPool) Ping(ctx context.Context, envID string) error {
	c, err := p.get(ctx, envID)
	if err != nil {
		return err
	}
	pingCtx, cancel := context.WithTimeout(ctx, CallTimeout)
	defer cancel()
	_, err = c.Ping(pingCtx, client.PingOptions{})
	return err
}

// IsAgentEnv returns true if the environment uses agent connection type.
func (p *ClientPool) IsAgentEnv(envID string) bool {
	return p.IsAgentEnvContext(context.Background(), envID)
}

// IsAgentEnvContext is IsAgentEnv bound to the caller's context, so a cancelled request stops the connection-type lookup.
func (p *ClientPool) IsAgentEnvContext(ctx context.Context, envID string) bool {
	if envID == "" || envID == "default" {
		resolved, err := p.resolveEnvironmentID(ctx, envID)
		if err != nil {
			return false
		}
		envID = resolved
	}
	var connType sql.NullString
	err := p.db.QueryRowContext(ctx, "SELECT connection_type FROM environments WHERE id = ?", envID).Scan(&connType)
	if err != nil {
		return false
	}
	return connType.String == "agent"
}

// AgentVersion returns the connected agent version for an environment.
func (p *ClientPool) AgentVersion(envID string) (string, bool) {
	return p.AgentVersionContext(context.Background(), envID)
}

// AgentVersionContext is AgentVersion bound to the caller's context, so a cancelled request stops the environment lookup.
func (p *ClientPool) AgentVersionContext(ctx context.Context, envID string) (string, bool) {
	if p.agentPool == nil {
		return "", false
	}
	if envID == "" || envID == "default" {
		resolved, err := p.resolveEnvironmentID(ctx, envID)
		if err != nil {
			return "", false
		}
		envID = resolved
	}
	return p.agentPool.Version(envID)
}

// AgentConnection returns the connected agent for an environment: the local connection,
// or in active/active one relayed through the node that holds it, so every agent message
// (compute, console, Compose, prune, transfers, backups) works from any node.
func (p *ClientPool) AgentConnection(envID string) (*agent.AgentConnection, bool) {
	return p.AgentConnectionContext(context.Background(), envID)
}

// AgentConnectionContext is AgentConnection bound to the caller's context, so a cancelled request stops the environment lookup.
func (p *ClientPool) AgentConnectionContext(ctx context.Context, envID string) (*agent.AgentConnection, bool) {
	if p.agentPool == nil {
		return nil, false
	}
	if envID == "" || envID == "default" {
		resolved, err := p.resolveEnvironmentID(ctx, envID)
		if err != nil {
			return nil, false
		}
		envID = resolved
	}
	return p.agentPool.Resolve(envID)
}

// AgentAtLeast reports whether the connected agent meets a minimum version.
func (p *ClientPool) AgentAtLeast(envID, minimum string) bool {
	return p.AgentAtLeastContext(context.Background(), envID, minimum)
}

// AgentAtLeastContext is AgentAtLeast bound to the caller's context, so a cancelled request stops the environment lookup.
func (p *ClientPool) AgentAtLeastContext(ctx context.Context, envID, minimum string) bool {
	version, ok := p.AgentVersionContext(ctx, envID)
	if !ok {
		return false
	}
	return compareAgentVersion(version, minimum) >= 0
}

func compareAgentVersion(version, minimum string) int {
	versionParts := parseAgentVersion(version)
	minimumParts := parseAgentVersion(minimum)
	for i := 0; i < 3; i++ {
		if versionParts[i] > minimumParts[i] {
			return 1
		}
		if versionParts[i] < minimumParts[i] {
			return -1
		}
	}
	return 0
}

func parseAgentVersion(version string) [3]int {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	parts := strings.Split(version, ".")
	var result [3]int
	for i := 0; i < len(parts) && i < len(result); i++ {
		n, err := strconv.Atoi(parts[i])
		if err == nil {
			result[i] = n
		}
	}
	return result
}

// DockerHost returns the Docker host URL for an environment (e.g., "unix:///var/run/docker.sock").
// Returns an error for agent-type environments (use IsAgentEnv to check first).
func (p *ClientPool) DockerHost(envID string) (string, error) {
	return p.DockerHostContext(context.Background(), envID)
}

// DockerHostContext is DockerHost bound to the caller's context, so a cancelled request stops the connection lookup.
func (p *ClientPool) DockerHostContext(ctx context.Context, envID string) (string, error) {
	if envID == "" || envID == "default" {
		if resolved, err := p.resolveDefaultEnvID(ctx); err == nil {
			envID = resolved
		}
	}
	conn, err := p.resolveConnection(ctx, envID)
	if err != nil {
		return "", err
	}
	return conn.Host, nil
}

// Close closes all clients in the pool.
func (p *ClientPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for id, c := range p.clients {
		if err := c.Close(); err != nil {
			p.logger.Warn("closing docker client", "env", id, "error", err)
		}
		delete(p.clients, id)
	}
}

// AgentExec runs a command inside a container via the agent exec protocol.
// Used for agent environments where ContainerExecAttach can't work through
// the agent transport (Docker SDK's postHijacked bypasses RoundTrip).
func (p *ClientPool) AgentExec(ctx context.Context, envID, containerID string, cmd []string) ([]byte, error) {
	cli, err := p.get(ctx, envID)
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}

	// Create exec on remote Docker via agent transport (standard HTTP, works through RoundTrip)
	execResp, err := cli.ExecCreate(ctx, containerID, client.ExecCreateOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
		TTY:          true, // Matches agent's rawExecAttach TTY setting
	})
	if err != nil {
		return nil, fmt.Errorf("exec create: %w", err)
	}

	if p.agentPool == nil {
		return nil, fmt.Errorf("agent pool not initialized")
	}
	agentConn, ok := p.agentPool.Resolve(envID)
	if !ok {
		return nil, fmt.Errorf("agent not connected for env %s", envID)
	}

	sessionID := execResp.ID

	// Register session to receive exec output from the agent
	session := agentConn.Transport.StartExecSession(sessionID)
	defer agentConn.Transport.StopExecSession(sessionID)

	// Tell agent to start the exec attach
	startMsg := agent.WSMessage{
		Type: agent.MsgExecStart,
		ID:   sessionID,
		ExecStart: &agent.ExecStartPayload{
			ExecID: execResp.ID,
		},
	}
	if err := agentConn.WriteJSON(startMsg); err != nil {
		return nil, fmt.Errorf("sending exec start to agent: %w", err)
	}

	// Collect output until exec ends or context cancels
	var buf bytes.Buffer
	for {
		select {
		case <-ctx.Done():
			return buf.Bytes(), ctx.Err()
		case data, ok := <-session.OutputCh:
			if !ok {
				return buf.Bytes(), nil
			}
			buf.Write(data)
		case <-session.DoneCh:
			// Drain remaining output from channel
			for {
				select {
				case data, ok := <-session.OutputCh:
					if !ok {
						return buf.Bytes(), nil
					}
					buf.Write(data)
				default:
					return buf.Bytes(), nil
				}
			}
		}
	}
}

// tryAgentClient checks if the environment uses agent connection type and
// has a connected agent, then returns a Docker client using the agent transport.
func (p *ClientPool) tryAgentClient(ctx context.Context, envID string) (*client.Client, error) {
	// Check connection type in DB
	var connType sql.NullString
	err := p.db.QueryRowContext(ctx, "SELECT connection_type FROM environments WHERE id = ?", envID).Scan(&connType)
	if err != nil {
		p.logger.Warn("tryAgentClient: DB query failed", "env", envID, "error", err)
		return nil, fmt.Errorf("not an agent environment")
	}
	if connType.String != "agent" {
		return nil, fmt.Errorf("not an agent environment (type=%s)", connType.String)
	}

	agentConn, ok := p.agentPool.Get(envID)
	var roundTripper http.RoundTripper
	switch {
	case ok:
		p.logger.Debug("tryAgentClient: agent found in pool", "env", envID)
		// Through the link even when the agent is local: the routing transport
		// delegates to the local one, and resolving per request means a client
		// cached now keeps working after the agent moves to another node.
		roundTripper = agentConn.Transport
		if p.nodeLink != nil {
			roundTripper = p.nodeLink.Transport(envID)
		}
	case p.nodeLink != nil:
		owner, remote := p.nodeLink.RemoteOwner(ctx, envID)
		if !remote {
			p.logger.Warn("tryAgentClient: agent not in pool", "env", envID, "poolConnected", p.agentPool.IsConnected(envID))
			return nil, fmt.Errorf("agent not connected for env %s", envID)
		}
		p.logger.Info("docker client routed to the node holding the agent", "env", envID, "node", owner.NodeID)
		roundTripper = p.nodeLink.Transport(envID)
	default:
		p.logger.Warn("tryAgentClient: agent not in pool", "env", envID, "poolConnected", p.agentPool.IsConnected(envID))
		return nil, fmt.Errorf("agent not connected for env %s", envID)
	}

	httpClient := &http.Client{Transport: roundTripper}
	c, err := client.New(client.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("creating agent Docker client: %w", err)
	}
	return c, nil
}
