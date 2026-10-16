// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/capacity"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/forecast"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
)

// DefaultIdleDays is how long a container must have been stopped before it is
// offered as a reclaim candidate. A fortnight is long enough that a weekend
// maintenance window does not put a container on the list.
const DefaultIdleDays = 14

// maxIdleDays bounds the caller-supplied threshold so a query cannot be asked to
// scan an unbounded history.
const maxIdleDays = 365

// quietCPUPercent is the CPU average below which a running container counts as
// doing nothing. It is not zero: a container that wakes for a healthcheck reports
// a fraction of a percent, and calling that busy would empty the list.
const quietCPUPercent = 0.05

// quietWindowCoverage is how much of the window a container must have been
// observed across to count as quiet for it. Nine tenths tolerates a restart of
// McHarbor or a missed sample without admitting a container that only appeared
// yesterday.
const quietWindowCoverage = 0.9

// CapacityService reads the retained capacity series and the idle-resource
// candidates. It writes nothing and deletes nothing — the reclaim action is the
// existing prune, under the existing key.
type CapacityService struct {
	db   *sql.DB
	pool *docker.ClientPool
}

// NewCapacityService returns a service reading db, using pool only to tell an
// agent environment from a local one.
func NewCapacityService(db *sql.DB, pool *docker.ClientPool) *CapacityService {
	return &CapacityService{db: db, pool: pool}
}

// Forecasts projects every capacity subject for one environment, or for all of
// them when envID is empty.
//
// The window is the configured rollup retention, not a parameter: the series is
// pruned at it, so accepting a larger one from a caller would promise history
// that was already deleted.
func (s *CapacityService) Forecasts(ctx context.Context, envID string) (*CapacityReport, error) {
	window := coreSettings.ReadRetentionSettingsContext(ctx, s.db).MetricsRollupDays
	subjects, err := capacity.Projections(ctx, s.db, envID, window, time.Now())
	if err != nil {
		return nil, err
	}
	return &CapacityReport{
		Subjects:     subjects,
		WindowDays:   window,
		RequiredDays: forecast.MinDays,
		AmberPercent: forecast.AmberPercent,
		RedPercent:   forecast.RedPercent,
		BucketHours:  float64(capacity.BucketSeconds) / 3600,
	}, nil
}

// Idle lists containers that have been stopped, or running with no CPU and no
// network, for longer than days.
//
// Both signals come from retained history rather than a live ContainerList: a
// live list knows a container is stopped and not for how long, which is the only
// thing that makes it a reclaim candidate rather than a container someone will
// start in a minute.
func (s *CapacityService) Idle(ctx context.Context, envID string, days int) (*IdleReport, error) {
	if days < 1 {
		days = DefaultIdleDays
	}
	if days > maxIdleDays {
		days = maxIdleDays
	}
	now := time.Now().UTC()
	report := &IdleReport{Containers: []IdleResource{}, ThresholdDays: days}

	sizes, sizesKnown, err := s.latestSizes(ctx, envID)
	if err != nil {
		return nil, err
	}
	report.SizesUnavailable = !sizesKnown

	stopped, err := s.stoppedFor(ctx, envID, days, now)
	if err != nil {
		return nil, err
	}
	quiet, err := s.quietFor(ctx, envID, days, now)
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	for _, group := range [][]IdleResource{stopped, quiet} {
		for _, row := range group {
			if seen[row.ContainerID] {
				continue
			}
			seen[row.ContainerID] = true
			if size, ok := sizes[row.ContainerID]; ok {
				row.SizeKnown = true
				row.SizeRwBytes = size.rw
				row.SizeRootBytes = size.root
			} else {
				row.SizeReason = s.sizeReason(ctx, row.EnvironmentID)
			}
			report.Containers = append(report.Containers, row)
		}
	}
	return report, nil
}

// sizeReason names why a candidate has no size, so the row can say it instead of
// showing 0 B.
func (s *CapacityService) sizeReason(ctx context.Context, envID string) string {
	if s.pool != nil && envID != "" && s.pool.IsAgentEnvContext(ctx, envID) {
		return "agent_environment"
	}
	return "not_sampled"
}

type containerSizeRow struct {
	rw   int64
	root int64
}

// latestSizes reads the newest non-NULL writable-layer size per container.
//
// The second return value is false when the whole environment has no size
// samples at all, which is the state every agent environment is permanently in:
// the collector skips Size:true for agents because the daemon would walk every
// container's diff directory and the result would then cross a WebSocket, and a
// pushed sample carries the two columns as NULL.
func (s *CapacityService) latestSizes(ctx context.Context, envID string) (map[string]containerSizeRow, bool, error) {
	where := "size_rw_avg IS NOT NULL"
	args := []any{}
	if envID != "" {
		where += " AND environment_id = ?"
		args = append(args, envID)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT container_id, size_rw_avg, COALESCE(size_root_fs_avg, 0), bucket_start
		 FROM container_metrics_rollup WHERE `+where+` ORDER BY bucket_start LIMIT 100000`, args...)
	if err != nil {
		return nil, false, fmt.Errorf("reading container sizes: %w", err)
	}
	defer closeRows(rows)
	out := map[string]containerSizeRow{}
	for rows.Next() {
		var id, at string
		var rw, root int64
		if err := rows.Scan(&id, &rw, &root, &at); err != nil {
			return nil, false, fmt.Errorf("scanning container size: %w", err)
		}
		// Ordered oldest first, so the last write per container wins.
		out[id] = containerSizeRow{rw: rw, root: root}
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	return out, len(out) > 0, nil
}

// stoppedFor lists containers whose last running observation is strictly older
// than the cutoff.
//
// Strictly: a container that stopped exactly `days` ago has reached the threshold
// but not passed it, and offering it for deletion on the boundary is how an
// off-by-one becomes a deleted container.
func (s *CapacityService) stoppedFor(ctx context.Context, envID string, days int, now time.Time) ([]IdleResource, error) {
	cutoff := now.AddDate(0, 0, -days).Format(capacitySampleLayout)
	where := "1 = 1"
	args := []any{}
	if envID != "" {
		where += " AND environment_id = ?"
		args = append(args, envID)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT COALESCE(environment_id, ''), container_id, COALESCE(MAX(container_name), ''),
		        MAX(sampled_at) AS newest,
		        MAX(CASE WHEN running = 1 THEN sampled_at END) AS last_running,
		        MAX(CASE WHEN running = 1 THEN 1 ELSE 0 END) AS ever_ran,
		        MIN(sampled_at) AS oldest
		 FROM container_state_samples WHERE `+where+`
		 GROUP BY environment_id, container_id LIMIT 5000`, args...)
	if err != nil {
		return nil, fmt.Errorf("reading container state history: %w", err)
	}
	defer closeRows(rows)

	out := []IdleResource{}
	for rows.Next() {
		var env, id, name, newest, oldest string
		var lastRunning sql.NullString
		var everRan int
		if err := rows.Scan(&env, &id, &name, &newest, &lastRunning, &everRan, &oldest); err != nil {
			return nil, fmt.Errorf("scanning container state history: %w", err)
		}
		// A container whose newest sample is itself older than the cutoff has
		// left the history, not gone idle — it was removed, and offering a
		// removed container as a reclaim candidate is noise.
		if newest < cutoff {
			continue
		}
		// Still running as of its last sample.
		if lastRunning.Valid && lastRunning.String == newest {
			continue
		}
		since := oldest
		if lastRunning.Valid {
			since = lastRunning.String
		}
		if since >= cutoff {
			continue
		}
		at, err := time.Parse(capacitySampleLayout, since)
		if err != nil {
			continue
		}
		out = append(out, IdleResource{
			EnvironmentID: env, ContainerID: id, ContainerName: name,
			Reason: "stopped", IdleDays: forecast.Round(now.Sub(at).Hours()/24, 1),
		})
	}
	return out, rows.Err()
}

// quietFor lists containers that ran throughout the window and moved no traffic
// and almost no CPU.
//
// This is the weaker of the two signals and is labelled differently everywhere it
// surfaces. It also requires the container to have been observed across
// substantially the whole window, so one quiet bucket from a container started
// yesterday is not "quiet for 14 days".
//
// "Substantially" and not "exactly": the window is the query's own cutoff, so the
// widest span any row can show is the threshold itself, and a single missed sample
// at either end would otherwise disqualify a container that was idle throughout.
func (s *CapacityService) quietFor(ctx context.Context, envID string, days int, now time.Time) ([]IdleResource, error) {
	cutoff := now.AddDate(0, 0, -days).Format(capacitySampleLayout)
	where := "bucket_start >= ?"
	args := []any{cutoff}
	if envID != "" {
		where += " AND environment_id = ?"
		args = append(args, envID)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT COALESCE(environment_id, ''), container_id, COALESCE(MAX(container_name), ''),
		        MAX(cpu_avg), SUM(net_rx_delta) + SUM(net_tx_delta), MIN(bucket_start), MAX(bucket_start)
		 FROM container_metrics_rollup WHERE `+where+`
		 GROUP BY environment_id, container_id LIMIT 5000`, args...)
	if err != nil {
		return nil, fmt.Errorf("reading container metrics history: %w", err)
	}
	defer closeRows(rows)

	out := []IdleResource{}
	for rows.Next() {
		var env, id, name, oldest, newest string
		var cpuMax float64
		var netTotal int64
		if err := rows.Scan(&env, &id, &name, &cpuMax, &netTotal, &oldest, &newest); err != nil {
			return nil, fmt.Errorf("scanning container metrics history: %w", err)
		}
		if cpuMax > quietCPUPercent || netTotal > 0 {
			continue
		}
		first, err1 := time.Parse(capacitySampleLayout, oldest)
		last, err2 := time.Parse(capacitySampleLayout, newest)
		if err1 != nil || err2 != nil {
			continue
		}
		span := last.Sub(first).Hours() / 24
		if span < float64(days)*quietWindowCoverage {
			continue
		}
		out = append(out, IdleResource{
			EnvironmentID: env, ContainerID: id, ContainerName: name,
			Reason: "quiet", IdleDays: forecast.Round(span, 1),
		})
	}
	return out, rows.Err()
}

// capacitySampleLayout is the timestamp format every metrics table stores, so a
// string comparison against a cutoff orders correctly without parsing.
const capacitySampleLayout = "2006-01-02 15:04:05"
