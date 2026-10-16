// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cluster

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/db"
	appversion "github.com/therealmcsparrow/mcharbor/core/version"
)

// Instance states. Draining is still serving what it has but tells load balancers
// and the HA controls to stop sending it new work.
const (
	StateRunning  = "running"
	StateDraining = "draining"
	StateStopped  = "stopped"
)

// HA roles. Peer is every node of a deployment that has not chosen active/passive.
const (
	RolePeer    = "peer"
	RoleActive  = "active"
	RoleStandby = "standby"
)

const (
	heartbeatEvery = 10 * time.Second
	// aliveWithin is three missed beats: long enough to ride out a GC pause or a slow
	// database, short enough that a dead node disappears from the topology quickly.
	aliveWithin     = 3 * heartbeatEvery
	pruneStoppedAge = 24 * time.Hour
)

// Instance is one node as the registry sees it.
type Instance struct {
	ID           string `json:"id"`
	Hostname     string `json:"hostname"`
	Version      string `json:"version"`
	Build        string `json:"build"`
	HARole       string `json:"haRole"`
	State        string `json:"state"`
	AdvertiseURL string `json:"advertiseUrl"`
	Agents       int    `json:"agents"`
	StartedAt    string `json:"startedAt"`
	HeartbeatAt  string `json:"heartbeatAt"`
	Alive        bool   `json:"alive"`
	Self         bool   `json:"self"`
}

// Registry writes this node's heartbeat into cluster_instances so every node can see
// the topology, a dead node shows as not alive, and two actives are detectable.
type Registry struct {
	database  *sql.DB
	nodeID    string
	hostname  string
	advertise string
	started   time.Time
	agents    func() int
	now       func() time.Time
	logger    *slog.Logger

	role  atomic.Value
	state atomic.Value

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewRegistry builds the registry for this process; agents reports how many remote
// agents are connected here, which is what cross-node routing will need to know.
func NewRegistry(database *sql.DB, nodeID, advertiseURL string, agents func() int, logger *slog.Logger) *Registry {
	host, err := os.Hostname()
	if err != nil {
		host = ""
	}
	r := &Registry{database: database, nodeID: nodeID, hostname: host, advertise: advertiseURL,
		started: time.Now().UTC(), agents: agents, now: time.Now, logger: logger}
	r.role.Store(RolePeer)
	r.state.Store(StateRunning)
	return r
}

// NodeID is the identity this registry beats under.
func (r *Registry) NodeID() string { return r.nodeID }

// Role is this node's current HA role.
func (r *Registry) Role() string { return r.role.Load().(string) }

// SetRole records a role change; the next heartbeat publishes it.
func (r *Registry) SetRole(role string) { r.role.Store(role) }

// State is this node's lifecycle state.
func (r *Registry) State() string { return r.state.Load().(string) }

// SetState records a state change and publishes it immediately, so draining is visible
// to the other nodes before the load balancer's next probe.
func (r *Registry) SetState(ctx context.Context, state string) {
	r.state.Store(state)
	if err := r.Beat(ctx); err != nil {
		r.logger.Warn("cluster: publishing state change failed", "state", state, "error", err)
	}
}

// Start beats immediately and then on a fixed interval until Stop.
func (r *Registry) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	if err := r.Beat(ctx); err != nil {
		r.logger.Warn("cluster: first heartbeat failed", "error", err)
	}
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		t := time.NewTicker(heartbeatEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := r.Beat(ctx); err != nil {
					r.logger.Warn("cluster: heartbeat failed", "error", err)
				}
			}
		}
	}()
}

// Stop ends the heartbeat and records the node as stopped, so a clean shutdown is not
// mistaken for a crash in the failover timeline.
func (r *Registry) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
	r.wg.Wait()
	r.state.Store(StateStopped)
	ctx, cancel := context.WithTimeout(context.Background(), clusterCallTimeout)
	defer cancel()
	if err := r.Beat(ctx); err != nil {
		r.logger.Warn("cluster: final heartbeat failed", "error", err)
	}
}

// Beat upserts this node's row. UPDATE-then-INSERT works unchanged on all four backends.
func (r *Registry) Beat(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, clusterCallTimeout)
	defer cancel()
	now := r.now().UTC().Format(time.RFC3339)
	agents := 0
	if r.agents != nil {
		agents = r.agents()
	}
	res, err := r.database.ExecContext(ctx, `UPDATE cluster_instances SET hostname = ?, version = ?, build = ?, ha_role = ?,
		state = ?, advertise_url = ?, agents = ?, started_at = ?, heartbeat_at = ?, updated_at = ? WHERE id = ?`,
		r.hostname, appversion.Current(), appversion.Build(), r.Role(), r.State(), r.advertise, agents,
		r.started.Format(time.RFC3339), now, now, r.nodeID)
	if err != nil {
		return fmt.Errorf("cluster: heartbeat update: %w", err)
	}
	if db.RowsAffected(res) > 0 {
		return nil
	}
	if _, err := r.database.ExecContext(ctx, `INSERT INTO cluster_instances (id, hostname, version, build, ha_role, state,
		advertise_url, agents, started_at, heartbeat_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.nodeID, r.hostname, appversion.Current(), appversion.Build(), r.Role(), r.State(), r.advertise, agents,
		r.started.Format(time.RFC3339), now, now, now); err != nil {
		return fmt.Errorf("cluster: heartbeat insert: %w", err)
	}
	return nil
}

// List returns every known node with Alive computed from its heartbeat age; long
// stopped nodes are pruned so the topology does not fill with old container ids.
func (r *Registry) List(ctx context.Context) ([]Instance, error) {
	ctx, cancel := context.WithTimeout(ctx, clusterCallTimeout)
	defer cancel()
	now := r.now().UTC()
	if _, err := r.database.ExecContext(ctx, `DELETE FROM cluster_instances WHERE state = ? AND heartbeat_at < ?`,
		StateStopped, now.Add(-pruneStoppedAge).Format(time.RFC3339)); err != nil {
		r.logger.Warn("cluster: pruning stopped instances failed", "error", err)
	}
	return r.list(ctx, now)
}

// ListLive returns every known node like List but never prunes, for a hot path such as
// the node link's peer source, which should not turn every read into a DELETE.
func (r *Registry) ListLive(ctx context.Context) ([]Instance, error) {
	ctx, cancel := context.WithTimeout(ctx, clusterCallTimeout)
	defer cancel()
	return r.list(ctx, r.now().UTC())
}

func (r *Registry) list(ctx context.Context, now time.Time) ([]Instance, error) {
	rows, err := r.database.QueryContext(ctx, `SELECT id, hostname, version, build, ha_role, state, advertise_url, agents,
		started_at, heartbeat_at FROM cluster_instances ORDER BY id LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("cluster: listing instances: %w", err)
	}
	defer func() { db.CloseRows(rows) }()
	out := []Instance{}
	for rows.Next() {
		var in Instance
		if err := rows.Scan(&in.ID, &in.Hostname, &in.Version, &in.Build, &in.HARole, &in.State, &in.AdvertiseURL,
			&in.Agents, &in.StartedAt, &in.HeartbeatAt); err != nil {
			return nil, fmt.Errorf("cluster: scanning instance: %w", err)
		}
		if beat, err := time.Parse(time.RFC3339, in.HeartbeatAt); err == nil {
			in.Alive = in.State != StateStopped && now.Sub(beat) <= aliveWithin
		}
		in.Self = in.ID == r.nodeID
		out = append(out, in)
	}
	return out, rows.Err()
}

// ActiveConflict reports the alive nodes claiming the active role; more than one is a
// split brain, which the UI surfaces and the controls refuse to act through.
func ActiveConflict(instances []Instance) []string {
	var ids []string
	for _, in := range instances {
		if in.Alive && in.HARole == RoleActive {
			ids = append(ids, in.ID)
		}
	}
	if len(ids) < 2 {
		return nil
	}
	return ids
}
