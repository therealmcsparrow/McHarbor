// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

// maxHistoryPoints bounds any single series. A 30-day window at 10-minute
// resolution is 4,320 points, which is already more than a chart can show; the
// cap exists so a hand-crafted `from` far in the past cannot pull the whole
// table into memory.
const maxHistoryPoints = 5000

// rawResolutionLimit is the window beyond which tier-1 rows are folded into
// bucket-sized groups on read.
//
// Below it the caller gets true 30-second resolution. Above it, returning raw
// and rollup points in the same series would hand the chart a line that
// silently changes resolution partway along — 30s detail for the last day,
// 10-minute averages before it — and a peak would appear to vanish at the
// seam. Folding on read keeps one uniform grid across the whole window.
const rawResolutionLimit = 24 * time.Hour

// HistoryService reads the tiered metric history. It takes only the database:
// history is entirely a stored-data question and never touches Docker.
type HistoryService struct {
	db *sql.DB
}

// NewHistoryService creates a history reader.
func NewHistoryService(database *sql.DB) *HistoryService {
	return &HistoryService{db: database}
}

// ContainerHistory returns the series for one container across [from, to).
func (h *HistoryService) ContainerHistory(ctx context.Context, envID, containerID string, from, to time.Time) (*MetricSeries, error) {
	envID = h.resolveEnvID(ctx, envID)
	series := &MetricSeries{
		EnvironmentID: envID,
		ContainerID:   containerID,
		From:          from.UTC().Format(time.RFC3339),
		To:            to.UTC().Format(time.RFC3339),
		BucketSeconds: resolutionFor(from, to),
	}

	// Which rows belong to this container is a question about its identity,
	// not about the id in the URL: a recreated container carries a new id and
	// the same key, and asking by id alone is what used to blank the graphs
	// every time a workload was updated.
	scope, err := h.scopeFor(ctx, envID, containerID)
	if err != nil {
		return nil, err
	}

	rollup, err := h.containerRollupPoints(ctx, envID, scope, from, to)
	if err != nil {
		return nil, err
	}
	raw, name, err := h.containerRawPoints(ctx, envID, scope, from, to)
	if err != nil {
		return nil, err
	}
	series.Name = name

	series.Points = mergeSeries(rollup, raw)
	finalize(series)
	return series, nil
}

// AllContainerHistory returns one series per container that has any history in
// the window. This is the shape reports consume: a per-container breakdown for
// a period, rather than a single container's detail.
func (h *HistoryService) AllContainerHistory(ctx context.Context, envID string, from, to time.Time, limit int) ([]MetricSeries, error) {
	if limit <= 0 || limit > 500 {
		limit = 500
	}

	envID = h.resolveEnvID(ctx, envID)
	ids, err := h.containerIDsInWindow(ctx, envID, from, to, limit)
	if err != nil {
		return nil, err
	}

	out := make([]MetricSeries, 0, len(ids))
	for _, id := range ids {
		s, err := h.ContainerHistory(ctx, envID, id, from, to)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, nil
}

// HostHistory returns the environment-level aggregate series.
func (h *HistoryService) HostHistory(ctx context.Context, envID string, from, to time.Time) (*MetricSeries, error) {
	envID = h.resolveEnvID(ctx, envID)
	series := &MetricSeries{
		EnvironmentID: envID,
		From:          from.UTC().Format(time.RFC3339),
		To:            to.UTC().Format(time.RFC3339),
		BucketSeconds: resolutionFor(from, to),
	}

	rollup, err := h.hostRollupPoints(ctx, envID, from, to)
	if err != nil {
		return nil, err
	}
	raw, err := h.hostRawPoints(ctx, envID, from, to)
	if err != nil {
		return nil, err
	}

	series.Points = mergeSeries(rollup, raw)
	finalize(series)
	return series, nil
}

// --- tier 2 reads ----------------------------------------------------

func (h *HistoryService) containerRollupPoints(ctx context.Context, envID string, scope coredb.ContainerScope, from, to time.Time) ([]MetricPoint, error) {
	rows, err := h.db.QueryContext(ctx, expandSQL(`
		SELECT bucket_start, samples, cpu_avg, cpu_max,
		       mem_used_avg, mem_used_max, mem_limit_max,
		       net_rx_last, net_tx_last, block_read_last, block_write_last,
		       net_rx_delta, net_tx_delta, block_read_delta, block_write_delta,
		       size_rw_avg, size_root_fs_avg
		FROM container_metrics_rollup
		WHERE {{where}} AND environment_id = ?
		  AND bucket_start >= ? AND bucket_start < ?
		ORDER BY bucket_start ASC
		LIMIT ?`, "{{where}}", scope.Where),
		scope.Args(envID, dbTime(from), dbTime(to), maxHistoryPoints)...)
	if err != nil {
		return nil, fmt.Errorf("querying container rollup: %w", err)
	}
	defer closeRows(rows)

	var points []MetricPoint
	for rows.Next() {
		var p MetricPoint
		var sizeRW, sizeRoot sql.NullInt64
		if err := rows.Scan(&p.Timestamp, &p.Samples, &p.CPUPercent, &p.CPUMax,
			&p.MemUsed, &p.MemUsedMax, &p.MemLimit,
			&p.NetRx, &p.NetTx, &p.BlockRead, &p.BlockWrite,
			&p.NetRxDelta, &p.NetTxDelta, &p.BlockReadDelta, &p.BlockWriteDelta,
			&sizeRW, &sizeRoot); err != nil {
			return nil, fmt.Errorf("scanning container rollup: %w", err)
		}
		p.Resolution = "rollup"
		p.SizeRw = optionalInt(sizeRW)
		p.SizeRootFs = optionalInt(sizeRoot)
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading container rollup: %w", err)
	}
	return points, nil
}

func (h *HistoryService) hostRollupPoints(ctx context.Context, envID string, from, to time.Time) ([]MetricPoint, error) {
	rows, err := h.db.QueryContext(ctx, `
		SELECT bucket_start, samples, cpu_avg, cpu_max,
		       mem_percent_avg, mem_percent_max,
		       mem_used_avg, mem_used_max, mem_total_max,
		       net_rx_last, net_tx_last, block_read_last, block_write_last,
		       net_rx_delta, net_tx_delta, block_read_delta, block_write_delta,
		       gpu_avg
		FROM host_metrics_rollup
		WHERE environment_id = ? AND bucket_start >= ? AND bucket_start < ?
		ORDER BY bucket_start ASC
		LIMIT ?`,
		envID, dbTime(from), dbTime(to), maxHistoryPoints)
	if err != nil {
		return nil, fmt.Errorf("querying host rollup: %w", err)
	}
	defer closeRows(rows)

	var points []MetricPoint
	for rows.Next() {
		var p MetricPoint
		var memPctMax float64
		var gpuAvg sql.NullFloat64
		if err := rows.Scan(&p.Timestamp, &p.Samples, &p.CPUPercent, &p.CPUMax,
			&p.MemPercent, &memPctMax,
			&p.MemUsed, &p.MemUsedMax, &p.MemLimit,
			&p.NetRx, &p.NetTx, &p.BlockRead, &p.BlockWrite,
			&p.NetRxDelta, &p.NetTxDelta, &p.BlockReadDelta, &p.BlockWriteDelta, &gpuAvg); err != nil {
			return nil, fmt.Errorf("scanning host rollup: %w", err)
		}
		p.Resolution = "rollup"
		p.GPUPercent = optionalFloat(gpuAvg)
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading host rollup: %w", err)
	}
	return points, nil
}

// --- tier 1 reads ----------------------------------------------------

// containerRawPoints returns the raw tail, folded on read when the window is
// wide enough that mixing resolutions would distort the chart.
func (h *HistoryService) containerRawPoints(ctx context.Context, envID string, scope coredb.ContainerScope, from, to time.Time) ([]MetricPoint, string, error) {
	if to.Sub(from) > rawResolutionLimit {
		pts, name, err := h.containerRawFolded(ctx, envID, scope, from, to)
		return pts, name, err
	}

	rows, err := h.db.QueryContext(ctx, expandSQL(`
		SELECT sampled_at, container_name, cpu_percent, mem_used, mem_limit,
		       net_rx, net_tx, block_read, block_write, size_rw, size_root_fs
		FROM container_metrics
		WHERE {{where}} AND environment_id = ?
		  AND sampled_at >= ? AND sampled_at < ?
		ORDER BY sampled_at ASC
		LIMIT ?`, "{{where}}", scope.Where),
		scope.Args(envID, dbTime(from), dbTime(to), maxHistoryPoints)...)
	if err != nil {
		return nil, "", fmt.Errorf("querying container samples: %w", err)
	}
	defer closeRows(rows)

	var (
		points []MetricPoint
		name   string
	)
	for rows.Next() {
		var p MetricPoint
		var rowName sql.NullString
		var sizeRW, sizeRoot sql.NullInt64
		if err := rows.Scan(&p.Timestamp, &rowName, &p.CPUPercent, &p.MemUsed, &p.MemLimit,
			&p.NetRx, &p.NetTx, &p.BlockRead, &p.BlockWrite, &sizeRW, &sizeRoot); err != nil {
			return nil, "", fmt.Errorf("scanning container sample: %w", err)
		}
		p.Resolution = "raw"
		p.Samples = 1
		p.CPUMax = p.CPUPercent
		p.MemUsedMax = p.MemUsed
		p.SizeRw = optionalInt(sizeRW)
		p.SizeRootFs = optionalInt(sizeRoot)
		if rowName.Valid && rowName.String != "" {
			name = rowName.String
		}
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("reading container samples: %w", err)
	}

	// Tier 1 stores counters, not deltas. Deriving them here rather than
	// storing them keeps the raw row a faithful record of what Docker
	// reported, and makes a raw point carry the same fields as a rollup one.
	deriveDeltas(points)
	return points, name, nil
}

// containerRawFolded aggregates tier-1 rows into bucket-sized groups using the
// same expression the collector folds with, so a read-time bucket and a
// compacted bucket describe the same interval.
func (h *HistoryService) containerRawFolded(ctx context.Context, envID string, scope coredb.ContainerScope, from, to time.Time) ([]MetricPoint, string, error) {
	b := bucketFor("sampled_at")
	rows, err := h.db.QueryContext(ctx, expandSQL(`
		SELECT {{b}} AS bucket_start,
		       COUNT(*), MAX(container_name),
		       AVG(cpu_percent), MAX(cpu_percent),
		       CAST(AVG(mem_used) AS INTEGER), MAX(mem_used), MAX(mem_limit),
		       MAX(net_rx), MAX(net_tx), MAX(block_read), MAX(block_write),
		       MAX(net_rx) - MIN(net_rx), MAX(net_tx) - MIN(net_tx),
		       MAX(block_read) - MIN(block_read), MAX(block_write) - MIN(block_write),
		       CAST(AVG(size_rw) AS INTEGER), CAST(AVG(size_root_fs) AS INTEGER)
		FROM container_metrics
		WHERE {{where}} AND environment_id = ?
		  AND sampled_at >= ? AND sampled_at < ?
		GROUP BY bucket_start
		ORDER BY bucket_start ASC
		LIMIT ?`, "{{b}}", b, "{{where}}", scope.Where),
		scope.Args(envID, dbTime(from), dbTime(to), maxHistoryPoints)...)
	if err != nil {
		return nil, "", fmt.Errorf("querying folded container samples: %w", err)
	}
	defer closeRows(rows)

	var (
		points []MetricPoint
		name   string
	)
	for rows.Next() {
		var p MetricPoint
		var rowName sql.NullString
		var sizeRW, sizeRoot sql.NullInt64
		if err := rows.Scan(&p.Timestamp, &p.Samples, &rowName,
			&p.CPUPercent, &p.CPUMax,
			&p.MemUsed, &p.MemUsedMax, &p.MemLimit,
			&p.NetRx, &p.NetTx, &p.BlockRead, &p.BlockWrite,
			&p.NetRxDelta, &p.NetTxDelta, &p.BlockReadDelta, &p.BlockWriteDelta,
			&sizeRW, &sizeRoot); err != nil {
			return nil, "", fmt.Errorf("scanning folded container sample: %w", err)
		}
		p.Resolution = "rollup"
		p.SizeRw = optionalInt(sizeRW)
		p.SizeRootFs = optionalInt(sizeRoot)
		if rowName.Valid && rowName.String != "" {
			name = rowName.String
		}
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("reading folded container samples: %w", err)
	}
	return points, name, nil
}

func (h *HistoryService) hostRawPoints(ctx context.Context, envID string, from, to time.Time) ([]MetricPoint, error) {
	folded := to.Sub(from) > rawResolutionLimit

	query := `
		SELECT timestamp, 1,
		       COALESCE(cpu_percent, 0), COALESCE(cpu_percent, 0),
		       COALESCE(memory_percent, 0),
		       COALESCE(memory_used, 0), COALESCE(memory_used, 0), COALESCE(memory_total, 0),
		       COALESCE(net_rx, 0), COALESCE(net_tx, 0),
		       COALESCE(block_read, 0), COALESCE(block_write, 0),
		       gpu_percent
		FROM host_metrics
		WHERE environment_id = ? AND timestamp >= ? AND timestamp < ?
		ORDER BY timestamp ASC
		LIMIT ?`

	if folded {
		b := bucketFor("timestamp")
		query = `
			SELECT {{b}} AS bucket_start, COUNT(*),
			       AVG(COALESCE(cpu_percent, 0)), MAX(COALESCE(cpu_percent, 0)),
			       AVG(COALESCE(memory_percent, 0)),
			       CAST(AVG(COALESCE(memory_used, 0)) AS INTEGER),
			       MAX(COALESCE(memory_used, 0)), MAX(COALESCE(memory_total, 0)),
			       MAX(COALESCE(net_rx, 0)), MAX(COALESCE(net_tx, 0)),
			       MAX(COALESCE(block_read, 0)), MAX(COALESCE(block_write, 0)),
			       AVG(gpu_percent)
			FROM host_metrics
			WHERE environment_id = ? AND timestamp >= ? AND timestamp < ?
			GROUP BY bucket_start
			ORDER BY bucket_start ASC
			LIMIT ?`
		query = strings.ReplaceAll(query, "{{b}}", b)
	}

	rows, err := h.db.QueryContext(ctx, query, envID, dbTime(from), dbTime(to), maxHistoryPoints)
	if err != nil {
		return nil, fmt.Errorf("querying host samples: %w", err)
	}
	defer closeRows(rows)

	var points []MetricPoint
	for rows.Next() {
		var p MetricPoint
		var gpuPct sql.NullFloat64
		if err := rows.Scan(&p.Timestamp, &p.Samples, &p.CPUPercent, &p.CPUMax,
			&p.MemPercent, &p.MemUsed, &p.MemUsedMax, &p.MemLimit,
			&p.NetRx, &p.NetTx, &p.BlockRead, &p.BlockWrite, &gpuPct); err != nil {
			return nil, fmt.Errorf("scanning host sample: %w", err)
		}
		p.Resolution = "raw"
		if folded {
			p.Resolution = "rollup"
		}
		p.GPUPercent = optionalFloat(gpuPct)
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading host samples: %w", err)
	}

	deriveDeltas(points)
	return points, nil
}

// containerIDsInWindow lists the containers that have any stored history in the
// window, checking both tiers so a container that stopped before the raw
// horizon is still reported.
//
// Grouping is by identity rather than by id, so a workload that was recreated
// during the window is one entry in a report rather than one per recreate. The
// id returned for each identity is the newest one seen, which is both the live
// container and the id ContainerHistory resolves the same identity back from.
func (h *HistoryService) containerIDsInWindow(ctx context.Context, envID string, from, to time.Time, limit int) ([]string, error) {
	id := coredb.ContainerIdentityExpr
	// ROW_NUMBER rather than a bare column beside MAX(): SQLite would resolve
	// the bare container_id from the max row, but that is a SQLite-only
	// guarantee and the other supported drivers reject the query outright.
	rows, err := h.db.QueryContext(ctx, expandSQL(`
		SELECT container_id FROM (
			SELECT container_id,
			       ROW_NUMBER() OVER (PARTITION BY identity ORDER BY seen DESC) AS rn,
			       MAX(seen) OVER (PARTITION BY identity) AS newest
			FROM (
				SELECT {{id}} AS identity, bucket_start AS seen, container_id
				FROM container_metrics_rollup
				WHERE environment_id = ? AND bucket_start >= ? AND bucket_start < ?
				UNION ALL
				SELECT {{id}} AS identity, sampled_at AS seen, container_id
				FROM container_metrics
				WHERE environment_id = ? AND sampled_at >= ? AND sampled_at < ?
			) seen_rows
		) ranked
		WHERE rn = 1
		ORDER BY newest DESC
		LIMIT ?`, "{{id}}", id),
		envID, dbTime(from), dbTime(to), envID, dbTime(from), dbTime(to), limit)
	if err != nil {
		return nil, fmt.Errorf("listing containers with history: %w", err)
	}
	defer closeRows(rows)

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning container id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading container ids: %w", err)
	}
	return ids, nil
}

// --- helpers ---------------------------------------------------------

// scopeFor resolves the stable identity of a container id from what has already
// been stored for it.
//
// Docker is deliberately not consulted: history answers from stored data alone
// so the graphs keep working when the daemon is down, and the collector has
// written a keyed row for any container it has seen since the last tick. In the
// ≤30s window between a container being created and that first tick, no keyed
// row exists yet and the scope falls back to the container id — which is what
// every one of these queries did before the key existed.
func (h *HistoryService) scopeFor(ctx context.Context, envID, containerID string) (coredb.ContainerScope, error) {
	var key, name sql.NullString
	err := h.db.QueryRowContext(ctx, `
		SELECT container_key, container_name FROM (
			SELECT container_key, container_name, sampled_at AS seen
			FROM container_metrics
			WHERE environment_id = ? AND container_id = ?
			UNION ALL
			SELECT container_key, container_name, bucket_start AS seen
			FROM container_metrics_rollup
			WHERE environment_id = ? AND container_id = ?
		) rows
		ORDER BY seen DESC
		LIMIT 1`,
		envID, containerID, envID, containerID,
	).Scan(&key, &name)
	if err != nil && err != sql.ErrNoRows {
		return coredb.ContainerScope{}, fmt.Errorf("resolving container identity: %w", err)
	}

	return coredb.NewContainerScope(key.String, name.String, containerID), nil
}

// resolveEnvID maps the empty "local" environment id used on the wire to the
// concrete row id the collector actually stored against. ParseEnvID yields ""
// for the default environment, but the metric tables carry a real foreign key,
// so querying with "" would silently match nothing.
func (h *HistoryService) resolveEnvID(ctx context.Context, envID string) string {
	if envID != "" {
		return envID
	}
	var id string
	if err := h.db.QueryRowContext(ctx,
		"SELECT id FROM environments WHERE is_default = 1 LIMIT 1").Scan(&id); err != nil {
		return envID
	}
	return id
}

// resolutionFor reports the grid the caller will actually receive, so a client
// does not have to infer it from the gap between two timestamps.
func resolutionFor(from, to time.Time) int {
	if to.Sub(from) > rawResolutionLimit {
		return metricsBucketSeconds
	}
	return 30
}

// mergeSeries concatenates the two tiers. Compaction deletes each raw row as it
// folds it, so the tiers cannot overlap and no de-duplication is needed; rollup
// is always the older half.
func mergeSeries(rollup, raw []MetricPoint) []MetricPoint {
	out := make([]MetricPoint, 0, len(rollup)+len(raw))
	out = append(out, rollup...)
	out = append(out, raw...)
	if len(out) > maxHistoryPoints {
		out = out[len(out)-maxHistoryPoints:]
	}
	return out
}

// finalize records what the series actually covers.
//
// CoveredFrom is deliberately the first stored point rather than the requested
// `from`: a caller asking for 30 days when only 6 exist must be able to tell,
// and a report that prints a period it does not have data for is worse than one
// that admits the shortfall.
func finalize(s *MetricSeries) {
	if len(s.Points) == 0 {
		return
	}
	s.CoveredFrom = s.Points[0].Timestamp
	s.CoveredTo = s.Points[len(s.Points)-1].Timestamp
}

// deriveDeltas fills the per-point counter movement for raw series. The first
// point has no predecessor and keeps a zero delta rather than inventing one
// from the container's whole lifetime.
func deriveDeltas(points []MetricPoint) {
	for i := 1; i < len(points); i++ {
		prev, cur := points[i-1], &points[i]
		cur.NetRxDelta = counterDelta(prev.NetRx, cur.NetRx)
		cur.NetTxDelta = counterDelta(prev.NetTx, cur.NetTx)
		cur.BlockReadDelta = counterDelta(prev.BlockRead, cur.BlockRead)
		cur.BlockWriteDelta = counterDelta(prev.BlockWrite, cur.BlockWrite)
	}
}

// dbTime renders a time in the layout the metric tables store, so SQL string
// comparison orders correctly.
func dbTime(t time.Time) string {
	return t.UTC().Format(dbTimeLayout)
}

// optionalInt maps a NULL aggregate to a nil pointer, keeping "no disk sample"
// distinct from "zero bytes" in the JSON.
func optionalInt(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	n := v.Int64
	return &n
}

// optionalFloat mirrors optionalInt for the nullable GPU column.
//
// nil and 0 mean different things here: nil is "this host reported no
// GPU telemetry", 0 is "the GPUs were idle". Collapsing them would draw
// a flat zero utilisation line on every host without a GPU.
func optionalFloat(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	value := v.Float64
	return &value
}
