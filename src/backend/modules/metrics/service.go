// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/api/types/container"
	dockerclient "github.com/moby/moby/client"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/docker"
)

// fullDiskUsage asks for every object type with its items, because the totals
// below are summed from the items and API 1.52+ only sends them when verbose.
var fullDiskUsage = dockerclient.DiskUsageOptions{Containers: true, Images: true, Volumes: true, BuildCache: true, Verbose: true}

// Service handles Docker metrics collection.
type Service struct {
	dockerPool *docker.ClientPool
	agentPool  *coreagent.AgentPool
}

// NewService creates a new metrics service.
func NewService(pool *docker.ClientPool) *Service {
	return &Service{dockerPool: pool}
}

// NewServiceWithAgent creates a metrics service that can also drive prune and
// host-stats operations against connected remote agents.
func NewServiceWithAgent(pool *docker.ClientPool, agentPool *coreagent.AgentPool) *Service {
	return &Service{dockerPool: pool, agentPool: agentPool}
}

// HostInfo returns host system info and disk usage for the given environment.
func (s *Service) HostInfo(ctx context.Context, envID string) (*HostMetricsResponse, error) {
	cli, err := s.dockerPool.GetContext(ctx, envID)
	if err != nil {
		return nil, fmt.Errorf("getting Docker client: %w", err)
	}

	infoCtx, infoCancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer infoCancel()
	infoRes, err := cli.Info(infoCtx, dockerclient.InfoOptions{})
	info := infoRes.Info
	if err != nil {
		return nil, fmt.Errorf("fetching Docker info: %w", err)
	}

	du, err := sharedDiskUsage.get(ctx, envID, func(ctx context.Context) (dockerclient.DiskUsageResult, error) {
		duCtx, duCancel := context.WithTimeout(ctx, docker.CallTimeout)
		defer duCancel()
		return cli.DiskUsage(duCtx, fullDiskUsage)
	})
	if err != nil {
		return nil, fmt.Errorf("fetching disk usage: %w", err)
	}

	agentLimit := s.dockerPool.IsAgentEnvContext(ctx, envID)

	host := HostInfo{
		NCPU:          info.NCPU,
		MemTotal:      info.MemTotal,
		ServerVersion: info.ServerVersion,
		OS:            info.OperatingSystem,
		Architecture:  info.Architecture,
		KernelVersion: info.KernelVersion,
		Hostname:      info.Name,
		SystemTime:    info.SystemTime,
	}

	// Live host metrics (CPU/mem/load/uptime) come from /proc. For local
	// environments we read it on the McHarbor process. For agent
	// environments we ask the agent to read it on the remote host
	// (agent >= 1.6.0). Older agents fall back to the agentLimit=true
	// state and the UI shows the "host metrics unavailable" notice.
	if !agentLimit {
		host.Uptime = readUptime()
		cpu, memUsed, load1, load5, load15, ok := readHostStats()
		if ok {
			host.CPUPercent = cpu
			host.MemUsed = int64(memUsed)
			if host.MemTotal > 0 {
				host.MemPercent = float64(memUsed) / float64(host.MemTotal) * 100.0
			}
			host.Load1 = load1
			host.Load5 = load5
			host.Load15 = load15
		}
	} else if s.agentPool != nil {
		if t := s.agentPool.ResolveTransport(envID); t != nil && t.HostStatsSupported() {
			statsCtx, statsCancel := context.WithTimeout(ctx, 15*time.Second)
			stats, supported, err := t.HostStats(statsCtx)
			statsCancel()
			if err == nil && supported {
				host.CPUPercent = stats.CPUPercent
				host.MemUsed = stats.MemUsed
				if host.MemTotal > 0 {
					host.MemPercent = float64(stats.MemUsed) / float64(host.MemTotal) * 100.0
				}
				host.Load1 = stats.Load1
				host.Load5 = stats.Load5
				host.Load15 = stats.Load15
				host.Uptime = stats.Uptime
				agentLimit = false
			}
		}
	}

	var imagesSize, containersSize, volumesSize, buildCacheSize int64
	for _, img := range du.Images.Items {
		imagesSize += img.Size
	}
	for _, c := range du.Containers.Items {
		containersSize += c.SizeRw
	}
	for _, v := range du.Volumes.Items {
		if v.UsageData != nil && v.UsageData.Size > 0 {
			volumesSize += v.UsageData.Size
		}
	}
	for _, bc := range du.BuildCache.Items {
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

	// Host filesystem usage — best-effort. For local envs we read it on
	// the McHarbor process; for agent envs the agent reports it
	// together with /proc stats above.
	hostFS := HostFSUsage{Path: "/"}
	if !agentLimit {
		if totalBytes, usedBytes, ok := readRootFSUsage(); ok {
			hostFS.Total = totalBytes
			hostFS.Used = usedBytes
			if totalBytes > 0 {
				hostFS.Percent = float64(usedBytes) / float64(totalBytes) * 100.0
			}
		}
	} else if s.agentPool != nil {
		if t := s.agentPool.ResolveTransport(envID); t != nil && t.HostStatsSupported() {
			statsCtx, statsCancel := context.WithTimeout(ctx, 15*time.Second)
			stats, supported, err := t.HostStats(statsCtx)
			statsCancel()
			if err == nil && supported {
				hostFS.Total = stats.FSTotal
				hostFS.Used = stats.FSUsed
				if stats.FSTotal > 0 {
					hostFS.Percent = float64(stats.FSUsed) / float64(stats.FSTotal) * 100.0
				}
			}
		}
	}

	return &HostMetricsResponse{Host: host, Disk: disk, HostFS: hostFS, AgentLimit: agentLimit}, nil
}

// Prune runs the requested Docker prune operation. The Confirm flag is
// required to actually perform a destructive prune; the request is rejected
// otherwise. Volume prune is only allowed for "system" and "volumes" types
// and requires Volumes=true to opt in (so we never delete named volumes by
// accident). For agent environments, the prune is forwarded to the agent
// (requires agent >= 1.6.0).
func (s *Service) Prune(ctx context.Context, envID string, req PruneRequest) (*PruneResult, error) {
	if !req.Confirm {
		return nil, fmt.Errorf("confirm flag is required to run a prune")
	}

	if s.dockerPool.IsAgentEnvContext(ctx, envID) {
		if s.agentPool == nil {
			return nil, fmt.Errorf("metrics service is not wired with an agent pool")
		}
		t := s.agentPool.ResolveTransport(envID)
		if t == nil {
			return nil, fmt.Errorf("agent is not connected for environment %s", envID)
		}
		if !t.PruneSupported() {
			return nil, fmt.Errorf("prune requires agent 1.6.0+; please upgrade mcharbor-agent")
		}
		pruneCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		result, err := t.Prune(pruneCtx, coreagent.PrunePayload{
			Type:    req.Type,
			Volumes: req.Volumes,
		})
		if err != nil {
			return nil, err
		}
		return &PruneResult{
			Type:           result.Type,
			ItemsDeleted:   result.ItemsDeleted,
			SpaceReclaimed: result.SpaceReclaimed,
		}, nil
	}

	cli, err := s.dockerPool.GetContext(ctx, envID)
	if err != nil {
		return nil, fmt.Errorf("getting Docker client: %w", err)
	}

	timeout, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	switch req.Type {
	case "system":
		// The Go Docker SDK does not expose a single "system prune" call
		// like `docker system prune`. We emulate it by running the
		// per-resource prunes in sequence. When Volumes=true, named
		// volumes are also pruned (the Volumes flag is the explicit
		// opt-in the user must consciously set).
		var (
			totalItems   int64
			totalReclaim int64
		)
		imgRes, err := cli.ImagePrune(timeout, dockerclient.ImagePruneOptions{})
		if err != nil {
			return nil, fmt.Errorf("system prune (images): %w", err)
		}
		img := imgRes.Report
		totalItems += int64(len(img.ImagesDeleted))
		totalReclaim += int64(img.SpaceReclaimed)

		ctnRes, err := cli.ContainerPrune(timeout, dockerclient.ContainerPruneOptions{})
		if err != nil {
			return nil, fmt.Errorf("system prune (containers): %w", err)
		}
		ctn := ctnRes.Report
		totalItems += int64(len(ctn.ContainersDeleted))
		totalReclaim += int64(ctn.SpaceReclaimed)

		netRes, err := cli.NetworkPrune(timeout, dockerclient.NetworkPruneOptions{})
		if err != nil {
			return nil, fmt.Errorf("system prune (networks): %w", err)
		}
		net := netRes.Report
		totalItems += int64(len(net.NetworksDeleted))

		if req.Volumes {
			volRes, err := cli.VolumePrune(timeout, dockerclient.VolumePruneOptions{})
			if err != nil {
				return nil, fmt.Errorf("system prune (volumes): %w", err)
			}
			vol := volRes.Report
			totalItems += int64(len(vol.VolumesDeleted))
			totalReclaim += int64(vol.SpaceReclaimed)
		}

		return &PruneResult{
			Type:           "system",
			ItemsDeleted:   totalItems,
			SpaceReclaimed: totalReclaim,
		}, nil
	case "builder":
		pruned, err := cli.BuildCachePrune(timeout, dockerclient.BuildCachePruneOptions{
			All:     false,
			Filters: dockerclient.Filters{},
		})
		if err != nil {
			return nil, fmt.Errorf("builder prune: %w", err)
		}
		return &PruneResult{
			Type:           "builder",
			ItemsDeleted:   int64(len(pruned.Report.CachesDeleted)),
			SpaceReclaimed: int64(pruned.Report.SpaceReclaimed),
		}, nil
	case "volumes":
		reportRes, err := cli.VolumePrune(timeout, dockerclient.VolumePruneOptions{})
		if err != nil {
			return nil, fmt.Errorf("volume prune: %w", err)
		}
		report := reportRes.Report
		return &PruneResult{
			Type:           "volumes",
			ItemsDeleted:   int64(len(report.VolumesDeleted)),
			SpaceReclaimed: int64(report.SpaceReclaimed),
		}, nil
	case "images":
		reportRes, err := cli.ImagePrune(timeout, dockerclient.ImagePruneOptions{})
		if err != nil {
			return nil, fmt.Errorf("image prune: %w", err)
		}
		report := reportRes.Report
		return &PruneResult{
			Type:           "images",
			ItemsDeleted:   int64(len(report.ImagesDeleted)),
			SpaceReclaimed: int64(report.SpaceReclaimed),
		}, nil
	case "containers":
		reportRes, err := cli.ContainerPrune(timeout, dockerclient.ContainerPruneOptions{})
		if err != nil {
			return nil, fmt.Errorf("container prune: %w", err)
		}
		report := reportRes.Report
		return &PruneResult{
			Type:           "containers",
			ItemsDeleted:   int64(len(report.ContainersDeleted)),
			SpaceReclaimed: int64(report.SpaceReclaimed),
		}, nil
	case "networks":
		reportRes, err := cli.NetworkPrune(timeout, dockerclient.NetworkPruneOptions{})
		if err != nil {
			return nil, fmt.Errorf("network prune: %w", err)
		}
		report := reportRes.Report
		return &PruneResult{
			Type:           "networks",
			ItemsDeleted:   int64(len(report.NetworksDeleted)),
			SpaceReclaimed: 0,
		}, nil
	default:
		return nil, fmt.Errorf("unknown prune type %q", req.Type)
	}
}

// readHostStats and readRootFSUsage live in host_stats_linux.go (build tag
// linux) and host_stats_other.go (build tag !linux). They return ok=false on
// non-Linux platforms so the service reports zeros and the UI can show a
// "host metrics unavailable" state.

// statsFanoutLocal and statsFanoutAgent bound how many one-shot stats calls
// AllContainerStats has outstanding at once. Concurrency is the point - a
// sequential loop over an agent environment costs ~2s per container and would
// never finish inside the collector's deadline - but unbounded concurrency is
// not. Every agent request shares one WebSocket, so a host with fifty
// containers used to put fifty round trips on that socket at once, and each
// one still outstanding when the connection dropped produced its own
// cancellation. The agent limit stays well under the local one for that reason.
const (
	statsFanoutLocal = 32
	statsFanoutAgent = 8
)

// AllContainerStats returns calculated stats for all running containers.
func (s *Service) AllContainerStats(ctx context.Context, envID string) ([]ContainerMetric, error) {
	cli, err := s.dockerPool.GetContext(ctx, envID)
	if err != nil {
		return nil, fmt.Errorf("getting Docker client: %w", err)
	}

	listCtx, listCancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer listCancel()
	containersRes, err := cli.ContainerList(listCtx, dockerclient.ContainerListOptions{})
	containers := containersRes.Items
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}

	type result struct {
		metric ContainerMetric
		ok     bool
	}
	results := make([]result, len(containers))

	fanout := statsFanoutLocal
	if s.dockerPool.IsAgentEnvContext(ctx, envID) {
		fanout = statsFanoutAgent
	}
	sem := make(chan struct{}, fanout)

	var wg sync.WaitGroup
	for i, c := range containers {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, ctr container.Summary) {
			defer wg.Done()
			defer func() { <-sem }()
			stat, err := getOneShot(ctx, cli, ctr.ID)
			if err != nil {
				return
			}
			name := ""
			if len(ctr.Names) > 0 {
				name = strings.TrimPrefix(ctr.Names[0], "/")
			}
			metric := buildMetric(ctr.ID, name, stat)
			// The labels are only available here, on the list result. The
			// collector writes this alongside the id so the stored series
			// survives the container being recreated.
			metric.Key = docker.ContainerKey(ctr.Labels, name)
			results[idx] = result{metric: metric, ok: true}
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
	cli, err := s.dockerPool.GetContext(ctx, envID)
	if err != nil {
		return fmt.Errorf("getting Docker client: %w", err)
	}
	tctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()
	infoRes, err := cli.ContainerInspect(tctx, containerID, dockerclient.ContainerInspectOptions{})
	if err != nil {
		return err
	}
	info := infoRes.Container
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

		cli, err := s.dockerPool.GetContext(ctx, envID)
		if err != nil {
			slog.Error("StreamContainerStats: get client failed", "env", envID, "error", err)
			errCh <- fmt.Errorf("getting Docker client: %w", err)
			return
		}

		// Get container name
		inspectCtx, inspectCancel := context.WithTimeout(ctx, docker.CallTimeout)
		inspectRes, err := cli.ContainerInspect(inspectCtx, containerID, dockerclient.ContainerInspectOptions{})
		inspect := inspectRes.Container
		inspectCancel()
		if err != nil {
			slog.Error("StreamContainerStats: inspect failed", "env", envID, "error", err)
			errCh <- fmt.Errorf("inspecting container: %w", err)
			return
		}
		name := strings.TrimPrefix(inspect.Name, "/")

		isAgent := s.dockerPool.IsAgentEnvContext(ctx, envID)
		slog.Info("StreamContainerStats: checked agent", "env", envID, "isAgent", isAgent)

		// Agent environments: poll one-shot stats (reliable through agent transport)
		if isAgent {
			s.pollContainerStats(ctx, cli, containerID, name, metricsCh, errCh)
			return
		}

		// Local/direct: use Docker streaming stats
		resp, err := cli.ContainerStats(ctx, containerID, dockerclient.ContainerStatsOptions{Stream: true})
		if err != nil {
			errCh <- fmt.Errorf("opening stats stream: %w", err)
			return
		}
		defer closeBody(resp.Body)

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
	statsCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	resp, err := cli.ContainerStats(statsCtx, containerID, dockerclient.ContainerStatsOptions{Stream: false, IncludePreviousSample: true})
	if err != nil {
		return nil, err
	}
	defer closeBody(resp.Body)

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

// readUptime lives in host_stats_linux.go (build tag linux) and
// host_stats_other.go (build tag !linux). It returns 0 on non-Linux
// platforms.
