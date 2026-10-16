// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"math"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/lxc"
)

// toProtocolCTStats converts a manager sample for the wire, where counters are
// signed and zero means "not reported" or "no limit".
func toProtocolCTStats(in lxc.Stats) *CTStats {
	return &CTStats{
		ContainerID:     in.ContainerID,
		CPUPercent:      in.CPUPercent,
		CPUUsageNSec:    clampInt64(in.CPUUsageNSec),
		MemoryBytes:     clampInt64(in.MemoryBytes),
		MemoryMaxBytes:  clampInt64(in.MemoryMaxBytes),
		MemoryHighBytes: clampInt64(in.MemoryHighBytes),
		SwapBytes:       clampInt64(in.SwapBytes),
		PidsCurrent:     clampInt64(in.PidsCurrent),
		PidsMax:         clampInt64(in.PidsMax),
		IOReadBytes:     clampInt64(in.IOReadBytes),
		IOWriteBytes:    clampInt64(in.IOWriteBytes),
		NetRxBytes:      clampInt64(in.NetRxBytes),
		NetTxBytes:      clampInt64(in.NetTxBytes),
		RootfsUsedBytes: clampInt64(in.RootfsUsedBytes),
		SampledAt:       in.SampledAt.UTC().Format(time.RFC3339Nano),
	}
}

// clampInt64 keeps a kernel counter from wrapping negative on the wire.
func clampInt64(v uint64) int64 {
	if v > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(v)
}
