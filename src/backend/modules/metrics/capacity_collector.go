// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package metrics collects, stores and serves container, host and capacity metrics per environment.
package metrics

import (
	"context"
	"slices"
	"sync"
	"time"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"

	"github.com/therealmcsparrow/mcharbor/core/capacity"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
)

// capacityInterval is how often filesystem capacity is sampled. Hourly, not
// every 30 seconds: a filesystem does not move fast enough to justify 2,880 rows
// a day per subject, and the fit reading them needs days rather than minutes.
const capacityInterval = time.Hour

// StorageUsageReader reports used-of-total for every configured storage
// destination. modules/storage_locations satisfies it; the collector holds the
// interface so this module keeps importing no sibling module.
type StorageUsageReader interface {
	CapacityReadings(ctx context.Context, environmentID string) ([]capacity.Reading, error)
}

// SetStorageUsage gives the capacity sweep access to storage destinations.
//
// It is settable after Start because the reader it takes needs the plugin
// runtime, which is wired much later in boot than the collector is. Until it is
// set the sweep records root filesystems only, which is the honest partial answer
// rather than a wrong complete one.
func (c *Collector) SetStorageUsage(r StorageUsageReader) {
	c.capacityMu.Lock()
	c.storageUsage = r
	c.capacityMu.Unlock()
}

// SetAgentPool lets the capacity sweep read a remote host's filesystem through
// its agent. Without it an agent environment reports no filesystem at all, which
// is why this is wired rather than left to the Docker pool.
func (c *Collector) SetAgentPool(p *coreagent.AgentPool) {
	c.capacityMu.Lock()
	c.agentPool = p
	c.capacityMu.Unlock()
}

func (c *Collector) storageUsageReader() StorageUsageReader {
	c.capacityMu.RLock()
	defer c.capacityMu.RUnlock()
	return c.storageUsage
}

// collectCapacity samples every environment's root filesystem and every storage
// destination into the retained capacity series.
//
// Environments are swept concurrently under the same bound the stats sweep uses:
// a one-shot call through the agent transport costs seconds, and a sequential
// loop over a dozen agent environments would outlast the tick it started on.
func (c *Collector) collectCapacity(ctx context.Context, at time.Time) {
	svc := c.capacityService()
	envIDs, defaultEnvID := c.capacitySubjects(ctx)

	var wg sync.WaitGroup
	sem := make(chan struct{}, collectEnvConcurrency)
	for _, envID := range envIDs {
		wg.Add(1)
		sem <- struct{}{}
		go func(envID string) {
			defer wg.Done()
			defer func() { <-sem }()
			c.sampleRootFS(ctx, svc, envID, at)
		}(envID)
	}
	wg.Wait()

	// Destinations are global configuration, not per-environment, so they are
	// attributed to the default environment: capacity_samples.environment_id is
	// a foreign key and every row needs one that exists.
	reader := c.storageUsageReader()
	if reader == nil || defaultEnvID == "" {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	readings, err := reader.CapacityReadings(readCtx, defaultEnvID)
	cancel()
	if err != nil {
		c.logger.Debug("capacity collector: storage destinations unavailable", "error", err)
		return
	}
	for _, r := range readings {
		if err := capacity.Write(ctx, c.db, r, at); err != nil {
			c.logger.Error("capacity collector: failed to record destination sample",
				"location", r.SubjectID, "error", err)
		}
	}
}

// capacitySubjects lists the environments to sample and the environment storage
// destinations are attributed to.
//
// It enumerates independently of collect(): an environment with container metrics
// switched off still has a filesystem that fills up, and tying capacity to the
// per-container collection flag would make the one setting mean two things.
func (c *Collector) capacitySubjects(ctx context.Context) ([]string, string) {
	// No LIMIT: the collector's own environment list, and a cap would silently skip sampling.
	rows, err := c.db.QueryContext(ctx, `SELECT id FROM environments WHERE is_active = 1`)
	if err != nil {
		c.logger.Error("capacity collector: failed to query environments", "error", err)
		return nil, ""
	}
	defer closeRows(rows)
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		c.logger.Error("capacity collector: failed to read environments", "error", err)
	}
	defaultEnvID := c.getDefaultEnvID(ctx)
	if defaultEnvID != "" && !slices.Contains(ids, defaultEnvID) {
		ids = append(ids, defaultEnvID)
	}
	return ids, defaultEnvID
}

// capacityService returns a metrics service that can reach remote agents, so an
// agent environment's filesystem is read over the transport rather than skipped.
func (c *Collector) capacityService() *Service {
	c.capacityMu.RLock()
	pool := c.agentPool
	c.capacityMu.RUnlock()
	if pool == nil {
		return NewService(c.dockerPool)
	}
	return NewServiceWithAgent(c.dockerPool, pool)
}

// sampleRootFS records one environment's root filesystem. Local environments are
// read with statfs(2) on the McHarbor process; agent environments ask the agent,
// which already carries FSTotal and FSUsed in its host-stats payload — no new
// protocol message is needed for any of this.
func (c *Collector) sampleRootFS(ctx context.Context, svc *Service, envID string, at time.Time) {
	dbEnvID := envID
	if dbEnvID == "" {
		dbEnvID = c.getDefaultEnvID(ctx)
	}
	if dbEnvID == "" {
		return
	}
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	info, err := svc.HostInfo(callCtx, envID)
	cancel()
	if err != nil {
		c.logger.Debug("capacity collector: skipping environment", "env", dbEnvID, "error", err)
		return
	}
	if info.HostFS.Total <= 0 {
		return
	}
	reading := capacity.Reading{
		EnvironmentID: dbEnvID,
		SubjectKind:   capacity.KindRootFS,
		SubjectID:     dbEnvID,
		SubjectName:   info.HostFS.Path,
		UsedBytes:     info.HostFS.Used,
		TotalBytes:    info.HostFS.Total,
	}
	if err := capacity.Write(ctx, c.db, reading, at); err != nil {
		c.logger.Error("capacity collector: failed to record root filesystem sample", "env", dbEnvID, "error", err)
	}
}

// pruneCapacity drops capacity samples past the rollup retention. The series
// shares retention_metrics_rollup_days rather than minting a sixth retention
// key: a capacity history shorter than the metrics rollup would make the disk
// forecast worse than the memory one for no reason an operator asked for.
func (c *Collector) pruneCapacity(ctx context.Context, retention coreSettings.RetentionSettings) {
	if _, err := capacity.Prune(ctx, c.db, retention.MetricsRollupDays); err != nil {
		c.logger.Error("capacity collector: failed to prune capacity samples", "error", err)
	}
}
