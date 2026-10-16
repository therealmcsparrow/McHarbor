// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"math"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/lxc"
)

func TestToProtocolCTStatsCarriesEveryCounter(t *testing.T) {
	at := time.Date(2026, 9, 27, 18, 30, 0, 0, time.UTC)
	got := toProtocolCTStats(lxc.Stats{
		ContainerID: "ct1", CPUUsageNSec: 1, CPUPercent: 12.5, MemoryBytes: 2, MemoryMaxBytes: 3,
		SwapBytes: 4, PidsCurrent: 5, PidsMax: 6, IOReadBytes: 7, IOWriteBytes: 8,
		NetRxBytes: 9, NetTxBytes: 10, MemoryHighBytes: 11, RootfsUsedBytes: 12, SampledAt: at,
	})
	want := CTStats{
		ContainerID: "ct1", CPUUsageNSec: 1, CPUPercent: 12.5, MemoryBytes: 2, MemoryMaxBytes: 3,
		SwapBytes: 4, PidsCurrent: 5, PidsMax: 6, IOReadBytes: 7, IOWriteBytes: 8,
		NetRxBytes: 9, NetTxBytes: 10, MemoryHighBytes: 11, RootfsUsedBytes: 12, SampledAt: "2026-09-27T18:30:00Z",
	}
	if *got != want {
		t.Fatalf("stats = %+v\nwant   %+v", *got, want)
	}
}

func TestToProtocolCTStatsNeverWrapsNegative(t *testing.T) {
	got := toProtocolCTStats(lxc.Stats{MemoryBytes: math.MaxUint64})
	if got.MemoryBytes != math.MaxInt64 {
		t.Fatalf("memory = %d, want clamped to MaxInt64", got.MemoryBytes)
	}
}
