// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
)

// AgentConnection represents a connected remote agent.
type AgentConnection struct {
	EnvID     string
	Hostname  string
	OS        string
	Arch      string
	Version   string
	DockerVer string
	Conn      *websocket.Conn
	Transport *AgentTransport
	mu        sync.Mutex
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
type AgentPool struct {
	mu     sync.RWMutex
	conns  map[string]*AgentConnection
	logger *slog.Logger
}

// NewAgentPool creates a new agent pool.
func NewAgentPool(logger *slog.Logger) *AgentPool {
	return &AgentPool{
		conns:  make(map[string]*AgentConnection),
		logger: logger,
	}
}

// Register adds an agent connection to the pool.
func (p *AgentPool) Register(envID string, conn *AgentConnection) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Close existing connection if any
	if existing, ok := p.conns[envID]; ok {
		existing.Conn.Close()
	}
	p.conns[envID] = conn
	p.logger.Info("agent registered", "env", envID, "hostname", conn.Hostname)
}

// Remove removes an agent connection from the pool.
func (p *AgentPool) Remove(envID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
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

// IsConnected checks if an agent is connected for the given environment.
func (p *AgentPool) IsConnected(envID string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	_, ok := p.conns[envID]
	return ok
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
			DockerVer: conn.DockerVer,
		})
	}
	return result
}

// StartPingLoop sends pings to all connected agents at the configured interval.
func (p *AgentPool) StartPingLoop(ctx context.Context, db *sql.DB) {
	agentSettings := coreSettings.ReadAgentSettings(db)
	currentInterval := time.Duration(agentSettings.PingInterval) * time.Second
	ticker := time.NewTicker(currentInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Re-read settings and adjust ticker if interval changed
			agentSettings = coreSettings.ReadAgentSettings(db)
			newInterval := time.Duration(agentSettings.PingInterval) * time.Second
			if newInterval != currentInterval {
				ticker.Reset(newInterval)
				currentInterval = newInterval
				p.logger.Info("agent ping interval updated", "interval", newInterval)
			}

			p.mu.RLock()
			for envID, conn := range p.conns {
				msg := WSMessage{Type: MsgPing}
				if err := conn.WriteJSON(msg); err != nil {
					p.logger.Warn("agent ping failed", "env", envID, "error", err)
				}
			}
			p.mu.RUnlock()
		}
	}
}
