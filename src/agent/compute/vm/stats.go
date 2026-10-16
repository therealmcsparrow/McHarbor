// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package vm

import (
	"context"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/supervisor"
)

// Stats is one sample of a running guest's resource use.
//
// Every field is a host-side observation. Nothing here comes from inside the
// guest, so none of it depends on a guest agent being installed.
type Stats struct {
	VMID           string
	CPUPercent     float64
	CPUUsageNSec   int64
	MemoryBytes    int64
	BalloonBytes   int64
	DiskReadBytes  int64
	DiskWriteBytes int64
	NetRxBytes     int64
	NetTxBytes     int64
	SampledAt      time.Time
}

// cpuSample is the previous CPU reading kept so a percentage can be derived.
type cpuSample struct {
	usageNSec uint64
	at        time.Time
}

// Stats samples one guest: CPU and memory from the unit's cgroup accounting,
// disk and balloon over QMP, and network from the tap counters.
//
// A guest that is not running is an error rather than a zero sample, because a
// zero sample is indistinguishable from an idle guest and the server would
// render it as one.
func (m *Manager) Stats(ctx context.Context, vmID string) (Stats, error) {
	status, err := m.sup.Status(ctx, supervisor.VMUnitName(vmID))
	if err != nil {
		return Stats{}, err
	}
	if !status.Running() {
		return Stats{}, ErrNotRunning
	}

	now := time.Now().UTC()
	out := Stats{
		VMID:         vmID,
		CPUUsageNSec: int64(status.CPUUsageNSec),
		MemoryBytes:  int64(status.MemoryCurrent),
		SampledAt:    now,
	}
	out.CPUPercent = m.cpuPercent(vmID, status.CPUUsageNSec, now)

	// QMP is best-effort: a guest whose monitor is briefly unavailable should
	// still report the CPU and memory the supervisor already gave us, rather
	// than failing the whole sample.
	if monitor, err := m.Monitor(ctx, vmID); err == nil {
		if blocks, err := monitor.BlockStats(ctx); err == nil {
			for _, device := range blocks {
				out.DiskReadBytes += device.Stats.ReadBytes
				out.DiskWriteBytes += device.Stats.WriteBytes
			}
		}
		if balloon, err := monitor.Balloon(ctx); err == nil {
			out.BalloonBytes = balloon.Actual
		}
	}

	out.NetRxBytes, out.NetTxBytes = tapCounters(vmID)
	return out, nil
}

// cpuPercent turns the unit's monotonically increasing CPU time into a
// percentage, against the wall time since the previous sample.
//
// The first sample for a guest has nothing to compare against and reports 0
// rather than a spike computed from the guest's whole lifetime — which is what
// dividing by uptime would give on a guest that was busy an hour ago.
//
// The result is not clamped to 100: a guest with four vCPUs can legitimately
// use 400%, and clamping it here would hide exactly the saturation an operator
// is looking for.
func (m *Manager) cpuPercent(vmID string, usageNSec uint64, now time.Time) float64 {
	m.statsMu.Lock()
	defer m.statsMu.Unlock()

	previous, seen := m.cpuSamples[vmID]
	m.cpuSamples[vmID] = cpuSample{usageNSec: usageNSec, at: now}
	if !seen {
		return 0
	}

	elapsed := now.Sub(previous.at)
	// A counter that went backwards means the guest restarted under the same
	// id; treat it as a first sample rather than reporting a negative rate.
	if elapsed <= 0 || usageNSec < previous.usageNSec {
		return 0
	}
	return float64(usageNSec-previous.usageNSec) / float64(elapsed.Nanoseconds()) * 100
}

// forgetCPUSample drops a guest's cached CPU reading so a later start computes
// its first percentage from scratch instead of against a stale counter.
func (m *Manager) forgetCPUSample(vmID string) {
	m.statsMu.Lock()
	delete(m.cpuSamples, vmID)
	m.statsMu.Unlock()
}
