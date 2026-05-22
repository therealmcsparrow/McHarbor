// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package dashboard

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	dockerclient "github.com/docker/docker/client"

	"github.com/therealmcsparrow/mcharbor/core/docker"
)

// TimeValue is a single data point for time series charts.
type TimeValue struct {
	Timestamp string  `json:"timestamp"`
	Value     float64 `json:"value"`
}

// Stats holds overall Docker resource counts.
type Stats struct {
	Containers       ContainerStats `json:"containers"`
	Images           int            `json:"images"`
	Volumes          int            `json:"volumes"`
	Networks         int            `json:"networks"`
	Stacks           int            `json:"stacks"`
	CPUHistory       []TimeValue    `json:"cpuHistory,omitempty"`
	MemoryHistory    []TimeValue    `json:"memoryHistory,omitempty"`
	NetworkRxHistory []TimeValue    `json:"networkRxHistory,omitempty"`
	NetworkTxHistory []TimeValue    `json:"networkTxHistory,omitempty"`
	BlockReadHistory []TimeValue    `json:"blockReadHistory,omitempty"`
	BlockWriteHistory []TimeValue   `json:"blockWriteHistory,omitempty"`
}

// ContainerStats breaks down container counts by state.
type ContainerStats struct {
	Total   int `json:"total"`
	Running int `json:"running"`
	Stopped int `json:"stopped"`
	Paused  int `json:"paused"`
}

// Metric represents a single host metrics data point.
type Metric struct {
	ID            string `json:"id"`
	EnvironmentID string `json:"environmentId"`
	CPUPercent    *int   `json:"cpuPercent,omitempty"`
	MemoryPercent *int   `json:"memoryPercent,omitempty"`
	MemoryUsed   *int64 `json:"memoryUsed,omitempty"`
	MemoryTotal  *int64 `json:"memoryTotal,omitempty"`
	NetRx         *int64 `json:"netRx,omitempty"`
	NetTx         *int64 `json:"netTx,omitempty"`
	BlockRead     *int64 `json:"blockRead,omitempty"`
	BlockWrite    *int64 `json:"blockWrite,omitempty"`
	Timestamp    string `json:"timestamp"`
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
	cli, err := s.dockerPool.Get(envID)
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
	imgCtx, imgCancel := context.WithTimeout(ctx, 30*time.Second)
	defer imgCancel()
	imgs, err := cli.ImageList(imgCtx, image.ListOptions{})
	if err == nil {
		stats.Images = len(imgs)
	}

	// Volume count
	volCtx, volCancel := context.WithTimeout(ctx, 30*time.Second)
	defer volCancel()
	volResp, err := cli.VolumeList(volCtx, volume.ListOptions{})
	if err == nil {
		stats.Volumes = len(volResp.Volumes)
	}

	// Network count
	netCtx, netCancel := context.WithTimeout(ctx, 30*time.Second)
	defer netCancel()
	nets, err := cli.NetworkList(netCtx, network.ListOptions{})
	if err == nil {
		stats.Networks = len(nets)
	}

	// Stack count from DB
	var stackCount int
	s.db.QueryRow("SELECT COUNT(*) FROM stacks").Scan(&stackCount)
	stats.Stacks = stackCount

	// Load recent host metrics for charts
	history := s.getMetricHistory(envID)
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
func (s *Service) getMetricHistory(envID string) metricHistory {
	dbEnvID := envID
	if dbEnvID == "" {
		if err := s.db.QueryRow("SELECT id FROM environments WHERE is_default = 1 LIMIT 1").Scan(&dbEnvID); err != nil || dbEnvID == "" {
			dbEnvID = "local"
		}
	}

	rows, err := s.db.Query(
		`SELECT cpu_percent, memory_used, net_rx, net_tx, block_read, block_write, timestamp
		 FROM host_metrics
		 WHERE environment_id = ?
		 ORDER BY timestamp DESC LIMIT 60`, dbEnvID)
	if err != nil {
		return metricHistory{}
	}
	defer rows.Close()

	var h metricHistory
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
		if netRx.Valid {
			h.netRx = append(h.netRx, TimeValue{Timestamp: label, Value: float64(netRx.Int64)})
		}
		if netTx.Valid {
			h.netTx = append(h.netTx, TimeValue{Timestamp: label, Value: float64(netTx.Int64)})
		}
		if blockRead.Valid {
			h.blockRead = append(h.blockRead, TimeValue{Timestamp: label, Value: float64(blockRead.Int64)})
		}
		if blockWrite.Valid {
			h.blockWrite = append(h.blockWrite, TimeValue{Timestamp: label, Value: float64(blockWrite.Int64)})
		}
	}

	// Reverse so oldest is first (for left-to-right chart rendering)
	reverseTimeValues(h.cpu)
	reverseTimeValues(h.memory)
	reverseTimeValues(h.netRx)
	reverseTimeValues(h.netTx)
	reverseTimeValues(h.blockRead)
	reverseTimeValues(h.blockWrite)

	return h
}

func reverseTimeValues(s []TimeValue) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

// Metrics returns historical host metrics from the DB.
func (s *Service) Metrics(envID string, limit int) ([]Metric, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	query := `
		SELECT id, environment_id, cpu_percent, memory_percent, memory_used, memory_total,
		       net_rx, net_tx, block_read, block_write, timestamp
		FROM host_metrics
	`
	args := []any{}

	if envID != "" {
		query += " WHERE environment_id = ?"
		args = append(args, envID)
	}

	query += " ORDER BY timestamp DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying metrics: %w", err)
	}
	defer rows.Close()

	var metrics []Metric
	for rows.Next() {
		var m Metric
		var cpuPct, memPct sql.NullInt64
		var memUsed, memTotal sql.NullInt64
		var netRx, netTx, blockRead, blockWrite sql.NullInt64

		err := rows.Scan(&m.ID, &m.EnvironmentID, &cpuPct, &memPct, &memUsed, &memTotal,
			&netRx, &netTx, &blockRead, &blockWrite, &m.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("scanning metric: %w", err)
		}

		if cpuPct.Valid {
			v := int(cpuPct.Int64)
			m.CPUPercent = &v
		}
		if memPct.Valid {
			v := int(memPct.Int64)
			m.MemoryPercent = &v
		}
		if memUsed.Valid {
			m.MemoryUsed = &memUsed.Int64
		}
		if memTotal.Valid {
			m.MemoryTotal = &memTotal.Int64
		}
		if netRx.Valid {
			m.NetRx = &netRx.Int64
		}
		if netTx.Valid {
			m.NetTx = &netTx.Int64
		}
		if blockRead.Valid {
			m.BlockRead = &blockRead.Int64
		}
		if blockWrite.Valid {
			m.BlockWrite = &blockWrite.Int64
		}

		metrics = append(metrics, m)
	}
	if metrics == nil {
		metrics = []Metric{}
	}
	return metrics, rows.Err()
}

// getContainerStats retrieves container counts broken down by state.
func (s *Service) getContainerStats(ctx context.Context, cli *dockerclient.Client) (ContainerStats, error) {
	tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	containers, err := cli.ContainerList(tctx, container.ListOptions{All: true})
	if err != nil {
		return ContainerStats{}, err
	}

	stats := ContainerStats{Total: len(containers)}
	for _, c := range containers {
		switch c.State {
		case "running":
			stats.Running++
		case "paused":
			stats.Paused++
		default:
			stats.Stopped++
		}
	}
	return stats, nil
}
