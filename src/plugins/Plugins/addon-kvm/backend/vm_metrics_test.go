// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"testing"
	"time"
)

// TestMetricsPointsAreRates guards the history route's contract: disk and
// network leave as bytes per second, and a guest restart reads 0, not negative.
func TestMetricsPointsAreRates(t *testing.T) {
	base := time.Date(2026, 10, 2, 8, 0, 0, 0, time.UTC)
	samples := []storedSample{
		{at: base, cpu: 10, memory: 512, memoryMax: 1024, diskRead: 1_000, diskWrite: 0, netRx: 3_000, netTx: 300},
		{at: base.Add(30 * time.Second), cpu: 20, memory: 768, memoryMax: 1024, diskRead: 4_000, diskWrite: 600, netRx: 6_000, netTx: 600},
		// Restarted guest: every counter starts again from zero.
		{at: base.Add(60 * time.Second), cpu: 5, memory: 256, memoryMax: 1024, diskRead: 30, diskWrite: 0, netRx: 0, netTx: 0},
	}
	points := metricsPoints(samples)
	if len(points) != 2 {
		t.Fatalf("points = %d, want 2 (one per interval)", len(points))
	}
	first := points[0]
	if first.DiskReadRate != 100 || first.DiskWriteRate != 20 || first.NetRxRate != 100 || first.NetTxRate != 10 {
		t.Errorf("first interval = %+v, want 100/20 disk and 100/10 net B/s", first)
	}
	if first.CPUPercent != 20 || first.MemoryPercent != 75 {
		t.Errorf("first interval cpu/mem = %v/%v, want the later sample's 20%% / 75%%", first.CPUPercent, first.MemoryPercent)
	}
	second := points[1]
	if second.DiskReadRate != 0 || second.NetRxRate != 0 {
		t.Errorf("after a restart = %+v, want 0 rates rather than negative ones", second)
	}
}

// TestMemoryCeilingFallsBackToTheRecord guards the zero the agent always sends:
// without the fallback the memory tile divided by nothing.
func TestMemoryCeilingFallsBackToTheRecord(t *testing.T) {
	const mib = 1024 * 1024
	cases := []struct {
		name     string
		reported int64
		vm       VM
		want     int64
	}{
		{"agent value wins", 4 * mib, VM{MemoryMB: 8192}, 4 * mib},
		{"hotplugged size", 0, VM{MemoryMB: 4096, MemoryCurrentMB: 6144}, 6144 * mib},
		{"boot size", 0, VM{MemoryMB: 4096}, 4096 * mib},
	}
	for _, tc := range cases {
		if got := memoryCeiling(tc.reported, tc.vm); got != tc.want {
			t.Errorf("%s: memoryCeiling = %d, want %d", tc.name, got, tc.want)
		}
	}
}
