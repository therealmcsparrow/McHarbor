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
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"

	"github.com/therealmcsparrow/mcharbor/core/agent"
)

// ClientPool manages Docker SDK clients per environment.
type ClientPool struct {
	mu        sync.RWMutex
	clients   map[string]*client.Client
	db        *sql.DB
	agentPool *agent.AgentPool
	logger    *slog.Logger
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

// Get returns a Docker client for the given environment ID.
// If envID is empty, returns the default/local client.
func (p *ClientPool) Get(envID string) (*client.Client, error) {
	// Resolve empty/default to actual environment ID so that
	// tryAgentClient receives the real ID (not the literal "default").
	if envID == "" || envID == "default" {
		if resolved, err := p.resolveDefaultEnvID(); err == nil {
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
		c, err := p.tryAgentClient(envID)
		if err == nil && c != nil {
			p.clients[envID] = c
			p.logger.Info("docker client created via agent", "env", envID)
			return c, nil
		}
		if err != nil {
			p.logger.Debug("tryAgentClient failed", "env", envID, "error", err)
		}
	}

	conn, err := p.resolveConnection(envID)
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

// resolveDefaultEnvID returns the actual environment ID for the default environment.
func (p *ClientPool) resolveDefaultEnvID() (string, error) {
	var id string
	err := p.db.QueryRow("SELECT id FROM environments WHERE is_default = 1 LIMIT 1").Scan(&id)
	if err == nil {
		return id, nil
	}
	err = p.db.QueryRow("SELECT id FROM environments ORDER BY created_at ASC LIMIT 1").Scan(&id)
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
		c.Close()
		delete(p.clients, envID)
	}
}

// Ping checks if a Docker client is reachable.
func (p *ClientPool) Ping(ctx context.Context, envID string) error {
	c, err := p.Get(envID)
	if err != nil {
		return err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err = c.Ping(pingCtx)
	return err
}

// IsAgentEnv returns true if the environment uses agent connection type.
func (p *ClientPool) IsAgentEnv(envID string) bool {
	if envID == "" || envID == "default" {
		return false
	}
	var connType sql.NullString
	err := p.db.QueryRow("SELECT connection_type FROM environments WHERE id = ?", envID).Scan(&connType)
	if err != nil {
		return false
	}
	return connType.String == "agent"
}

// DockerHost returns the Docker host URL for an environment (e.g., "unix:///var/run/docker.sock").
// Returns an error for agent-type environments (use IsAgentEnv to check first).
func (p *ClientPool) DockerHost(envID string) (string, error) {
	if envID == "" || envID == "default" {
		if resolved, err := p.resolveDefaultEnvID(); err == nil {
			envID = resolved
		}
	}
	conn, err := p.resolveConnection(envID)
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
		c.Close()
		delete(p.clients, id)
	}
}

// AgentExec runs a command inside a container via the agent exec protocol.
// Used for agent environments where ContainerExecAttach can't work through
// the agent transport (Docker SDK's postHijacked bypasses RoundTrip).
func (p *ClientPool) AgentExec(ctx context.Context, envID, containerID string, cmd []string) ([]byte, error) {
	cli, err := p.Get(envID)
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}

	// Create exec on remote Docker via agent transport (standard HTTP, works through RoundTrip)
	execResp, err := cli.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true, // Matches agent's rawExecAttach TTY setting
	})
	if err != nil {
		return nil, fmt.Errorf("exec create: %w", err)
	}

	if p.agentPool == nil {
		return nil, fmt.Errorf("agent pool not initialized")
	}
	agentConn, ok := p.agentPool.Get(envID)
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
func (p *ClientPool) tryAgentClient(envID string) (*client.Client, error) {
	// Check connection type in DB
	var connType sql.NullString
	err := p.db.QueryRow("SELECT connection_type FROM environments WHERE id = ?", envID).Scan(&connType)
	if err != nil {
		p.logger.Warn("tryAgentClient: DB query failed", "env", envID, "error", err)
		return nil, fmt.Errorf("not an agent environment")
	}
	if connType.String != "agent" {
		return nil, fmt.Errorf("not an agent environment (type=%s)", connType.String)
	}

	agentConn, ok := p.agentPool.Get(envID)
	if !ok {
		p.logger.Warn("tryAgentClient: agent not in pool", "env", envID, "poolConnected", p.agentPool.IsConnected(envID))
		return nil, fmt.Errorf("agent not connected for env %s", envID)
	}
	p.logger.Debug("tryAgentClient: agent found in pool", "env", envID)

	httpClient := &http.Client{Transport: agentConn.Transport}
	c, err := client.NewClientWithOpts(
		client.WithHTTPClient(httpClient),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("creating agent Docker client: %w", err)
	}
	return c, nil
}
