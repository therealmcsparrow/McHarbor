// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lxc

import (
	"math"
	"strconv"
	"strings"
	"time"
)

// Stats is one sample of a container's cgroup and network counters.
type Stats struct {
	ContainerID    string
	CPUUsageNSec   uint64
	CPUPercent     float64
	MemoryBytes    uint64
	MemoryMaxBytes uint64
	// MemoryHighBytes is the memory.high throttle actually in force, 0 when none.
	MemoryHighBytes uint64
	// SwapBytes is swap alone. libcontainer's SwapUsage adds memory to it for
	// cgroup v1 compatibility, which made a container with no swap report its
	// whole memory as swap.
	SwapBytes    uint64
	PidsCurrent  uint64
	PidsMax      uint64
	IOReadBytes  uint64
	IOWriteBytes uint64
	NetRxBytes   uint64
	NetTxBytes   uint64
	// RootfsUsedBytes is the space the rootfs occupies, from a bounded walk that
	// is cached rather than repeated on every sample.
	RootfsUsedBytes uint64
	SampledAt       time.Time
}

// cpuSample is one reading of a container's cumulative CPU time.
type cpuSample struct {
	usageNSec uint64
	at        time.Time
}

// cpuSampleMaxAge is how old a previous reading may be and still give a
// percentage that describes "now" rather than an average since long ago.
const cpuSampleMaxAge = time.Minute

// usableCPUSample reports whether prev can serve as the baseline for cur.
func usableCPUSample(prev, cur cpuSample) bool {
	if prev.at.IsZero() || !cur.at.After(prev.at) {
		return false
	}
	return cur.at.Sub(prev.at) <= cpuSampleMaxAge && cur.usageNSec >= prev.usageNSec
}

// cpuPercent is the share of the container's CPU allowance used between two
// readings, 0-100, so a container pinned at its limit reads 100 whatever the limit.
func cpuPercent(prev, cur cpuSample, allowance float64) float64 {
	if !usableCPUSample(prev, cur) || allowance <= 0 {
		return 0
	}
	wall := float64(cur.at.Sub(prev.at).Nanoseconds())
	used := float64(cur.usageNSec - prev.usageNSec)
	percent := used / (wall * allowance) * 100
	return math.Min(100, math.Max(0, percent))
}

// cpuAllowance is how many CPUs a container may use: its cpu.max quota when one
// is set, otherwise every CPU on the host.
func cpuAllowance(quota int64, period uint64, hostCPUs int) float64 {
	if quota > 0 && period > 0 {
		return float64(quota) / float64(period)
	}
	if hostCPUs <= 0 {
		return 1
	}
	return float64(hostCPUs)
}

// unlimitedAsZero maps cgroup v2's "max", which libcontainer reports as
// MaxUint64, to zero: the wire's word for "no limit".
func unlimitedAsZero(limit uint64) uint64 {
	if limit >= math.MaxInt64 {
		return 0
	}
	return limit
}

// parseNetDev sums received and transmitted bytes over every interface in a
// /proc/<pid>/net/dev table except loopback, which is traffic that never left.
func parseNetDev(body string) (rx, tx uint64) {
	for _, line := range strings.Split(body, "\n") {
		name, counters, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "" || name == "lo" || strings.Contains(name, "|") {
			continue
		}
		fields := strings.Fields(counters)
		if len(fields) < 9 {
			continue
		}
		received, errRx := strconv.ParseUint(fields[0], 10, 64)
		sent, errTx := strconv.ParseUint(fields[8], 10, 64)
		if errRx != nil || errTx != nil {
			continue
		}
		rx += received
		tx += sent
	}
	return rx, tx
}

// parseCgroupLimit reads a cgroup v2 limit file such as memory.high, where
// "max" means no limit and maps to zero, the wire's word for "none".
func parseCgroupLimit(body string) uint64 {
	value := strings.TrimSpace(body)
	if value == "" || value == "max" {
		return 0
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0
	}
	return unlimitedAsZero(parsed)
}

// reportedMemoryHigh is the memory.high to report: the configured value when the
// kernel holds it, which it does rounded down to a whole page, and the value in
// force otherwise — a throttle that was never applied, or was changed outside
// McHarbor, is reported as it actually is.
func reportedMemoryHigh(configured, inForce uint64, pageSize int) uint64 {
	if configured == 0 || inForce == 0 || pageSize <= 0 {
		return inForce
	}
	if configured >= inForce && configured-inForce < uint64(pageSize) {
		return configured
	}
	return inForce
}
