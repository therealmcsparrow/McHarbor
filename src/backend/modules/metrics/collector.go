// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/docker"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
)

// Collector periodically aggregates container stats into the host_metrics table.
type Collector struct {
	db         *sql.DB
	dockerPool *docker.ClientPool
	logger     *slog.Logger
	done       chan struct{}
}

// NewCollector creates a new metrics collector.
func NewCollector(db *sql.DB, pool *docker.ClientPool, logger *slog.Logger) *Collector {
	return &Collector{
		db:         db,
		dockerPool: pool,
		logger:     logger,
		done:       make(chan struct{}),
	}
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
	c.collect()
	c.prune()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			c.collect()
			c.prune()
		}
	}
}

// fallbackEnvID is used only when no default environment exists in the DB.
// Must NOT be "default" since ClientPool.resolveConnection treats "default" as
// an alias and calls resolveDefault(), which would cause infinite recursion.
const fallbackEnvID = "local"

// ensureDefaultEnv creates the local environment row if one doesn't already exist,
// so host_metrics can reference it via the foreign key.
func (c *Collector) ensureDefaultEnv() {
	// Check if any default environment exists (may have been created by auth setup with a different ID)
	var existingID string
	err := c.db.QueryRow("SELECT id FROM environments WHERE is_default = 1 LIMIT 1").Scan(&existingID)
	if err == nil {
		return
	}

	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, err = c.db.Exec(
		`INSERT INTO environments (id, name, connection_type, is_default, is_active, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		fallbackEnvID, "Local", "socket", 1, 1, now, now,
	)
	if err != nil {
		c.logger.Error("metrics collector: failed to create default environment", "error", err)
	}
}

// getDefaultEnvID returns the ID of the default environment from the DB.
func (c *Collector) getDefaultEnvID() string {
	var id string
	if err := c.db.QueryRow("SELECT id FROM environments WHERE is_default = 1 LIMIT 1").Scan(&id); err != nil {
		return fallbackEnvID
	}
	return id
}

// collect fetches stats for all active environments and inserts aggregated metrics.
func (c *Collector) collect() {
	c.ensureDefaultEnv()

	rows, err := c.db.Query(`
		SELECT id
		FROM environments
		WHERE is_active = 1
		  AND orchestrator_type = 'docker'
		  AND collect_container_metrics_enabled = 1
	`)
	if err != nil {
		c.logger.Error("metrics collector: failed to query environments", "error", err)
		return
	}
	defer rows.Close()

	var envIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		envIDs = append(envIDs, id)
	}

	// Also collect for the default (local) environment when enabled.
	if c.isMetricsCollectionEnabled("") {
		envIDs = append(envIDs, "")
	}

	svc := NewService(c.dockerPool)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	agentSettings := coreSettings.ReadAgentSettings(c.db)

	for _, envID := range envIDs {
		// Skip agent environments unless metrics collection is explicitly enabled
		if envID != "" && c.dockerPool.IsAgentEnv(envID) && !agentSettings.MetricsEnabled {
			continue
		}

		stats, err := svc.AllContainerStats(ctx, envID)
		if err != nil {
			c.logger.Debug("metrics collector: skipping environment", "env", envID, "error", err)
			continue
		}

		if len(stats) == 0 {
			continue
		}

		// Aggregate: sum CPU%, sum memory usage / get max memory limit, sum net/block I/O
		var totalCPU float64
		var totalMemUsed, totalMemLimit int64
		var totalNetRx, totalNetTx, totalBlockRead, totalBlockWrite int64
		for _, s := range stats {
			totalCPU += s.CPUPercent
			totalMemUsed += s.MemUsage
			if s.MemLimit > totalMemLimit {
				totalMemLimit = s.MemLimit
			}
			totalNetRx += s.NetRx
			totalNetTx += s.NetTx
			totalBlockRead += s.BlockRead
			totalBlockWrite += s.BlockWrite
		}

		cpuPct := int(totalCPU)
		var memPct int
		if totalMemLimit > 0 {
			memPct = int(float64(totalMemUsed) / float64(totalMemLimit) * 100)
		}

		// Use the actual default env ID for the local environment in DB
		dbEnvID := envID
		if dbEnvID == "" {
			dbEnvID = c.getDefaultEnvID()
		}

		now := time.Now().UTC().Format("2006-01-02 15:04:05")
		_, err = c.db.Exec(
			`INSERT INTO host_metrics (id, environment_id, cpu_percent, memory_percent, memory_used, memory_total,
			                           net_rx, net_tx, block_read, block_write, timestamp)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			xid.New().String(), dbEnvID, cpuPct, memPct, totalMemUsed, totalMemLimit,
			totalNetRx, totalNetTx, totalBlockRead, totalBlockWrite, now,
		)
		if err != nil {
			c.logger.Error("metrics collector: failed to insert metrics", "error", err, "env", dbEnvID)
		}
	}
}

// prune removes metrics older than 24 hours.
func (c *Collector) prune() {
	_, err := c.db.Exec("DELETE FROM host_metrics WHERE timestamp < datetime('now', '-24 hours')")
	if err != nil {
		c.logger.Error("metrics collector: failed to prune old metrics", "error", err)
	}
}

func (c *Collector) isMetricsCollectionEnabled(envID string) bool {
	if envID == "" {
		envID = c.getDefaultEnvID()
		if envID == "" {
			return true
		}
	}

	var enabled int
	if err := c.db.QueryRow(
		"SELECT collect_container_metrics_enabled FROM environments WHERE id = ? LIMIT 1",
		envID,
	).Scan(&enabled); err != nil {
		return true
	}

	return enabled == 1
}
