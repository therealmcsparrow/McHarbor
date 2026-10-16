// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/rs/xid"
)

// hostAggregate folds one tick's per-container samples into the numbers the
// host_metrics row carries: summed CPU, summed memory against the largest
// limit seen, and summed I/O counters. Shared by the live collector and the
// offline replay so both paths produce the same host row for the same input.
type hostAggregate struct {
	cpuPct, memPct                      int
	memUsed, memLimit                   int64
	netRx, netTx, blockRead, blockWrite int64
	// hasIO is false when no container contributed, so the I/O columns are
	// written NULL ("no data") rather than as a zero counter.
	hasIO bool
}

func aggregateHost(stats []ContainerMetric) hostAggregate {
	agg := hostAggregate{hasIO: len(stats) > 0}
	var totalCPU float64
	for _, s := range stats {
		totalCPU += s.CPUPercent
		agg.memUsed += s.MemUsage
		if s.MemLimit > agg.memLimit {
			agg.memLimit = s.MemLimit
		}
		agg.netRx += s.NetRx
		agg.netTx += s.NetTx
		agg.blockRead += s.BlockRead
		agg.blockWrite += s.BlockWrite
	}
	agg.cpuPct = int(totalCPU)
	if agg.memLimit > 0 {
		agg.memPct = int(float64(agg.memUsed) / float64(agg.memLimit) * 100)
	}
	return agg
}

// insertHostRow writes the tier-1 host aggregate for one tick. gpuPct is nil
// when the host reports no GPU telemetry.
func insertHostRow(ctx context.Context, db *sql.DB, envID string, sampledAt time.Time, agg hostAggregate, gpuPct any) error {
	var netRx, netTx, blockRead, blockWrite any
	if agg.hasIO {
		netRx, netTx, blockRead, blockWrite = agg.netRx, agg.netTx, agg.blockRead, agg.blockWrite
	}
	_, err := db.ExecContext(ctx,
		`INSERT INTO host_metrics (id, environment_id, cpu_percent, memory_percent, memory_used, memory_total,
		                           net_rx, net_tx, block_read, block_write, gpu_percent, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		xid.New().String(), envID, agg.cpuPct, agg.memPct, agg.memUsed, agg.memLimit,
		netRx, netTx, blockRead, blockWrite, gpuPct, sampledAt.UTC().Format(dbTimeLayout),
	)
	if err != nil {
		return fmt.Errorf("inserting host metrics: %w", err)
	}
	return nil
}

// IngestSamples persists one tick of container stats that the remote agent
// collected while it was disconnected. It writes the same two tier-1 rows the
// live collector writes — container_metrics per container and one host_metrics
// aggregate — so the compaction and history readers need no special case.
//
// Container sizes are never available offline (the agent does not walk diff
// directories), so those columns stay NULL, exactly as they do on the 29 of 30
// live ticks that are not disk ticks. The GPU column is NULL too: the agent
// does not sample GPU telemetry.
//
// The container rows use INSERT OR REPLACE on (container_id, sampled_at), and
// the host row is skipped when one already exists at that instant, so a
// replayed batch — offline, or a pushed one whose ack was lost — corrects
// rather than duplicates.
func IngestSamples(ctx context.Context, db *sql.DB, envID string, sampledAt time.Time, stats []ContainerMetric) error {
	return ingestSamples(ctx, db, envID, sampledAt, stats, nil)
}

// ingestSamples is IngestSamples with an optional live host reading: when host
// is set the host row's CPU and memory come from it, and a batch with no
// containers still writes that row, so a host that runs none keeps a history.
func ingestSamples(ctx context.Context, db *sql.DB, envID string, sampledAt time.Time, stats []ContainerMetric, host *HostReading) error {
	if len(stats) == 0 && host == nil {
		return nil
	}
	if len(stats) > 0 {
		if err := insertContainerSamples(ctx, db, envID, sampledAt, stats, nil); err != nil {
			return err
		}
	}
	var existing int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM host_metrics WHERE environment_id = ? AND timestamp = ?`,
		envID, sampledAt.UTC().Format(dbTimeLayout),
	).Scan(&existing)
	if err != nil {
		return fmt.Errorf("checking for an existing host row: %w", err)
	}
	if existing > 0 {
		return nil
	}
	agg := aggregateHost(stats)
	if host != nil {
		agg = mergeHost(agg, *host, true)
	}
	return insertHostRow(ctx, db, envID, sampledAt, agg, nil)
}
