// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rs/xid"
)

const (
	// metricsSampleInterval matches the host collector's tick, so a guest's
	// history has the same density as its host's.
	metricsSampleInterval = 30 * time.Second
	// metricsSweepTimeout bounds one sweep below the interval, so a wedged
	// agent cannot make two sweeps overlap.
	metricsSweepTimeout = 25 * time.Second
	// metricsRetention is how long raw samples are kept; the detail page shows
	// the last hour, the rest is there for a longer look back.
	metricsRetention = 24 * time.Hour
	// metricsPerEnvironment caps concurrent vm_stats calls to one host.
	metricsPerEnvironment = 4
	// metricsHistoryPoints is how many rates the history route returns: one
	// hour at the sample interval.
	metricsHistoryPoints = 120
)

// MetricsPoint is one interval of a guest's history: CPU and memory as sampled,
// disk and network as bytes per second since the previous sample.
type MetricsPoint struct {
	SampledAt      time.Time `json:"sampledAt"`
	CPUPercent     float64   `json:"cpuPercent"`
	MemoryPercent  float64   `json:"memoryPercent"`
	DiskReadRate   float64   `json:"diskReadRate"`
	DiskWriteRate  float64   `json:"diskWriteRate"`
	NetRxRate      float64   `json:"netRxRate"`
	NetTxRate      float64   `json:"netTxRate"`
	MemoryBytes    int64     `json:"memoryBytes"`
	MemoryMaxBytes int64     `json:"memoryMaxBytes"`
}

// storedSample is one raw row: the counters exactly as the agent reported them.
type storedSample struct {
	at                                time.Time
	cpu                               float64
	memory, memoryMax                 int64
	diskRead, diskWrite, netRx, netTx int64
}

// ensureVMMetricsSchema creates the history table on an installation whose
// migrations ran before it existed; plugin migrations only run at install.
func (s *Service) ensureVMMetricsSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS plugin_addon_kvm_vm_metrics (
		    id               TEXT PRIMARY KEY,
		    vm_id            TEXT NOT NULL,
		    environment_id   TEXT NOT NULL,
		    sampled_at       TEXT NOT NULL,
		    cpu_percent      REAL NOT NULL DEFAULT 0,
		    memory_bytes     INTEGER NOT NULL DEFAULT 0,
		    memory_max_bytes INTEGER NOT NULL DEFAULT 0,
		    disk_read_bytes  INTEGER NOT NULL DEFAULT 0,
		    disk_write_bytes INTEGER NOT NULL DEFAULT 0,
		    net_rx_bytes     INTEGER NOT NULL DEFAULT 0,
		    net_tx_bytes     INTEGER NOT NULL DEFAULT 0,
		    created_at       TEXT NOT NULL,
		    updated_at       TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_vm_metrics_vm_time
		    ON plugin_addon_kvm_vm_metrics(vm_id, sampled_at)`,
		`CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_vm_metrics_env
		    ON plugin_addon_kvm_vm_metrics(environment_id)`,
		`CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_vm_metrics_time
		    ON plugin_addon_kvm_vm_metrics(sampled_at)`,
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(ctx, statement); err != nil {
			return fmt.Errorf("preparing the guest metrics table: %w", err)
		}
	}
	return nil
}

// recordStats stores one sample of a guest.
func (s *Service) recordStats(ctx context.Context, envID string, st Stats) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_kvm_vm_metrics
			(id, vm_id, environment_id, sampled_at, cpu_percent, memory_bytes, memory_max_bytes,
			 disk_read_bytes, disk_write_bytes, net_rx_bytes, net_tx_bytes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		xid.New().String(), st.VMID, envID, st.SampledAt.UTC().Format(time.RFC3339),
		st.CPUPercent, st.MemoryBytes, st.MemoryMaxBytes,
		st.DiskReadBytes, st.DiskWriteBytes, st.NetRxBytes, st.NetTxBytes, now, now)
	if err != nil {
		return fmt.Errorf("recording guest metrics: %w", err)
	}
	return nil
}

// pruneStats drops samples older than the retention window.
func (s *Service) pruneStats(ctx context.Context, now time.Time) error {
	cutoff := now.Add(-metricsRetention).UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(ctx, `DELETE FROM plugin_addon_kvm_vm_metrics WHERE sampled_at < ?`, cutoff); err != nil {
		return fmt.Errorf("pruning guest metrics: %w", err)
	}
	return nil
}

// StatsHistory returns the guest's last hour as rates, oldest first. A guest
// that has never been sampled returns an empty list, not an error.
func (s *Service) StatsHistory(ctx context.Context, id string) ([]MetricsPoint, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `
		SELECT sampled_at, cpu_percent, memory_bytes, memory_max_bytes,
		       disk_read_bytes, disk_write_bytes, net_rx_bytes, net_tx_bytes
		FROM plugin_addon_kvm_vm_metrics
		WHERE vm_id = ?
		ORDER BY sampled_at DESC
		LIMIT ?`, id, metricsHistoryPoints+1)
	if err != nil {
		return nil, fmt.Errorf("reading guest metrics: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var samples []storedSample
	for rows.Next() {
		var sample storedSample
		var at string
		if err := rows.Scan(&at, &sample.cpu, &sample.memory, &sample.memoryMax,
			&sample.diskRead, &sample.diskWrite, &sample.netRx, &sample.netTx); err != nil {
			return nil, fmt.Errorf("scanning guest metrics: %w", err)
		}
		sample.at = parseTime(at)
		samples = append(samples, sample)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading guest metrics: %w", err)
	}
	for i, j := 0, len(samples)-1; i < j; i, j = i+1, j-1 {
		samples[i], samples[j] = samples[j], samples[i]
	}
	return metricsPoints(samples), nil
}

// memoryCeiling is the denominator for a guest's memory use. The agent samples
// from the host side and has no view of the configured size, so unless it sent
// one the ceiling comes from the record: what the guest holds now after hotplug,
// else its boot size.
func memoryCeiling(reported int64, vm VM) int64 {
	if reported > 0 {
		return reported
	}
	if vm.MemoryCurrentMB > 0 {
		return vm.MemoryCurrentMB * 1024 * 1024
	}
	return vm.MemoryMB * 1024 * 1024
}

// metricsPoints differences consecutive samples into rates. A counter that went
// backwards means the guest restarted, so that interval reads 0 rather than a
// negative spike; a pair with no time between them is skipped.
func metricsPoints(samples []storedSample) []MetricsPoint {
	out := make([]MetricsPoint, 0, len(samples))
	for i := 1; i < len(samples); i++ {
		prev, cur := samples[i-1], samples[i]
		seconds := cur.at.Sub(prev.at).Seconds()
		if seconds <= 0 {
			continue
		}
		rate := func(now, then int64) float64 {
			if now < then {
				return 0
			}
			return float64(now-then) / seconds
		}
		point := MetricsPoint{
			SampledAt:      cur.at,
			CPUPercent:     cur.cpu,
			DiskReadRate:   rate(cur.diskRead, prev.diskRead),
			DiskWriteRate:  rate(cur.diskWrite, prev.diskWrite),
			NetRxRate:      rate(cur.netRx, prev.netRx),
			NetTxRate:      rate(cur.netTx, prev.netTx),
			MemoryBytes:    cur.memory,
			MemoryMaxBytes: cur.memoryMax,
		}
		if cur.memoryMax > 0 {
			point.MemoryPercent = float64(cur.memory) / float64(cur.memoryMax) * 100
		}
		out = append(out, point)
	}
	return out
}

// metricsSampler records every running guest's stats on a fixed interval, owned
// by the plugin so Shutdown can stop it and wait.
type metricsSampler struct {
	svc      *Service
	logger   pluginLogger
	interval time.Duration

	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

func newMetricsSampler(svc *Service, logger pluginLogger) *metricsSampler {
	return &metricsSampler{svc: svc, logger: logger, interval: metricsSampleInterval, done: make(chan struct{})}
}

// Start runs the loop until Stop is called.
func (m *metricsSampler) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	// goroutine lifecycle: ends when Stop cancels ctx, and Stop waits on done.
	go func() {
		defer close(m.done)
		ticker := time.NewTicker(m.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sweepCtx, sweepCancel := context.WithTimeout(ctx, metricsSweepTimeout)
				m.sweep(sweepCtx)
				sweepCancel()
			}
		}
	}()
}

// Stop cancels an in-flight sweep and waits for the loop to exit; it is safe
// on a sampler that never started.
func (m *metricsSampler) Stop() {
	m.once.Do(func() {
		if m.cancel == nil {
			return
		}
		m.cancel()
		<-m.done
	})
}

// sweep samples every running guest, hosts in parallel and at most
// metricsPerEnvironment guests at a time on each, then prunes old rows.
func (m *metricsSampler) sweep(ctx context.Context) {
	guests, err := m.svc.List(ctx, "", false)
	if err != nil {
		m.logger.Warn("kvm: guest metrics could not list guests", "error", err)
		return
	}
	byEnv := map[string][]VM{}
	for _, vm := range guests {
		if vm.IsTemplate || !vm.HostAvailable || vm.State != StateRunning {
			continue
		}
		byEnv[vm.EnvironmentID] = append(byEnv[vm.EnvironmentID], vm)
	}

	var wg sync.WaitGroup
	for _, vms := range byEnv {
		wg.Add(1)
		go func(vms []VM) {
			defer wg.Done()
			slots := make(chan struct{}, metricsPerEnvironment)
			var envWG sync.WaitGroup
			for _, vm := range vms {
				select {
				case <-ctx.Done():
					envWG.Wait()
					return
				case slots <- struct{}{}:
				}
				envWG.Add(1)
				go func(vm VM) {
					defer envWG.Done()
					defer func() { <-slots }()
					m.sampleOne(ctx, vm)
				}(vm)
			}
			envWG.Wait()
		}(vms)
	}
	wg.Wait()

	if err := m.svc.pruneStats(ctx, time.Now()); err != nil {
		m.logger.Warn("kvm: pruning guest metrics failed", "error", err)
	}
}

// sampleOne records one guest. A guest that stopped between the list and the
// sample, or an agent too old to answer vm_stats, is skipped quietly: neither
// is a fault, and logging them would repeat every 30 seconds.
func (m *metricsSampler) sampleOne(ctx context.Context, vm VM) {
	st, err := m.svc.Stats(ctx, vm.ID)
	if err != nil {
		if !errors.Is(err, ErrNotRunning) && !errors.Is(err, ErrStatsUnavailable) {
			m.logger.Debug("kvm: sampling guest metrics failed", "vm", vm.ID, "error", err)
		}
		return
	}
	if err := m.svc.recordStats(ctx, vm.EnvironmentID, st); err != nil {
		m.logger.Warn("kvm: storing guest metrics failed", "vm", vm.ID, "error", err)
	}
}
