// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/quiesce"
)

// healthCollectInterval matches the metrics collector's cadence and is well
// under the shortest sensible healthcheck interval, so no probe is missed:
// Docker keeps the last five results in State.Health.Log, meaning a probe
// stays visible for five of its own intervals after it ran.
const healthCollectInterval = 30 * time.Second

// healthInspectConcurrency bounds the fan-out when a host runs many
// healthchecked containers. Inspect is cheap locally but is a round trip, so
// the pool keeps a busy host from producing a 200-deep serial chain per tick.
const healthInspectConcurrency = 8

// HealthCollector persists healthcheck probes and container run-state samples
// so the Overview tab can graph them.
//
// Agent environments are deliberately skipped here, for the same reason the
// metrics collector skips them: every call is a WebSocket round trip and a
// per-tick sweep would be costly over a slow link. Agent environments still
// accumulate history, just lazily — Service.healthHistory records what it sees
// whenever someone opens a container's detail page.
type HealthCollector struct {
	db          *sql.DB
	dockerPool  *docker.ClientPool
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
func NewHealthCollector(database *sql.DB, pool *docker.ClientPool, logger *slog.Logger) *HealthCollector {
	return &HealthCollector{
		db:         database,
		dockerPool: pool,
		logger:     logger,
		done:       make(chan struct{}),
	}
}

// NewHealthCollectorWithCoordinator creates a collector that only scrapes on
// the elected HA leader. SQLite installs always report leader=true, so
// single-node behaviour is unchanged.
func NewHealthCollectorWithCoordinator(database *sql.DB, pool *docker.ClientPool, logger *slog.Logger, coord healthCoordinator, nodeID string) *HealthCollector {
	c := NewHealthCollector(database, pool, logger)
	c.coordinator = coord
	c.nodeID = nodeID
	c.hasCoord = coord != nil
	return c
}

// Start launches the background collection goroutine.
func (c *HealthCollector) Start() {
	go c.run()
	c.logger.Info("container health collector started", "interval", healthCollectInterval.String())
}

// Stop signals the collector to shut down.
func (c *HealthCollector) Stop() {
	close(c.done)
}

func (c *HealthCollector) run() {
	ticker := time.NewTicker(healthCollectInterval)
	defer ticker.Stop()

	// Stagger the first scrape so startup is not competing with the metrics
	// and activity collectors for the Docker socket.
	select {
	case <-c.done:
		return
	case <-time.After(8 * time.Second):
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
		c.logger.Debug("container health collector: not leader this tick; skipping", "node", c.nodeID)
		return
	}

	// Read-only: the collector lists and inspects, it never creates or
	// recreates, so it needs no secret resolver.
	svc := NewService(c.dockerPool, c.db, "", nil)
	for _, envID := range c.environmentIDs(ctx) {
		if c.dockerPool.IsAgentEnvContext(ctx, envID) {
			continue
		}
		c.collectEnv(ctx, svc, envID)
	}
}

func (c *HealthCollector) leaderFor(ctx context.Context) bool {
	if !c.hasCoord {
		return true
	}
	return c.coordinator.LeaderOf(ctx, "container-health-collector")
}

// environmentIDs returns the active Docker environments to sweep, plus the
// implicit local environment ("").
func (c *HealthCollector) environmentIDs(ctx context.Context) []string {
	ids := []string{""}
	if c.db == nil {
		return ids
	}

	rows, err := c.db.QueryContext(ctx, `
		SELECT id
		FROM environments
		WHERE is_active = 1
		LIMIT 1000
	`)
	if err != nil {
		c.logger.Error("container health collector: failed to query environments", "error", err)
		return ids
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing rows failed", "error", err)
		}
	}()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		c.logger.Error("container health collector: failed to read environments", "error", err)
	}
	return ids
}

// collectEnv records one run-state sample per container in the environment and
// appends any new healthcheck probes.
func (c *HealthCollector) collectEnv(ctx context.Context, svc *Service, envID string) {
	cli, err := c.dockerPool.GetContext(ctx, envID)
	if err != nil {
		c.logger.Debug("container health collector: docker unavailable", "env", logEnvID(envID), "error", err)
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	summariesRes, err := cli.ContainerList(listCtx, client.ContainerListOptions{All: true})
	if err != nil {
		c.logger.Debug("container health collector: list failed", "env", logEnvID(envID), "error", err)
		return
	}
	summaries := summariesRes.Items

	bucket := currentSampleBucket()
	var healthChecked []container.Summary

	for _, summary := range summaries {
		name := summaryName(summary)
		sample := UptimeSample{
			SampledAt: bucket,
			State:     string(summary.State),
			Running:   string(summary.State) == "running",
			Health:    healthFromStatus(summary.Status),
		}
		// Labels come free on the list result, so the sweep writes the stable
		// identity without an extra inspect per container.
		key := docker.ContainerKey(summary.Labels, name)
		if err := svc.recordStateSample(ctx, envID, summary.ID, name, key, sample); err != nil {
			c.logger.Error("container health collector: sample failed",
				"env", logEnvID(envID), "container", summary.ID, "error", err)
			continue
		}
		if sample.Health != "" {
			healthChecked = append(healthChecked, summary)
		}
	}

	c.collectProbes(ctx, svc, cli, envID, healthChecked)
}

// collectProbes inspects the healthchecked containers concurrently and stores
// whatever probe results have not been seen yet.
func (c *HealthCollector) collectProbes(ctx context.Context, svc *Service, cli dockerInspector, envID string, summaries []container.Summary) {
	if len(summaries) == 0 {
		return
	}

	sem := make(chan struct{}, healthInspectConcurrency)
	var wg sync.WaitGroup

	for _, summary := range summaries {
		wg.Add(1)
		go func(summary container.Summary) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			inspectCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
			defer cancel()

			infoRes, err := cli.ContainerInspect(inspectCtx, summary.ID, client.ContainerInspectOptions{})
			if err != nil {
				c.logger.Debug("container health collector: inspect failed",
					"env", logEnvID(envID), "container", summary.ID, "error", err)
				return
			}
			info := infoRes.Container
			if info.State == nil || info.State.Health == nil {
				return
			}

			name := strings.TrimPrefix(info.Name, "/")
			var labels map[string]string
			if info.Config != nil {
				labels = info.Config.Labels
			}
			key := docker.ContainerKey(labels, name)
			if err := svc.recordHealthProbes(ctx, envID, summary.ID, name, key, info.State.Health.Log); err != nil {
				c.logger.Error("container health collector: probe write failed",
					"env", logEnvID(envID), "container", summary.ID, "error", err)
			}
		}(summary)
	}

	wg.Wait()
}

func (c *HealthCollector) prune(ctx context.Context) {
	deleted, err := pruneHealthHistory(ctx, c.db)
	if err != nil {
		c.logger.Error("container health collector: prune failed", "error", err)
		return
	}
	if deleted > 0 {
		c.logger.Debug("container health collector: pruned history rows", "rows", deleted)
	}
}

// dockerInspector is the slice of the Docker client the probe pass needs,
// which keeps collectProbes testable without a live daemon.
type dockerInspector interface {
	ContainerInspect(ctx context.Context, id string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error)
}

// healthFromStatus extracts the health marker Docker embeds in the list
// endpoint's status string ("Up 2 hours (healthy)"), avoiding an inspect for
// containers that have no healthcheck at all.
func healthFromStatus(status string) string {
	switch {
	case strings.Contains(status, "(healthy)"):
		return "healthy"
	case strings.Contains(status, "(unhealthy)"):
		return "unhealthy"
	case strings.Contains(status, "(health: starting)"):
		return "starting"
	default:
		return ""
	}
}

func summaryName(summary container.Summary) string {
	if len(summary.Names) == 0 {
		return ""
	}
	return strings.TrimPrefix(summary.Names[0], "/")
}

func logEnvID(envID string) string {
	if envID == "" {
		return "local"
	}
	return envID
}
