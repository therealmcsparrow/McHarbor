// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"

	"github.com/therealmcsparrow/mcharbor/core/docker"
)

// Service handles Docker metrics collection.
type Service struct {
	dockerPool *docker.ClientPool
}

// NewService creates a new metrics service.
func NewService(pool *docker.ClientPool) *Service {
	return &Service{dockerPool: pool}
}

// HostInfo returns host system info and disk usage for the given environment.
func (s *Service) HostInfo(ctx context.Context, envID string) (*HostMetricsResponse, error) {
	cli, err := s.dockerPool.Get(envID)
	if err != nil {
		return nil, fmt.Errorf("getting Docker client: %w", err)
	}

	infoCtx, infoCancel := context.WithTimeout(ctx, 30*time.Second)
	defer infoCancel()
	info, err := cli.Info(infoCtx)
	if err != nil {
		return nil, fmt.Errorf("fetching Docker info: %w", err)
	}

	duCtx, duCancel := context.WithTimeout(ctx, 30*time.Second)
	defer duCancel()
	du, err := cli.DiskUsage(duCtx, types.DiskUsageOptions{})
	if err != nil {
		return nil, fmt.Errorf("fetching disk usage: %w", err)
	}

	host := HostInfo{
		NCPU:          info.NCPU,
		MemTotal:      info.MemTotal,
		ServerVersion: info.ServerVersion,
		OS:            info.OperatingSystem,
		Architecture:  info.Architecture,
		KernelVersion: info.KernelVersion,
		Hostname:      info.Name,
		Uptime:        readUptime(),
		SystemTime:    info.SystemTime,
	}

	var imagesSize, containersSize, volumesSize, buildCacheSize int64
	for _, img := range du.Images {
		imagesSize += img.Size
	}
	for _, c := range du.Containers {
		containersSize += c.SizeRw
	}
	if du.Volumes != nil {
		for _, v := range du.Volumes {
			if v.UsageData.Size > 0 {
				volumesSize += v.UsageData.Size
			}
		}
	}
	for _, bc := range du.BuildCache {
		buildCacheSize += bc.Size
	}
	total := imagesSize + containersSize + volumesSize + buildCacheSize

	disk := DiskUsage{
		ImagesSize:     imagesSize,
		ContainersSize: containersSize,
		VolumesSize:    volumesSize,
		BuildCacheSize: buildCacheSize,
		Total:          total,
	}

	return &HostMetricsResponse{Host: host, Disk: disk}, nil
}

// AllContainerStats returns calculated stats for all running containers.
func (s *Service) AllContainerStats(ctx context.Context, envID string) ([]ContainerMetric, error) {
	cli, err := s.dockerPool.Get(envID)
	if err != nil {
		return nil, fmt.Errorf("getting Docker client: %w", err)
	}

	listCtx, listCancel := context.WithTimeout(ctx, 30*time.Second)
	defer listCancel()
	containers, err := cli.ContainerList(listCtx, container.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}

	type result struct {
		metric ContainerMetric
		ok     bool
	}
	results := make([]result, len(containers))

	var wg sync.WaitGroup
	for i, c := range containers {
		wg.Add(1)
		go func(idx int, ctr types.Container) {
			defer wg.Done()
			stat, err := getOneShot(ctx, cli, ctr.ID)
			if err != nil {
				return
			}
			name := ""
			if len(ctr.Names) > 0 {
				name = strings.TrimPrefix(ctr.Names[0], "/")
			}
			results[idx] = result{metric: buildMetric(ctr.ID, name, stat), ok: true}
		}(i, c)
	}
	wg.Wait()

	metrics := make([]ContainerMetric, 0, len(containers))
	for _, r := range results {
		if r.ok {
			metrics = append(metrics, r.metric)
		}
	}

	return metrics, nil
}

// ValidateContainerRunning checks that a container exists and is running.
// Returns the Docker SDK error directly (use client.IsErrNotFound to check for 404).
func (s *Service) ValidateContainerRunning(ctx context.Context, envID, containerID string) error {
	cli, err := s.dockerPool.Get(envID)
	if err != nil {
		return fmt.Errorf("getting Docker client: %w", err)
	}
	tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	info, err := cli.ContainerInspect(tctx, containerID)
	if err != nil {
		return err
	}
	if !info.State.Running {
		return fmt.Errorf("container is not running")
	}
	return nil
}

// StreamContainerStats opens a streaming stats connection and sends ContainerMetric on the channel.
// For agent environments, uses polling one-shot stats instead of streaming (streaming through
// the agent transport requires the agent to detect chunked encoding correctly).
func (s *Service) StreamContainerStats(ctx context.Context, envID, containerID string) (<-chan ContainerMetric, <-chan error) {
	metricsCh := make(chan ContainerMetric)
	errCh := make(chan error, 1)

	go func() {
		defer close(metricsCh)
		defer close(errCh)

		slog.Info("StreamContainerStats: starting", "env", envID, "container", containerID)

		cli, err := s.dockerPool.Get(envID)
		if err != nil {
			slog.Error("StreamContainerStats: get client failed", "env", envID, "error", err)
			errCh <- fmt.Errorf("getting Docker client: %w", err)
			return
		}

		// Get container name
		inspectCtx, inspectCancel := context.WithTimeout(ctx, 30*time.Second)
		inspect, err := cli.ContainerInspect(inspectCtx, containerID)
		inspectCancel()
		if err != nil {
			slog.Error("StreamContainerStats: inspect failed", "env", envID, "error", err)
			errCh <- fmt.Errorf("inspecting container: %w", err)
			return
		}
		name := strings.TrimPrefix(inspect.Name, "/")

		isAgent := s.dockerPool.IsAgentEnv(envID)
		slog.Info("StreamContainerStats: checked agent", "env", envID, "isAgent", isAgent)

		// Agent environments: poll one-shot stats (reliable through agent transport)
		if isAgent {
			s.pollContainerStats(ctx, cli, containerID, name, metricsCh, errCh)
			return
		}

		// Local/direct: use Docker streaming stats
		resp, err := cli.ContainerStats(ctx, containerID, true)
		if err != nil {
			errCh <- fmt.Errorf("opening stats stream: %w", err)
			return
		}
		defer resp.Body.Close()

		decoder := json.NewDecoder(resp.Body)
		for {
			var stat container.StatsResponse
			if err := decoder.Decode(&stat); err != nil {
				if ctx.Err() != nil {
					return
				}
				errCh <- fmt.Errorf("decoding stats: %w", err)
				return
			}

			m := buildMetric(containerID, name, &stat)
			select {
			case <-ctx.Done():
				return
			case metricsCh <- m:
			}
		}
	}()

	return metricsCh, errCh
}

// pollContainerStats polls one-shot stats for agent environments.
func (s *Service) pollContainerStats(ctx context.Context, cli *dockerclient.Client, containerID, name string, metricsCh chan<- ContainerMetric, errCh chan<- error) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Send first stat immediately
	stat, err := getOneShot(ctx, cli, containerID)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		slog.Error("pollContainerStats: first getOneShot failed", "container", containerID, "error", err)
		errCh <- fmt.Errorf("polling stats: %w", err)
		return
	}
	m := buildMetric(containerID, name, stat)
	slog.Info("pollContainerStats: sending metric", "container", containerID, "cpu", m.CPUPercent, "mem", m.MemUsage, "memLimit", m.MemLimit)
	select {
	case <-ctx.Done():
		return
	case metricsCh <- m:
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stat, err := getOneShot(ctx, cli, containerID)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				errCh <- fmt.Errorf("polling stats: %w", err)
				return
			}
			m := buildMetric(containerID, name, stat)
			select {
			case <-ctx.Done():
				return
			case metricsCh <- m:
			}
		}
	}
}

// getOneShot fetches a single stats snapshot (non-streaming).
func getOneShot(ctx context.Context, cli *dockerclient.Client, containerID string) (*container.StatsResponse, error) {
	statsCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := cli.ContainerStats(statsCtx, containerID, false)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var stat container.StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&stat); err != nil {
		return nil, err
	}
	return &stat, nil
}

// buildMetric calculates CPU%, memory%, net I/O, block I/O from raw Docker stats.
func buildMetric(id, name string, stat *container.StatsResponse) ContainerMetric {
	m := ContainerMetric{
		ID:   id,
		Name: name,
	}

	m.CPUPercent = calculateCPUPercent(stat)
	m.MemUsage = int64(stat.MemoryStats.Usage)
	m.MemLimit = int64(stat.MemoryStats.Limit)
	if stat.MemoryStats.Limit > 0 {
		m.MemPercent = float64(stat.MemoryStats.Usage) / float64(stat.MemoryStats.Limit) * 100.0
	}
	m.PIDs = stat.PidsStats.Current

	// Network I/O (sum across all interfaces)
	for _, net := range stat.Networks {
		m.NetRx += int64(net.RxBytes)
		m.NetTx += int64(net.TxBytes)
	}

	// Block I/O
	for _, bio := range stat.BlkioStats.IoServiceBytesRecursive {
		switch bio.Op {
		case "read", "Read":
			m.BlockRead += int64(bio.Value)
		case "write", "Write":
			m.BlockWrite += int64(bio.Value)
		}
	}

	return m
}

// calculateCPUPercent computes the CPU usage percentage from Docker stats.
func calculateCPUPercent(stat *container.StatsResponse) float64 {
	cpuDelta := float64(stat.CPUStats.CPUUsage.TotalUsage) - float64(stat.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stat.CPUStats.SystemUsage) - float64(stat.PreCPUStats.SystemUsage)

	if systemDelta > 0.0 && cpuDelta > 0.0 {
		return (cpuDelta / systemDelta) * float64(stat.CPUStats.OnlineCPUs) * 100.0
	}
	return 0.0
}

// readUptime reads the host uptime in seconds from /proc/uptime.
// Returns 0 if unavailable (non-Linux or read error).
func readUptime() int64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	return int64(secs)
}
