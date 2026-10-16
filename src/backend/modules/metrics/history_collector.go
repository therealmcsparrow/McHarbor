// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/moby/moby/client"
	"github.com/rs/xid"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

// metricsBucketSeconds is the tier-2 bucket width. It is deliberately not
// configurable: changing it would invalidate every bucket already written,
// since a stored average only means anything against the grid it was
// collected on. The value is recorded per row as bucket_seconds so a future
// change can at least be detected rather than silently rescaling history.
const metricsBucketSeconds = 600

// metricsDiskInterval is how often writable-layer sizes are sampled. Sizes
// require ContainerList with Size:true, which makes the daemon walk each
// container's filesystem — far too expensive for the 30s tick, and the number
// barely moves in an hour anyway.
const metricsDiskInterval = time.Hour

// dbTimeLayout matches the format the rest of the collector writes, so string
// comparison in SQL (`WHERE sampled_at < ?`) orders correctly.
const dbTimeLayout = "2006-01-02 15:04:05"

// bucketExpr floors a timestamp column to a 10-minute boundary. It is
// duplicated into the PARTITION BY clauses because SQLite cannot reference a
// SELECT alias from a window definition in the same query level.
const bucketExpr = `strftime('%Y-%m-%d %H:', {{col}}) || printf('%02d', (CAST(strftime('%M', {{col}}) AS INTEGER) / 10) * 10) || ':00'`

// bucketFor renders bucketExpr against a specific column name.
func bucketFor(col string) string {
	return strings.ReplaceAll(bucketExpr, "{{col}}", col)
}

// containerSize carries the two filesystem numbers Docker reports for a
// container. Collected on the hourly sub-tick only.
type containerSize struct {
	rw     int64
	rootFS int64
}

// ContainerSizes returns writable-layer and total filesystem sizes per
// container id.
//
// This is a separate call rather than part of AllContainerStats because
// Size:true is expensive: the daemon stats the container's diff directory.
// Callers must respect metricsDiskInterval rather than invoking it per tick.
func (s *Service) ContainerSizes(ctx context.Context, envID string) (map[string]containerSize, error) {
	cli, err := s.dockerPool.GetContext(ctx, envID)
	if err != nil {
		return nil, fmt.Errorf("getting Docker client: %w", err)
	}

	listCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	listRes, err := cli.ContainerList(listCtx, client.ContainerListOptions{All: true, Size: true})
	if err != nil {
		return nil, fmt.Errorf("listing containers with sizes: %w", err)
	}
	list := listRes.Items

	sizes := make(map[string]containerSize, len(list))
	for _, ctr := range list {
		sizes[ctr.ID] = containerSize{rw: ctr.SizeRw, rootFS: ctr.SizeRootFs}
	}
	return sizes, nil
}

// recordContainerSamples writes one tier-1 row per container for this tick.
//
// Every container of a tick shares one sampled_at so the rows line up into a
// single column of the series; that pairing is also the dedupe key, so a
// retried tick corrects rather than duplicates.
//
// sizes may be nil on the 29 out of 30 ticks that are not disk ticks, in which
// case the two size columns stay NULL — SQL AVG() ignores NULLs, so the sparse
// column folds into the rollup without a special case.
func (c *Collector) recordContainerSamples(
	ctx context.Context,
	envID string,
	sampledAt time.Time,
	stats []ContainerMetric,
	sizes map[string]containerSize,
) error {
	return insertContainerSamples(ctx, c.db, envID, sampledAt, stats, sizes)
}

// insertContainerSamples is the tier-1 write shared by the live collector and
// the agent offline replay (IngestSamples).
func insertContainerSamples(
	ctx context.Context,
	db *sql.DB,
	envID string,
	sampledAt time.Time,
	stats []ContainerMetric,
	sizes map[string]containerSize,
) error {
	if len(stats) == 0 {
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer rollbackTx(tx)

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO container_metrics (
			id, environment_id, container_id, container_name, container_key,
			cpu_percent, mem_used, mem_limit,
			net_rx, net_tx, block_read, block_write, pids,
			size_rw, size_root_fs, sampled_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("preparing sample insert: %w", err)
	}
	defer closeStmt(stmt)

	ts := sampledAt.UTC().Format(dbTimeLayout)
	for _, m := range stats {
		var sizeRW, sizeRoot any
		if sz, ok := sizes[m.ID]; ok {
			sizeRW, sizeRoot = sz.rw, sz.rootFS
		}
		// NULL rather than "" when nothing identifies the container, so the
		// readers' legacy `container_key IS NULL` branch keeps working for it
		// instead of matching an empty-string key shared with every other
		// unidentifiable container.
		var key any
		if m.Key != "" {
			key = m.Key
		}
		if _, err := stmt.ExecContext(ctx,
			xid.New().String(), envID, m.ID, m.Name, key,
			m.CPUPercent, m.MemUsage, m.MemLimit,
			m.NetRx, m.NetTx, m.BlockRead, m.BlockWrite, int64(m.PIDs),
			sizeRW, sizeRoot, ts,
		); err != nil {
			return fmt.Errorf("inserting sample for %s: %w", m.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing samples: %w", err)
	}
	return nil
}

// compact folds every tier-1 bucket that lies entirely past the raw retention
// line into a tier-2 average, then deletes the raw rows it just folded.
//
// The boundary is the raw cutoff floored to a bucket edge. A bucket starting
// before that boundary ends at or before it, and therefore ends at or before
// the cutoff — which is what "entirely past the line" has to mean. Folding a
// bucket that is still filling would average fewer samples than the bucket
// claims, and a later re-run would disagree with the row it had already
// written. With a 24h raw window this is why the fold lands at 24h+10min.
func (c *Collector) compact(ctx context.Context, rawHours int) {
	if rawHours < 1 {
		rawHours = 1
	}
	boundary := time.Now().UTC().
		Add(-time.Duration(rawHours) * time.Hour).
		Truncate(metricsBucketSeconds * time.Second).
		Format(dbTimeLayout)

	if err := c.compactContainers(ctx, boundary); err != nil {
		c.logger.Error("metrics collector: container compaction failed", "error", err)
	}
	if err := c.compactHost(ctx, boundary); err != nil {
		c.logger.Error("metrics collector: host compaction failed", "error", err)
	}
}

// containerBucket is one folded tier-2 row, read out of the aggregation query
// before being written back. Counter deltas are clamped in Go rather than SQL
// because a container that restarted mid-bucket resets its counter to zero,
// which would otherwise be written as a large negative "throughput".
type containerBucket struct {
	envID, containerID, name, bucketStart                string
	key                                                  sql.NullString
	samples                                              int
	cpuAvg, cpuMax                                       float64
	memUsedAvg, memUsedMax, memLimitMax                  int64
	netRxLast, netTxLast, blockReadLast, blockWrLast     int64
	netRxFirst, netTxFirst, blockReadFirst, blockWrFirst int64
	sizeRW, sizeRoot                                     sql.NullInt64
}

func (c *Collector) compactContainers(ctx context.Context, boundary string) error {
	b := bucketFor("sampled_at")
	query := `
		WITH s AS (
			SELECT
				environment_id, container_id, container_name, container_key,
				cpu_percent, mem_used, mem_limit,
				net_rx, net_tx, block_read, block_write,
				size_rw, size_root_fs,
				{{b}} AS bucket_start,
				ROW_NUMBER() OVER (PARTITION BY container_id, {{b}} ORDER BY sampled_at DESC) AS rn_desc,
				ROW_NUMBER() OVER (PARTITION BY container_id, {{b}} ORDER BY sampled_at ASC) AS rn_asc
			FROM container_metrics
			WHERE sampled_at < ?
		)
		SELECT
			MAX(environment_id), container_id, MAX(container_name), MAX(container_key), bucket_start,
			COUNT(*),
			AVG(cpu_percent), MAX(cpu_percent),
			CAST(AVG(CAST(mem_used AS FLOAT)) AS BIGINT), MAX(mem_used), MAX(mem_limit),
			MAX(CASE WHEN rn_desc = 1 THEN net_rx END),
			MAX(CASE WHEN rn_desc = 1 THEN net_tx END),
			MAX(CASE WHEN rn_desc = 1 THEN block_read END),
			MAX(CASE WHEN rn_desc = 1 THEN block_write END),
			MAX(CASE WHEN rn_asc = 1 THEN net_rx END),
			MAX(CASE WHEN rn_asc = 1 THEN net_tx END),
			MAX(CASE WHEN rn_asc = 1 THEN block_read END),
			MAX(CASE WHEN rn_asc = 1 THEN block_write END),
			CAST(AVG(CAST(size_rw AS FLOAT)) AS BIGINT),
			CAST(AVG(CAST(size_root_fs AS FLOAT)) AS BIGINT)
		FROM s
		GROUP BY container_id, bucket_start
		LIMIT 1000`
	query = strings.ReplaceAll(query, "{{b}}", b)

	rows, err := c.db.QueryContext(ctx, query, boundary)
	if err != nil {
		return fmt.Errorf("aggregating container buckets: %w", err)
	}

	var buckets []containerBucket
	for rows.Next() {
		var k containerBucket
		if err := rows.Scan(
			&k.envID, &k.containerID, &k.name, &k.key, &k.bucketStart,
			&k.samples, &k.cpuAvg, &k.cpuMax,
			&k.memUsedAvg, &k.memUsedMax, &k.memLimitMax,
			&k.netRxLast, &k.netTxLast, &k.blockReadLast, &k.blockWrLast,
			&k.netRxFirst, &k.netTxFirst, &k.blockReadFirst, &k.blockWrFirst,
			&k.sizeRW, &k.sizeRoot,
		); err != nil {
			closeRows(rows)
			return fmt.Errorf("scanning container bucket: %w", err)
		}
		buckets = append(buckets, k)
	}
	if err := rows.Err(); err != nil {
		closeRows(rows)
		return fmt.Errorf("reading container buckets: %w", err)
	}
	closeRows(rows)

	if len(buckets) == 0 {
		return nil
	}

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer rollbackTx(tx)

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO container_metrics_rollup (
			id, environment_id, container_id, container_name, container_key,
			bucket_start, bucket_seconds, samples,
			cpu_avg, cpu_max, mem_used_avg, mem_used_max, mem_limit_max,
			net_rx_last, net_tx_last, block_read_last, block_write_last,
			net_rx_delta, net_tx_delta, block_read_delta, block_write_delta,
			size_rw_avg, size_root_fs_avg
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("preparing rollup insert: %w", err)
	}
	defer closeStmt(stmt)

	for _, k := range buckets {
		if _, err := stmt.ExecContext(ctx,
			xid.New().String(), k.envID, k.containerID, k.name, nullableText(k.key),
			k.bucketStart, metricsBucketSeconds, k.samples,
			k.cpuAvg, k.cpuMax, k.memUsedAvg, k.memUsedMax, k.memLimitMax,
			k.netRxLast, k.netTxLast, k.blockReadLast, k.blockWrLast,
			counterDelta(k.netRxFirst, k.netRxLast),
			counterDelta(k.netTxFirst, k.netTxLast),
			counterDelta(k.blockReadFirst, k.blockReadLast),
			counterDelta(k.blockWrFirst, k.blockWrLast),
			nullableInt(k.sizeRW), nullableInt(k.sizeRoot),
		); err != nil {
			return fmt.Errorf("inserting rollup for %s: %w", k.containerID, err)
		}
	}

	res, err := tx.ExecContext(ctx, "DELETE FROM container_metrics WHERE sampled_at < ?", boundary)
	if err != nil {
		return fmt.Errorf("deleting folded samples: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing compaction: %w", err)
	}

	c.logger.Debug("metrics collector: folded container samples",
		"buckets", len(buckets), "rawRowsDeleted", coredb.RowsAffected(res), "boundary", boundary)
	return nil
}

// hostBucket mirrors containerBucket for the environment-level aggregate.
type hostBucket struct {
	envID, bucketStart                                   string
	samples                                              int
	cpuAvg, cpuMax, memPctAvg, memPctMax                 float64
	memUsedAvg, memUsedMax, memTotalMax                  int64
	netRxLast, netTxLast, blockReadLast, blockWrLast     int64
	netRxFirst, netTxFirst, blockReadFirst, blockWrFirst int64
	// NULL when no sample in the bucket carried GPU telemetry.
	gpuAvg, gpuMax sql.NullFloat64
}

func (c *Collector) compactHost(ctx context.Context, boundary string) error {
	b := bucketFor("timestamp")
	// I/O columns are NULL on rows with no counter (before migration 003, and on
	// rows whose container-sum counters were cleared when host-wide counters
	// replaced them). The io_* row numbers sort those rows last, so a bucket's
	// first and last counters are its first and last real readings — folding a
	// NULL to 0 would turn the whole counter into one bucket's "throughput".
	query := `
		WITH s AS (
			SELECT
				environment_id,
				COALESCE(cpu_percent, 0) AS cpu_percent,
				COALESCE(memory_percent, 0) AS memory_percent,
				COALESCE(memory_used, 0) AS memory_used,
				COALESCE(memory_total, 0) AS memory_total,
				net_rx, net_tx, block_read, block_write,
				gpu_percent,
				{{b}} AS bucket_start,
				ROW_NUMBER() OVER (PARTITION BY environment_id, {{b}} ORDER BY (net_rx IS NULL), timestamp DESC) AS io_desc,
				ROW_NUMBER() OVER (PARTITION BY environment_id, {{b}} ORDER BY (net_rx IS NULL), timestamp ASC) AS io_asc
			FROM host_metrics
			WHERE timestamp < ?
		)
		SELECT
			environment_id, bucket_start, COUNT(*),
			AVG(cpu_percent), MAX(cpu_percent),
			AVG(memory_percent), MAX(memory_percent),
			CAST(AVG(CAST(memory_used AS FLOAT)) AS BIGINT), MAX(memory_used), MAX(memory_total),
			COALESCE(MAX(CASE WHEN io_desc = 1 THEN net_rx END), 0),
			COALESCE(MAX(CASE WHEN io_desc = 1 THEN net_tx END), 0),
			COALESCE(MAX(CASE WHEN io_desc = 1 THEN block_read END), 0),
			COALESCE(MAX(CASE WHEN io_desc = 1 THEN block_write END), 0),
			COALESCE(MAX(CASE WHEN io_asc = 1 THEN net_rx END), 0),
			COALESCE(MAX(CASE WHEN io_asc = 1 THEN net_tx END), 0),
			COALESCE(MAX(CASE WHEN io_asc = 1 THEN block_read END), 0),
			COALESCE(MAX(CASE WHEN io_asc = 1 THEN block_write END), 0),
			AVG(gpu_percent), MAX(gpu_percent)
		FROM s
		GROUP BY environment_id, bucket_start
		LIMIT 1000`
	query = strings.ReplaceAll(query, "{{b}}", b)

	rows, err := c.db.QueryContext(ctx, query, boundary)
	if err != nil {
		return fmt.Errorf("aggregating host buckets: %w", err)
	}

	var buckets []hostBucket
	for rows.Next() {
		var k hostBucket
		if err := rows.Scan(
			&k.envID, &k.bucketStart, &k.samples,
			&k.cpuAvg, &k.cpuMax, &k.memPctAvg, &k.memPctMax,
			&k.memUsedAvg, &k.memUsedMax, &k.memTotalMax,
			&k.netRxLast, &k.netTxLast, &k.blockReadLast, &k.blockWrLast,
			&k.netRxFirst, &k.netTxFirst, &k.blockReadFirst, &k.blockWrFirst,
			&k.gpuAvg, &k.gpuMax,
		); err != nil {
			closeRows(rows)
			return fmt.Errorf("scanning host bucket: %w", err)
		}
		buckets = append(buckets, k)
	}
	if err := rows.Err(); err != nil {
		closeRows(rows)
		return fmt.Errorf("reading host buckets: %w", err)
	}
	closeRows(rows)

	if len(buckets) == 0 {
		return nil
	}

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer rollbackTx(tx)

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO host_metrics_rollup (
			id, environment_id, bucket_start, bucket_seconds, samples,
			cpu_avg, cpu_max, mem_percent_avg, mem_percent_max,
			mem_used_avg, mem_used_max, mem_total_max,
			net_rx_last, net_tx_last, block_read_last, block_write_last,
			net_rx_delta, net_tx_delta, block_read_delta, block_write_delta,
			gpu_avg, gpu_max
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("preparing host rollup insert: %w", err)
	}
	defer closeStmt(stmt)

	for _, k := range buckets {
		if _, err := stmt.ExecContext(ctx,
			xid.New().String(), k.envID, k.bucketStart, metricsBucketSeconds, k.samples,
			k.cpuAvg, k.cpuMax, k.memPctAvg, k.memPctMax,
			k.memUsedAvg, k.memUsedMax, k.memTotalMax,
			k.netRxLast, k.netTxLast, k.blockReadLast, k.blockWrLast,
			counterDelta(k.netRxFirst, k.netRxLast),
			counterDelta(k.netTxFirst, k.netTxLast),
			counterDelta(k.blockReadFirst, k.blockReadLast),
			counterDelta(k.blockWrFirst, k.blockWrLast),
			k.gpuAvg, k.gpuMax,
		); err != nil {
			return fmt.Errorf("inserting host rollup for %s: %w", k.envID, err)
		}
	}

	res, err := tx.ExecContext(ctx, "DELETE FROM host_metrics WHERE timestamp < ?", boundary)
	if err != nil {
		return fmt.Errorf("deleting folded host samples: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing host compaction: %w", err)
	}

	c.logger.Debug("metrics collector: folded host samples",
		"buckets", len(buckets), "rawRowsDeleted", coredb.RowsAffected(res), "boundary", boundary)
	return nil
}

// pruneRollups drops tier-2 rows past their retention. Tier 1 is not pruned
// here: compact() already deletes raw rows as it folds them, so anything still
// in the raw tables is inside the raw window by construction.
func (c *Collector) pruneRollups(ctx context.Context, rollupDays int) {
	if rollupDays < 1 {
		rollupDays = 1
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -rollupDays).Format(dbTimeLayout)

	// bounded by the two fixed rollup tables
	for _, table := range []string{"container_metrics_rollup", "host_metrics_rollup"} {
		if _, err := c.db.ExecContext(ctx,
			"DELETE FROM "+table+" WHERE bucket_start < ?", cutoff, // SQL identifier: table iterates a fixed literal slice of two rollup tables
		); err != nil {
			c.logger.Error("metrics collector: failed to prune rollup", "table", table, "error", err)
		}
	}
}

// pruneOrphanedRaw is a backstop, not the normal path. Raw rows are removed by
// compact() as it folds them; if compaction is failing (a malformed timestamp,
// a locked database, a bug in the aggregation) the raw tables would otherwise
// grow without limit. Deleting at twice the raw horizon is far enough past the
// fold boundary that it can never race a bucket compaction still intends to
// process, while still bounding the damage.
func (c *Collector) pruneOrphanedRaw(ctx context.Context, rawHours int) {
	if rawHours < 1 {
		rawHours = 1
	}
	cutoff := time.Now().UTC().Add(-2 * time.Duration(rawHours) * time.Hour).Format(dbTimeLayout)

	res, err := c.db.ExecContext(ctx, "DELETE FROM container_metrics WHERE sampled_at < ?", cutoff)
	if err != nil {
		c.logger.Error("metrics collector: failed to prune orphaned samples", "error", err)
		return
	}
	if n := coredb.RowsAffected(res); n > 0 {
		c.logger.Warn("metrics collector: deleted raw samples that compaction never folded",
			"rows", n, "cutoff", cutoff)
	}

	res, err = c.db.ExecContext(ctx, "DELETE FROM host_metrics WHERE timestamp < ?", cutoff)
	if err != nil {
		c.logger.Error("metrics collector: failed to prune orphaned host metrics", "error", err)
		return
	}
	if n := coredb.RowsAffected(res); n > 0 {
		c.logger.Warn("metrics collector: deleted host metrics that compaction never folded",
			"rows", n, "cutoff", cutoff)
	}
}

// counterDelta returns the movement of a cumulative counter across a bucket.
//
// Docker's network and block counters reset to zero when a container restarts,
// so last < first is not an error condition — it means the container was
// replaced mid-bucket. Reporting zero understates that bucket's throughput by
// whatever ran before the restart, which is the honest option: the alternative
// is a negative number, and treating the post-restart value as the delta would
// invent traffic that the previous container generated.
func counterDelta(first, last int64) int64 {
	if last < first {
		return 0
	}
	return last - first
}

// nullableText keeps a NULL identity NULL rather than folding it to "", so a
// bucket compacted from rows written before migration 083 stays on the readers'
// legacy `container_key IS NULL` branch instead of matching an empty-string key.
func nullableText(v sql.NullString) any {
	if !v.Valid || v.String == "" {
		return nil
	}
	return v.String
}

// nullableInt keeps a NULL aggregate NULL rather than writing it back as 0,
// so "no disk sample in this bucket" stays distinguishable from "zero bytes".
func nullableInt(v sql.NullInt64) any {
	if !v.Valid {
		return nil
	}
	return v.Int64
}
