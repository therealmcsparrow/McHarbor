// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"context"
	"database/sql"
	"log/slog"
	"slices"
	"sync"
	"time"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/gpu"
	"github.com/therealmcsparrow/mcharbor/core/quiesce"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
	"github.com/therealmcsparrow/mcharbor/core/statshub"
	"github.com/therealmcsparrow/mcharbor/core/telemetry"
)

// Collector periodically aggregates container stats into the host_metrics table.
type Collector struct {
	db          *sql.DB
	dockerPool  *docker.ClientPool
	logger      *slog.Logger
	done        chan struct{}
	coordinator metricsCoordinator
	nodeID      string
	hasCoord    bool

	// gpu reads the local host's GPUs. Nil-safe: gpuUtilizationSample
	// returns ok=false when nvidia-smi is absent, and the sample is
	// then written as NULL rather than as a zero.
	gpu *gpu.Reader

	// hub is the shared live stats cache. Nil means every tick pulls on its own,
	// which is what the collector did before the hub existed.
	hub *statshub.Hub

	// pending tracks push-capable agents that have been subscribed but are not
	// pushing yet, so one that never delivers falls back to pulling instead of
	// leaving the environment without history.
	subMu   sync.Mutex
	pending map[string]*pendingPush

	// lastDiskAt gates the hourly writable-layer sweep. Zero value means the
	// first tick after startup takes a disk sample, so a freshly started
	// instance does not wait an hour for its first size reading.
	lastDiskAt time.Time

	// lastCapacityAt gates the hourly filesystem-capacity sweep, on the same
	// zero-value-samples-immediately rule as lastDiskAt.
	lastCapacityAt time.Time

	// capacityMu guards the two dependencies the capacity sweep needs but boot
	// cannot supply before Start: the agent pool and the storage-destination
	// reader, which needs the plugin runtime.
	capacityMu   sync.RWMutex
	storageUsage StorageUsageReader
	agentPool    *coreagent.AgentPool

	// optimizeAt is when the leader last refreshed SQLite's planner statistics;
	// optimize is that refresh, swappable so the daily schedule can be tested.
	optimizeAt time.Time
	optimize   func(ctx context.Context) error

	// hostReader overrides the live host reading; nil means Service.ReadHost.
	hostReader HostReader
}

// metricsCoordinator is the subset of the cluster coordinator needed by
// the collector. Keeping this local interface avoids coupling the metrics
// module to the concrete coordinator package.
type metricsCoordinator interface {
	LeaderOf(ctx context.Context, name string) bool
	NodeID() string
}

// NewCollector creates a new metrics collector.
func NewCollector(db *sql.DB, pool *docker.ClientPool, logger *slog.Logger) *Collector {
	return &Collector{
		db:         db,
		dockerPool: pool,
		logger:     logger,
		done:       make(chan struct{}),
		gpu:        gpu.NewReader(),

		pending: make(map[string]*pendingPush),
	}
}

// SetStatsHub routes the collector's reads through hub, so a sweep the
// containers page just paid for is not paid for again, and agents that push
// are written by their own batches rather than pulled.
func (c *Collector) SetStatsHub(hub *statshub.Hub) {
	c.hub = hub
}

// NewCollectorWithCoordinator creates a collector that only runs each scrape
// on the elected HA leader. SQLite installs still report leader=true, so
// single-node behavior remains unchanged.
func NewCollectorWithCoordinator(db *sql.DB, pool *docker.ClientPool, logger *slog.Logger, coord metricsCoordinator, nodeID string) *Collector {
	c := NewCollector(db, pool, logger)
	c.coordinator = coord
	c.nodeID = nodeID
	c.hasCoord = coord != nil
	return c
}

// Start launches the background collection goroutine.
func (c *Collector) Start() {
	go c.run()
	c.logger.Info("metrics collector started", "interval", "30s")
}

// Stop signals the collector to shut down and waits for it to finish.
func (c *Collector) Stop() {
	close(c.done)
}

func (c *Collector) run() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Collect once on startup after a short delay
	time.Sleep(5 * time.Second)
	c.tick(context.Background())

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			quiesce.Run(func() { c.tick(context.Background()) })
		}
	}
}

func (c *Collector) tick(ctx context.Context) {
	if !c.leaderFor(ctx) {
		c.logger.Debug("metrics collector: not leader this tick; skipping", "node", c.nodeID)
		return
	}
	// Read once per tick rather than per environment: retention is global, and
	// an operator changing it mid-tick should not split one sweep across two
	// policies.
	started := time.Now()
	defer func() { telemetry.CollectorSweep(time.Since(started)) }()
	retention := coreSettings.ReadRetentionSettingsContext(ctx, c.db)
	c.collect(ctx, retention)
	if now := time.Now().UTC(); now.Sub(c.lastCapacityAt) >= capacityInterval {
		c.collectCapacity(ctx, now)
		c.lastCapacityAt = now
	}
	c.compact(ctx, retention.MetricsRawHours)
	c.prune(ctx, retention)
	c.pruneCapacity(ctx, retention)
	c.maybeOptimize(ctx, time.Now().UTC())
}

func (c *Collector) leaderFor(ctx context.Context) bool {
	if !c.hasCoord {
		return true
	}
	return c.coordinator.LeaderOf(ctx, "metrics-collector")
}

// getDefaultEnvID returns the ID of the default environment, or "" when the
// install has none.
//
// It used to answer "local" instead, which is a row that no longer exists:
// McHarbor stopped inventing a Local environment, and every sample attributed
// to that id failed the host_metrics foreign key on every tick. An empty id is
// the honest answer and the callers skip the sample.
func (c *Collector) getDefaultEnvID(ctx context.Context) string {
	var id string
	if err := c.db.QueryRowContext(ctx, "SELECT id FROM environments WHERE is_default = 1 LIMIT 1").Scan(&id); err != nil {
		return ""
	}
	return id
}

// collect fetches stats for all active environments and inserts aggregated metrics.
func (c *Collector) collect(ctx context.Context, retention coreSettings.RetentionSettings) {
	// One timestamp for the whole tick, so every container of this sweep lands
	// in the same column of the series and the host aggregate lines up with
	// the per-container rows it was summed from.
	sampledAt := time.Now().UTC()
	diskTick := retention.MetricsDiskEnabled && sampledAt.Sub(c.lastDiskAt) >= metricsDiskInterval

	// No LIMIT: this is the set of environments to collect from, not a page
	// of them. A cap would silently stop collecting for whatever fell past it.
	rows, err := c.db.QueryContext(ctx, `
		SELECT id
		FROM environments
		WHERE is_active = 1
		  AND collect_container_metrics_enabled = 1
	`)
	if err != nil {
		c.logger.Error("metrics collector: failed to query environments", "error", err)
		return
	}
	defer closeRows(rows)

	var envIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		envIDs = append(envIDs, id)
	}
	if err := rows.Err(); err != nil {
		c.logger.Error("metrics collector: failed to read environments", "error", err)
	}

	// Also collect the default environment through the pool's own resolution,
	// but only when there is one and the query above did not already list it —
	// an install whose environments are all agents has no default at all, and
	// sampling it anyway wrote rows for an environment id that does not exist.
	if defaultID := c.getDefaultEnvID(ctx); defaultID != "" &&
		!slices.Contains(envIDs, defaultID) &&
		c.isMetricsCollectionEnabled(ctx, defaultID) {
		envIDs = append(envIDs, "")
	}

	svc := NewService(c.dockerPool)
	agentSettings := coreSettings.ReadAgentSettingsContext(ctx, c.db)

	// Environments are sampled concurrently (bounded) so one slow or
	// unreachable host — an agent environment can take up to 60s to answer
	// — does not delay every other environment's sample past the tick.
	var wg sync.WaitGroup
	sem := make(chan struct{}, collectEnvConcurrency)
	for _, envID := range envIDs {
		isAgent := envID != "" && c.dockerPool.IsAgentEnvContext(ctx, envID)
		// Keep the global agent metrics setting as an emergency off switch.
		if isAgent && !agentSettings.MetricsEnabled {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(envID string, isAgent bool) {
			defer wg.Done()
			defer func() { <-sem }()
			c.collectEnvironment(ctx, svc, envID, isAgent, sampledAt, diskTick)
		}(envID, isAgent)
	}
	wg.Wait()

	if diskTick {
		c.lastDiskAt = sampledAt
	}
}

// collectEnvConcurrency bounds how many environments are sampled at once.
const collectEnvConcurrency = 4

// collectEnvironment samples one environment and writes its per-container
// and host rows. Safe to run concurrently per environment: it only touches
// *sql.DB (goroutine-safe) and the logger.
func (c *Collector) collectEnvironment(ctx context.Context, svc *Service, envID string, isAgent bool, sampledAt time.Time, diskTick bool) {
	timeout := 20 * time.Second
	if isAgent {
		timeout = 60 * time.Second
	}
	if isAgent && c.pushHandles(ctx, envID) {
		return
	}
	statsCtx, cancel := context.WithTimeout(ctx, timeout)
	stats, err := c.sample(statsCtx, svc, envID)
	cancel()
	if err != nil {
		c.logger.Debug("metrics collector: skipping environment", "env", envID, "error", err)
		return
	}

	// Use the actual default env ID for the local environment in DB
	dbEnvID := envID
	if dbEnvID == "" {
		dbEnvID = c.getDefaultEnvID(ctx)
	}
	if dbEnvID == "" {
		c.logger.Debug("metrics collector: no default environment to attribute the sample to")
		return
	}

	// The host row's CPU and memory are the host's own reading, not a sum of
	// its containers: a host that runs no containers (a KVM hypervisor, say)
	// would otherwise never get a row and its card and graphs stay empty.
	host, hostOK := c.readHost(ctx, envID)
	if len(stats) == 0 && !hostOK {
		return
	}

	if len(stats) > 0 {
		// Sizes are skipped for agent environments regardless of the hourly
		// gate: Size:true makes the daemon walk every container's diff
		// directory, and that result would then have to cross the agent
		// WebSocket.
		var sizes map[string]containerSize
		if diskTick && !isAgent {
			sizeCtx, sizeCancel := context.WithTimeout(ctx, 60*time.Second)
			collected, sizeErr := svc.ContainerSizes(sizeCtx, envID)
			sizeCancel()
			if sizeErr != nil {
				c.logger.Debug("metrics collector: container sizes unavailable", "env", envID, "error", sizeErr)
			} else {
				sizes = collected
			}
		}

		// Tier 1 for the per-container series. Written before the aggregate so
		// a failure here is visible in logs while the host row still lands.
		if err := c.recordContainerSamples(ctx, dbEnvID, sampledAt, stats, sizes); err != nil {
			c.logger.Error("metrics collector: failed to record container samples", "error", err, "env", dbEnvID)
		}
	}

	agg := mergeHost(aggregateHost(stats), host, hostOK)

	// NULL when this host reports no GPU, so the history reader can
	// tell "no telemetry" from "idle" and omit the series rather
	// than draw a flat zero.
	var gpuPct any
	if sample, ok := gpuUtilizationSample(ctx, c.gpu); ok {
		gpuPct = sample
	}

	if err := insertHostRow(ctx, c.db, dbEnvID, sampledAt, agg, gpuPct); err != nil {
		c.logger.Error("metrics collector: failed to insert metrics", "error", err, "env", dbEnvID)
	}
}

// pushSubscribeGrace is how long a freshly subscribed agent has to deliver its
// first batch before the collector stops waiting and pulls that tick itself.
const pushSubscribeGrace = 2 * time.Minute

// pendingPush is one agent's wait for its first batch: since opens the grace
// window, lastSub paces the resubscribes inside and after it.
type pendingPush struct {
	since   time.Time
	lastSub time.Time
}

// pushHandles reports whether envID's history is written by its agent's own
// pushes this tick. A capable agent that is not pushing is subscribed and gets
// one grace window to start; after that the collector pulls every tick while
// still resubscribing, so a broken pusher costs a gap of one window at most.
func (c *Collector) pushHandles(ctx context.Context, envID string) bool {
	if c.hub == nil {
		return false
	}
	if c.hub.PushActive(envID) {
		c.subMu.Lock()
		delete(c.pending, envID)
		c.subMu.Unlock()
		return true
	}
	conn, ok := c.dockerPool.AgentConnectionContext(ctx, envID)
	if !ok || conn.Transport == nil || !conn.Transport.StatsPushSupported() {
		return false
	}
	now := time.Now()
	c.subMu.Lock()
	st, seen := c.pending[envID]
	if !seen {
		st = &pendingPush{since: now}
		c.pending[envID] = st
	}
	subscribe := !seen || now.Sub(st.lastSub) >= pushSubscribeGrace
	if subscribe {
		st.lastSub = now
	}
	inGrace := now.Sub(st.since) < pushSubscribeGrace
	c.subMu.Unlock()
	if subscribe {
		if err := conn.Transport.SubscribeStats(c.hub.Interval()); err != nil {
			c.logger.Debug("metrics collector: stats push subscribe failed", "env", envID, "error", err)
		} else if inGrace {
			c.hub.MarkSubscribed(envID)
		}
	}
	return inGrace
}

// sample reads one environment's stats: through the hub when there is one,
// accepting a snapshot no older than half a tick, or with a direct sweep.
func (c *Collector) sample(ctx context.Context, svc *Service, envID string) ([]ContainerMetric, error) {
	if c.hub == nil {
		return svc.AllContainerStats(ctx, envID)
	}
	snap, err := c.hub.SnapshotFresh(ctx, envID, 15*time.Second)
	if err != nil {
		return nil, err
	}
	return fromSamples(snap.Samples), nil
}

// prune removes metrics older than 24 hours.
// prune enforces the configured retention on both tiers.
//
// Raw rows are NOT deleted on the raw horizon here: compact() already removes
// each raw row as it folds it, and deleting on the horizon itself would take
// out the partial bucket compaction is deliberately still waiting on.
// pruneOrphanedRaw is the backstop for the case where compaction is failing.
func (c *Collector) prune(ctx context.Context, retention coreSettings.RetentionSettings) {
	c.pruneRollups(ctx, retention.MetricsRollupDays)
	c.pruneOrphanedRaw(ctx, retention.MetricsRawHours)
}

func (c *Collector) isMetricsCollectionEnabled(ctx context.Context, envID string) bool {
	if envID == "" {
		envID = c.getDefaultEnvID(ctx)
		if envID == "" {
			return true
		}
	}

	var enabled int
	if err := c.db.QueryRowContext(ctx,
		"SELECT collect_container_metrics_enabled FROM environments WHERE id = ? LIMIT 1",
		envID,
	).Scan(&enabled); err != nil {
		return true
	}

	return enabled == 1
}
