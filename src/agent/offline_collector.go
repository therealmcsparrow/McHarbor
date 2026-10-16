// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// Compose / Swarm labels that identify a workload across recreation. Mirrors
// core/docker/identity.go on the server so a sample buffered offline lands in
// the same series as the ones the server collected while online.
const (
	labelComposeProject = "com.docker.compose.project"
	labelComposeService = "com.docker.compose.service"
	labelComposeNumber  = "com.docker.compose.container-number"
	labelSwarmService   = "com.docker.swarm.service.name"
	labelSwarmSlot      = "com.docker.swarm.task.slot"
	labelSwarmTaskName  = "com.docker.swarm.task.name"
)

// statsConcurrency bounds how many one-shot stats calls run at once. A stats
// call with stream=false makes the daemon sample twice one second apart, so
// running them in parallel is what keeps a tick under a few seconds.
const statsConcurrency = 8

// OfflineBuffer owns the local store, the collector that fills it while the
// agent is disconnected, and the online/offline flag the log tee reads.
type OfflineBuffer struct {
	store    *OfflineStore
	proxy    *Proxy
	logger   *slog.Logger
	interval time.Duration

	// offline is true from the moment a connection attempt fails until the
	// next successful auth. The log tee consults it on every record.
	offline atomic.Bool

	mu          sync.Mutex
	collectStop context.CancelFunc
	collectDone chan struct{}
}

// NewOfflineBuffer wires the store to the Docker proxy.
func NewOfflineBuffer(store *OfflineStore, proxy *Proxy, interval time.Duration, logger *slog.Logger) *OfflineBuffer {
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}
	return &OfflineBuffer{store: store, proxy: proxy, logger: logger, interval: interval}
}

// Store exposes the buffer's SQLite store for the flush path.
func (b *OfflineBuffer) Store() *OfflineStore { return b.store }

// IsOffline reports whether the agent currently has no server connection.
func (b *OfflineBuffer) IsOffline() bool { return b.offline.Load() }

// MarkOffline flips the flag and starts collecting if not already running.
// Safe to call repeatedly: every failed connection attempt calls it.
func (b *OfflineBuffer) MarkOffline(ctx context.Context) {
	b.offline.Store(true)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.collectStop != nil {
		return
	}
	cctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	b.collectStop = cancel
	b.collectDone = done
	go b.collect(cctx, done)
	logs, events, metrics := b.store.Counts(ctx)
	b.logger.Info("offline buffering started",
		"path", b.store.Path(), "queuedLogs", logs, "queuedEvents", events, "queuedMetrics", metrics)
}

// MarkOnline flips the flag and stops the collector, blocking until its
// goroutines have exited so nothing is written behind a running flush.
func (b *OfflineBuffer) MarkOnline() {
	b.offline.Store(false)
	b.mu.Lock()
	stop, done := b.collectStop, b.collectDone
	b.collectStop, b.collectDone = nil, nil
	b.mu.Unlock()
	if stop == nil {
		return
	}
	stop()
	<-done
	b.logger.Info("offline buffering stopped")
}

// Close stops any collector and closes the store.
func (b *OfflineBuffer) Close() {
	b.MarkOnline()
	if err := b.store.Close(); err != nil {
		b.logger.Warn("closing offline store failed", "error", err)
	}
}

func (b *OfflineBuffer) collect(ctx context.Context, done chan struct{}) {
	defer close(done)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		b.streamEvents(ctx)
	}()
	go func() {
		defer wg.Done()
		b.sampleMetrics(ctx)
	}()
	wg.Wait()
}

// ─── Docker events ────────────────────────────────────────────────────

// dockerEvent is the subset of the daemon's event message the buffer keeps.
type dockerEvent struct {
	Type   string `json:"Type"`
	Action string `json:"Action"`
	Actor  struct {
		ID         string            `json:"ID"`
		Attributes map[string]string `json:"Attributes"`
	} `json:"Actor"`
	Time     int64 `json:"time"`
	TimeNano int64 `json:"timeNano"`
}

// streamEvents follows GET /events and buffers every container, image, volume
// and network event until ctx is cancelled. The stream is re-opened from the
// last seen timestamp after any error so a daemon restart costs nothing.
func (b *OfflineBuffer) streamEvents(ctx context.Context) {
	since := time.Now()
	for {
		last, err := b.streamEventsOnce(ctx, since)
		if ctx.Err() != nil {
			return
		}
		if !last.IsZero() {
			since = last
		}
		b.logger.Debug("offline event stream ended, reopening", "error", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

func (b *OfflineBuffer) streamEventsOnce(ctx context.Context, since time.Time) (time.Time, error) {
	q := url.Values{}
	q.Set("since", strconv.FormatInt(since.Unix(), 10))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/events?"+q.Encode(), nil)
	if err != nil {
		return time.Time{}, fmt.Errorf("building events request: %w", err)
	}
	resp, err := b.proxy.httpClient.Do(req)
	if err != nil {
		return time.Time{}, fmt.Errorf("opening events stream: %w", err)
	}
	defer func() { errlog.Log(resp.Body.Close()) }()
	if resp.StatusCode >= 300 {
		body := errlog.ReadAll(io.LimitReader(resp.Body, 2048))
		return time.Time{}, fmt.Errorf("events stream returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var last time.Time
	dec := json.NewDecoder(bufio.NewReaderSize(resp.Body, 64*1024))
	trimTick := 0
	for {
		var ev dockerEvent
		if err := dec.Decode(&ev); err != nil {
			return last, fmt.Errorf("decoding event: %w", err)
		}
		ts := time.Unix(0, ev.TimeNano)
		if ev.TimeNano == 0 {
			ts = time.Unix(ev.Time, 0)
		}
		last = ts

		switch ev.Type {
		case "container", "image", "volume", "network":
		default:
			continue
		}
		// Healthcheck exec churn is the large majority of events on a host
		// that uses healthchecks and the server records health separately, so
		// it never earns a slot in the bounded buffer.
		if strings.HasPrefix(ev.Action, "exec_") {
			continue
		}

		entry := OfflineEventEntry{
			Type:       ev.Type,
			Action:     ev.Action,
			ActorID:    ev.Actor.ID,
			ActorName:  ev.Actor.Attributes["name"],
			Attributes: ev.Actor.Attributes,
			Timestamp:  ts.UTC().Format(time.RFC3339),
		}
		if err := b.store.AddEvent(ctx, entry); err != nil {
			b.logger.Warn("offline event not buffered", "error", err)
		}
		trimTick++
		if trimTick%500 == 0 {
			b.store.Trim(ctx)
		}
	}
}

// ─── Container stats ──────────────────────────────────────────────────

type dockerContainerListItem struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	Labels map[string]string `json:"Labels"`
}

// dockerStats mirrors the fields of the daemon's stats response the server's
// metrics service reads (container.StatsResponse), so the derived numbers match.
type dockerStats struct {
	CPUStats    dockerCPUStats `json:"cpu_stats"`
	PreCPUStats dockerCPUStats `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64 `json:"usage"`
		Limit uint64 `json:"limit"`
	} `json:"memory_stats"`
	PidsStats struct {
		Current uint64 `json:"current"`
	} `json:"pids_stats"`
	Networks map[string]struct {
		RxBytes uint64 `json:"rx_bytes"`
		TxBytes uint64 `json:"tx_bytes"`
	} `json:"networks"`
	BlkioStats struct {
		IoServiceBytesRecursive []struct {
			Op    string `json:"op"`
			Value uint64 `json:"value"`
		} `json:"io_service_bytes_recursive"`
	} `json:"blkio_stats"`
}

type dockerCPUStats struct {
	CPUUsage struct {
		TotalUsage uint64 `json:"total_usage"`
	} `json:"cpu_usage"`
	SystemUsage uint64 `json:"system_cpu_usage"`
	OnlineCPUs  uint32 `json:"online_cpus"`
}

// sampleMetrics takes one stats sample of every running container per interval.
func (b *OfflineBuffer) sampleMetrics(ctx context.Context) {
	// First tick shortly after going offline so a brief outage still leaves a
	// data point, then on the configured cadence.
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		b.sampleOnce(ctx)
		timer.Reset(b.interval)
	}
}

func (b *OfflineBuffer) sampleOnce(ctx context.Context) {
	tickCtx, cancel := context.WithTimeout(ctx, b.interval)
	defer cancel()

	containers, err := b.listRunningContainers(tickCtx)
	if err != nil {
		b.logger.Debug("offline metrics tick skipped", "error", err)
		return
	}
	if len(containers) == 0 {
		return
	}

	sampledAt := time.Now().UTC().Format(time.RFC3339)
	entries := make([]OfflineMetricEntry, len(containers))
	ok := make([]bool, len(containers))

	sem := make(chan struct{}, statsConcurrency)
	var wg sync.WaitGroup
	for i, c := range containers {
		wg.Add(1)
		go func(idx int, ctr dockerContainerListItem) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			stat, err := b.oneShotStats(tickCtx, ctr.ID)
			if err != nil {
				return
			}
			name := ""
			if len(ctr.Names) > 0 {
				name = strings.TrimPrefix(ctr.Names[0], "/")
			}
			entries[idx] = buildOfflineMetric(ctr.ID, name, containerKey(ctr.Labels, name), stat, sampledAt)
			ok[idx] = true
		}(i, c)
	}
	wg.Wait()

	samples := make([]OfflineMetricEntry, 0, len(entries))
	for i := range entries {
		if ok[i] {
			samples = append(samples, entries[i])
		}
	}
	if err := b.store.AddMetrics(ctx, samples); err != nil {
		b.logger.Warn("offline metrics not buffered", "error", err)
		return
	}
	b.store.Trim(ctx)
}

func (b *OfflineBuffer) listRunningContainers(ctx context.Context) ([]dockerContainerListItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/containers/json", nil)
	if err != nil {
		return nil, fmt.Errorf("building container list request: %w", err)
	}
	resp, err := b.proxy.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}
	defer func() { errlog.Log(resp.Body.Close()) }()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("listing containers returned status %d", resp.StatusCode)
	}
	var items []dockerContainerListItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("decoding container list: %w", err)
	}
	return items, nil
}

// oneShotStats asks for stream=false, which makes the daemon return one
// document holding both the current and the previous CPU sample — the pair
// the CPU percentage is computed from. one-shot=true would omit precpu_stats
// and every sample would read 0% CPU.
func (b *OfflineBuffer) oneShotStats(ctx context.Context, containerID string) (*dockerStats, error) {
	statsCtx, cancel := context.WithTimeout(ctx, hostOpTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(statsCtx, http.MethodGet,
		"http://docker/containers/"+url.PathEscape(containerID)+"/stats?stream=false", nil)
	if err != nil {
		return nil, fmt.Errorf("building stats request: %w", err)
	}
	resp, err := b.proxy.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reading stats for %s: %w", containerID, err)
	}
	defer func() { errlog.Log(resp.Body.Close()) }()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("stats for %s returned status %d", containerID, resp.StatusCode)
	}
	var stat dockerStats
	if err := json.NewDecoder(resp.Body).Decode(&stat); err != nil {
		return nil, fmt.Errorf("decoding stats for %s: %w", containerID, err)
	}
	return &stat, nil
}

// buildOfflineMetric mirrors metrics.buildMetric + calculateCPUPercent on the
// server so offline samples are directly comparable with online ones.
func buildOfflineMetric(id, name, key string, stat *dockerStats, sampledAt string) OfflineMetricEntry {
	m := OfflineMetricEntry{
		ContainerID:   id,
		ContainerName: name,
		ContainerKey:  key,
		MemUsed:       int64(stat.MemoryStats.Usage),
		MemLimit:      int64(stat.MemoryStats.Limit),
		PIDs:          int64(stat.PidsStats.Current),
		SampledAt:     sampledAt,
	}
	cpuDelta := float64(stat.CPUStats.CPUUsage.TotalUsage) - float64(stat.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stat.CPUStats.SystemUsage) - float64(stat.PreCPUStats.SystemUsage)
	if systemDelta > 0 && cpuDelta > 0 {
		m.CPUPercent = (cpuDelta / systemDelta) * float64(stat.CPUStats.OnlineCPUs) * 100.0
	}
	for _, n := range stat.Networks {
		m.NetRx += int64(n.RxBytes)
		m.NetTx += int64(n.TxBytes)
	}
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

// containerKey mirrors docker.ContainerKey on the server.
func containerKey(labels map[string]string, name string) string {
	project := strings.TrimSpace(labels[labelComposeProject])
	service := strings.TrimSpace(labels[labelComposeService])
	if project != "" && service != "" {
		key := "compose:" + project + "/" + service
		if number := strings.TrimSpace(labels[labelComposeNumber]); number != "" {
			key += "/" + number
		}
		return key
	}
	if swarmService := strings.TrimSpace(labels[labelSwarmService]); swarmService != "" {
		if slot := strings.TrimSpace(labels[labelSwarmSlot]); slot != "" {
			return "swarm:" + swarmService + "/" + slot
		}
		if task := strings.TrimSpace(labels[labelSwarmTaskName]); task != "" {
			return "swarm:" + swarmService + "/" + task
		}
		return "swarm:" + swarmService
	}
	if trimmed := strings.TrimSpace(strings.TrimPrefix(name, "/")); trimmed != "" {
		return "name:" + trimmed
	}
	return ""
}
