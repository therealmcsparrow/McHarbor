// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/quiesce"
)

// agentHealthInterval matches the container health collector's cadence and the
// default keepalive interval, so each tick lands roughly one ping apart.
const agentHealthInterval = 30 * time.Second

// HealthCollector persists one health and uptime sample per agent environment
// per tick, so the environment Overview tab can graph them.
//
// Note the deliberate inversion relative to containers.HealthCollector, which
// skips agent environments because every call it makes is a WebSocket round
// trip: this collector looks at agent environments ONLY, and costs no agent
// traffic at all. Everything it records - whether the socket is up, when the
// last pong arrived, the round-trip latency - is already in this process,
// stamped on the pooled connection by the keepalive loop. The tick is a read of
// local memory plus one INSERT.
//
// That is also why it must be a collector rather than an on-read sampler alone:
// an agent that drops at 02:00 is only interesting if something was watching,
// and nobody has the environment detail page open at 02:00.
type HealthCollector struct {
	db          *sql.DB
	enc         *encryption.Service
	agentPool   *coreagent.AgentPool
	logger      *slog.Logger
	done        chan struct{}
	coordinator healthCoordinator
	nodeID      string
	hasCoord    bool
}

// healthCoordinator is the subset of the cluster coordinator the collector
// needs. Declaring it locally keeps this module free of a cluster dependency.
type healthCoordinator interface {
	LeaderOf(ctx context.Context, name string) bool
	NodeID() string
}

// NewHealthCollector creates a collector that runs on every node.
func NewHealthCollector(database *sql.DB, enc *encryption.Service, pool *coreagent.AgentPool, logger *slog.Logger) *HealthCollector {
	return &HealthCollector{
		db:        database,
		enc:       enc,
		agentPool: pool,
		logger:    logger,
		done:      make(chan struct{}),
	}
}

// NewHealthCollectorWithCoordinator creates a collector that only samples on
// the elected HA leader. SQLite installs always report leader=true, so
// single-node behaviour is unchanged.
//
// Leader-gating matters more here than it looks: an agent holds its WebSocket
// to exactly one node, so a follower would sample every agent as disconnected
// and write a fabricated outage into the shared series.
func NewHealthCollectorWithCoordinator(database *sql.DB, enc *encryption.Service, pool *coreagent.AgentPool, logger *slog.Logger, coord healthCoordinator, nodeID string) *HealthCollector {
	c := NewHealthCollector(database, enc, pool, logger)
	c.coordinator = coord
	c.nodeID = nodeID
	c.hasCoord = coord != nil
	return c
}

// Start launches the background collection goroutine.
func (c *HealthCollector) Start() {
	go c.run()
	c.logger.Info("agent health collector started", "interval", agentHealthInterval.String())
}

// Stop signals the collector to shut down.
func (c *HealthCollector) Stop() {
	close(c.done)
}

func (c *HealthCollector) run() {
	ticker := time.NewTicker(agentHealthInterval)
	defer ticker.Stop()

	// Stagger the first sample past the point where agents reconnect after a
	// restart. Sampling immediately would record every agent as down for one
	// block on every deploy, which is an outage the operator did not have.
	select {
	case <-c.done:
		return
	case <-time.After(20 * time.Second):
	}
	c.tick(context.Background())

	pruneEvery := 0
	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			if !quiesce.Run(func() { c.tick(context.Background()) }) {
				continue
			}
			pruneEvery++
			// Roughly hourly at a 30s cadence.
			if pruneEvery >= 120 {
				pruneEvery = 0
				c.prune(context.Background())
			}
		}
	}
}

func (c *HealthCollector) tick(ctx context.Context) {
	if !c.leaderFor(ctx) {
		c.logger.Debug("agent health collector: not leader this tick; skipping", "node", c.nodeID)
		return
	}

	svc := NewService(c.db, c.enc, c.agentPool, "")
	pingSeconds := int(svc.pingInterval(ctx) / time.Second)
	bucket := currentAgentSampleBucket()

	for _, env := range c.agentEnvironments(ctx) {
		health, latency := svc.health(ctx, env.id, env.everConnected)
		sample := HealthSample{
			SampledAt: bucket,
			Connected: c.agentPool.IsConnected(env.id),
			Health:    health,
			LatencyMs: latency,
		}
		if err := recordAgentSample(ctx, c.db, env.id, sample, env.version, pingSeconds); err != nil {
			c.logger.Error("agent health collector: sample failed", "env", env.id, "error", err)
		}
	}
}

func (c *HealthCollector) leaderFor(ctx context.Context) bool {
	if !c.hasCoord {
		return true
	}
	return c.coordinator.LeaderOf(ctx, "agent-health-collector")
}

// agentEnv is one row of the sweep: the environment to sample plus the two
// facts the derivation needs that live only in the database.
type agentEnv struct {
	id string
	// everConnected separates an agent that has gone down from one that has
	// never been installed, which read as unhealthy and unknown respectively.
	everConnected bool
	version       string
}

func (c *HealthCollector) agentEnvironments(ctx context.Context) []agentEnv {
	rows, err := c.db.QueryContext(ctx, `
		SELECT id, agent_last_seen, agent_version
		FROM environments
		WHERE connection_type = 'agent' AND is_active = 1
		LIMIT 1000
	`)
	if err != nil {
		c.logger.Error("agent health collector: failed to query environments", "error", err)
		return nil
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()

	var envs []agentEnv
	for rows.Next() {
		var env agentEnv
		var lastSeen, version sql.NullString
		if err := rows.Scan(&env.id, &lastSeen, &version); err != nil {
			continue
		}
		env.everConnected = lastSeen.Valid && lastSeen.String != ""
		env.version = version.String
		envs = append(envs, env)
	}
	if err := rows.Err(); err != nil {
		c.logger.Error("agent health collector: failed to read environments", "error", err)
	}
	return envs
}

func (c *HealthCollector) prune(ctx context.Context) {
	deleted, err := pruneAgentHealth(ctx, c.db)
	if err != nil {
		c.logger.Error("agent health collector: prune failed", "error", err)
		return
	}
	if deleted > 0 {
		c.logger.Debug("agent health collector: pruned history rows", "rows", deleted)
	}
}
