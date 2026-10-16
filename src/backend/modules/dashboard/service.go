// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package dashboard

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"

	dockerclient "github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/docker"
)

// TimeValue is a single data point for time series charts.
type TimeValue struct {
	Timestamp string  `json:"timestamp"`
	Value     float64 `json:"value"`
}

// Stats holds overall Docker resource counts.
type Stats struct {
	Containers        ContainerStats `json:"containers"`
	Images            int            `json:"images"`
	Volumes           int            `json:"volumes"`
	Networks          int            `json:"networks"`
	Stacks            int            `json:"stacks"`
	CPUHistory        []TimeValue    `json:"cpuHistory,omitempty"`
	MemoryHistory     []TimeValue    `json:"memoryHistory,omitempty"`
	NetworkRxHistory  []TimeValue    `json:"networkRxHistory,omitempty"`
	NetworkTxHistory  []TimeValue    `json:"networkTxHistory,omitempty"`
	BlockReadHistory  []TimeValue    `json:"blockReadHistory,omitempty"`
	BlockWriteHistory []TimeValue    `json:"blockWriteHistory,omitempty"`
}

// Summary is the estate-wide count behind the dashboard's page head.
//
// Every figure but Environments is a pointer, because a caller who may not read
// a resource must see the segment absent rather than reported as zero.
type Summary struct {
	Environments int  `json:"environments"`
	Containers   *int `json:"containers,omitempty"`
	Stacks       *int `json:"stacks,omitempty"`
	// Unreachable is how many of Environments did not answer, so a smaller
	// container count is never mistaken for a quieter estate.
	Unreachable int `json:"unreachable,omitempty"`
}

// ContainerStats breaks down container counts by state.
type ContainerStats struct {
	Total      int `json:"total"`
	Running    int `json:"running"`
	Stopped    int `json:"stopped"`
	Paused     int `json:"paused"`
	Restarting int `json:"restarting"`
	Killed     int `json:"killed"`
}

// Metric represents a single host metrics data point.
type Metric struct {
	ID            string `json:"id"`
	EnvironmentID string `json:"environmentId"`
	CPUPercent    *int   `json:"cpuPercent,omitempty"`
	MemoryPercent *int   `json:"memoryPercent,omitempty"`
	MemoryUsed    *int64 `json:"memoryUsed,omitempty"`
	MemoryTotal   *int64 `json:"memoryTotal,omitempty"`
	NetRx         *int64 `json:"netRx,omitempty"`
	NetTx         *int64 `json:"netTx,omitempty"`
	BlockRead     *int64 `json:"blockRead,omitempty"`
	BlockWrite    *int64 `json:"blockWrite,omitempty"`
	Timestamp     string `json:"timestamp"`
}

// Service handles dashboard data aggregation.
type Service struct {
	db         *sql.DB
	dockerPool *docker.ClientPool
}

// NewService creates a new dashboard service.
func NewService(db *sql.DB, pool *docker.ClientPool) *Service {
	return &Service{db: db, dockerPool: pool}
}

// Stats fetches overall Docker resource counts for the given environment.
func (s *Service) Stats(ctx context.Context, envID string) (*Stats, error) {
	cli, err := s.dockerPool.GetContext(ctx, envID)
	if err != nil {
		return nil, fmt.Errorf("getting Docker client: %w", err)
	}

	stats := &Stats{}

	// Container counts
	containerStats, err := s.getContainerStats(ctx, cli)
	if err == nil {
		stats.Containers = containerStats
	}

	// Image count
	imgCtx, imgCancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer imgCancel()
	imgs, err := cli.ImageList(imgCtx, dockerclient.ImageListOptions{})
	if err == nil {
		stats.Images = len(imgs.Items)
	}

	// Volume count
	volCtx, volCancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer volCancel()
	vols, err := cli.VolumeList(volCtx, dockerclient.VolumeListOptions{})
	if err == nil {
		stats.Volumes = len(vols.Items)
	}

	// Network count
	netCtx, netCancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer netCancel()
	nets, err := cli.NetworkList(netCtx, dockerclient.NetworkListOptions{})
	if err == nil {
		stats.Networks = len(nets.Items)
	}

	stats.Stacks = s.stackCount(ctx, envID)

	// Load recent host metrics for charts
	history := s.getMetricHistory(ctx, envID)
	if len(history.cpu) > 0 {
		stats.CPUHistory = history.cpu
	}
	if len(history.memory) > 0 {
		stats.MemoryHistory = history.memory
	}
	if len(history.netRx) > 0 {
		stats.NetworkRxHistory = history.netRx
	}
	if len(history.netTx) > 0 {
		stats.NetworkTxHistory = history.netTx
	}
	if len(history.blockRead) > 0 {
		stats.BlockReadHistory = history.blockRead
	}
	if len(history.blockWrite) > 0 {
		stats.BlockWriteHistory = history.blockWrite
	}

	return stats, nil
}

// metricHistory holds all time series slices returned by getMetricHistory.
type metricHistory struct {
	cpu        []TimeValue
	memory     []TimeValue
	netRx      []TimeValue
	netTx      []TimeValue
	blockRead  []TimeValue
	blockWrite []TimeValue
}

// getMetricHistory returns the last 60 data points for all metric dimensions.
// Network and block I/O are stored as cumulative counters and served as bytes
// per second, so 61 rows are read to yield 60 rates.
func (s *Service) getMetricHistory(ctx context.Context, envID string) metricHistory {
	dbEnvID := envID
	if dbEnvID == "" {
		if err := s.db.QueryRowContext(ctx, "SELECT id FROM environments WHERE is_default = 1 LIMIT 1").Scan(&dbEnvID); err != nil || dbEnvID == "" {
			dbEnvID = "local"
		}
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT cpu_percent, memory_used, net_rx, net_tx, block_read, block_write, timestamp
		 FROM host_metrics
		 WHERE environment_id = ?
		 ORDER BY timestamp DESC LIMIT 61`, dbEnvID)
	if err != nil {
		return metricHistory{}
	}
	defer func() { db.CloseRows(rows) }()

	var h metricHistory
	var io []ioSample
	for rows.Next() {
		var cpuPct, memUsed, netRx, netTx, blockRead, blockWrite sql.NullInt64
		var ts string
		if err := rows.Scan(&cpuPct, &memUsed, &netRx, &netTx, &blockRead, &blockWrite, &ts); err != nil {
			continue
		}
		// Format timestamp for display (HH:MM)
		label := ts
		if len(ts) >= 16 {
			label = ts[11:16]
		}
		if cpuPct.Valid {
			h.cpu = append(h.cpu, TimeValue{Timestamp: label, Value: float64(cpuPct.Int64)})
		}
		if memUsed.Valid {
			h.memory = append(h.memory, TimeValue{Timestamp: label, Value: float64(memUsed.Int64)})
		}
		io = append(io, ioSample{at: ts, label: label, netRx: netRx, netTx: netTx, blockRead: blockRead, blockWrite: blockWrite})
	}
	if err := rows.Err(); err != nil {
		slog.Warn("dashboard: reading metric history failed, charting the rows read so far", "env", dbEnvID, "error", err)
	}

	// Reverse so oldest is first (for left-to-right chart rendering)
	reverseTimeValues(h.cpu)
	reverseTimeValues(h.memory)
	h.cpu, h.memory = lastN(h.cpu, historyPoints), lastN(h.memory, historyPoints)
	slices.Reverse(io)
	h.netRx, h.netTx, h.blockRead, h.blockWrite = ioRates(io)

	return h
}

func reverseTimeValues(s []TimeValue) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

// Metrics returns historical host metrics from the DB.
// getContainerStats retrieves container counts broken down by state.
// stackCount returns how many stacks belong to an environment.
//
// Scoped, unlike the query this replaced: every other figure on this endpoint
// comes from the selected environment's Docker daemon, so an unscoped count
// made the dashboard and the sidebar disagree with the Stacks page itself.
// A row with no environment_id is a discovered stack that was never attached
// to one, and it is visible from every environment, so it counts in each.
//
// Returns zero on failure rather than an error: a dashboard that renders with
// one figure missing is better than one that refuses to render.
func (s *Service) stackCount(ctx context.Context, envID string) int {
	query := "SELECT COUNT(*) FROM stacks"
	args := []any{}
	if envID != "" {
		query += " WHERE environment_id = ? OR environment_id IS NULL"
		args = append(args, envID)
	}

	var count int
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		slog.Warn("dashboard: counting stacks failed", "env", envID, "error", err)
		return 0
	}
	return count
}

func (s *Service) getContainerStats(ctx context.Context, cli *dockerclient.Client) (ContainerStats, error) {
	tctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()
	containersRes, err := cli.ContainerList(tctx, dockerclient.ContainerListOptions{All: true})
	if err != nil {
		return ContainerStats{}, err
	}
	containers := containersRes.Items

	stats := ContainerStats{Total: len(containers)}
	for _, c := range containers {
		switch c.State {
		case "running":
			stats.Running++
		case "paused":
			stats.Paused++
		case "restarting":
			stats.Restarting++
		case "dead":
			stats.Killed++
		default:
			stats.Stopped++
		}
	}
	return stats, nil
}

// Summary counts what the whole estate holds, for the dashboard's page head.
//
// Only envIDs the caller may read are passed in; an environment that cannot be
// reached is counted in Unreachable rather than failing the whole response.
func (s *Service) Summary(ctx context.Context, envIDs []string, wantContainers, wantStacks bool) Summary {
	out := Summary{Environments: len(envIDs)}

	if wantContainers {
		containers, unreachable := s.containerTotal(ctx, envIDs)
		out.Containers = &containers
		out.Unreachable = unreachable
	}

	if wantStacks {
		stacks := s.stackTotal(ctx, envIDs)
		out.Stacks = &stacks
	}

	return out
}

// containerTotal lists every environment concurrently and returns the total
// plus how many hosts did not answer. Sequentially this costs seconds per
// agent-backed host, which is the whole page-head budget.
func (s *Service) containerTotal(ctx context.Context, envIDs []string) (int, int) {
	var (
		mu          sync.Mutex
		total       int
		unreachable int
		wg          sync.WaitGroup
	)

	for _, envID := range envIDs {
		wg.Add(1)
		go func(envID string) {
			defer wg.Done()

			cli, err := s.dockerPool.GetContext(ctx, envID)
			if err != nil {
				mu.Lock()
				unreachable++
				mu.Unlock()
				return
			}

			tctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
			defer cancel()

			listRes, err := cli.ContainerList(tctx, dockerclient.ContainerListOptions{All: true})
			if err != nil {
				slog.Warn("dashboard: counting containers failed", "env", envID, "error", err)
				mu.Lock()
				unreachable++
				mu.Unlock()
				return
			}
			list := listRes.Items

			mu.Lock()
			total += len(list)
			mu.Unlock()
		}(envID)
	}

	wg.Wait()
	return total, unreachable
}

// stackTotal counts stacks across envIDs, including the rows no environment
// claims, which predate the column and would otherwise vanish from the total.
func (s *Service) stackTotal(ctx context.Context, envIDs []string) int {
	if len(envIDs) == 0 {
		return 0
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(envIDs)), ",")
	args := make([]any, 0, len(envIDs))
	for _, id := range envIDs {
		args = append(args, id)
	}

	var count int
	query := "SELECT COUNT(*) FROM stacks WHERE environment_id IN (" + placeholders + ") OR environment_id IS NULL"
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		slog.Warn("dashboard: counting stacks failed", "error", err)
		return 0
	}
	return count
}

// EnvironmentIDs returns every environment id, oldest first, for the caller to
// filter by permission before anything is counted.
func (s *Service) EnvironmentIDs(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM environments ORDER BY created_at LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing environments: %w", err)
	}
	defer func() { db.CloseRows(rows) }()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning environment id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading environments: %w", err)
	}
	return ids, nil
}
