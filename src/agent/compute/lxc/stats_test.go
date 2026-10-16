// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lxc

import (
	"math"
	"testing"
	"time"
)

func TestCPUPercentIsTheShareOfTheAllowance(t *testing.T) {
	at := time.Date(2026, 9, 27, 18, 0, 0, 0, time.UTC)
	prev := cpuSample{usageNSec: 1_000_000_000, at: at}
	cur := cpuSample{usageNSec: 1_500_000_000, at: at.Add(time.Second)}
	if got := cpuPercent(prev, cur, 1); got != 50 {
		t.Fatalf("half a CPU-second in one second on one CPU = %v%%, want 50", got)
	}
	if got := cpuPercent(prev, cur, 2); got != 25 {
		t.Fatalf("on a two-CPU allowance = %v%%, want 25", got)
	}
	busy := cpuSample{usageNSec: 9_000_000_000, at: cur.at}
	if got := cpuPercent(prev, busy, 1); got != 100 {
		t.Fatalf("a burst over the allowance = %v%%, want clamped to 100", got)
	}
}

func TestCPUPercentNeedsAUsableBaseline(t *testing.T) {
	at := time.Date(2026, 9, 27, 18, 0, 0, 0, time.UTC)
	cur := cpuSample{usageNSec: 5, at: at}
	for name, prev := range map[string]cpuSample{
		"none":         {},
		"stale":        {usageNSec: 1, at: at.Add(-2 * cpuSampleMaxAge)},
		"future":       {usageNSec: 1, at: at.Add(time.Second)},
		"restarted":    {usageNSec: 10, at: at.Add(-time.Second)},
		"same-instant": {usageNSec: 1, at: at},
	} {
		if usableCPUSample(prev, cur) {
			t.Fatalf("%s: accepted as a baseline", name)
		}
		if got := cpuPercent(prev, cur, 1); got != 0 {
			t.Fatalf("%s: percent = %v, want 0", name, got)
		}
	}
}

func TestCPUAllowanceUsesTheQuotaWhenSet(t *testing.T) {
	if got := cpuAllowance(150000, 100000, 8); got != 1.5 {
		t.Fatalf("quota 150ms/100ms = %v CPUs, want 1.5", got)
	}
	if got := cpuAllowance(0, 100000, 8); got != 8 {
		t.Fatalf("no quota = %v CPUs, want the host's 8", got)
	}
}

func TestUnlimitedAsZeroMapsCgroupMaxToNoLimit(t *testing.T) {
	if got := unlimitedAsZero(math.MaxUint64); got != 0 {
		t.Fatalf("max = %d, want 0", got)
	}
	if got := unlimitedAsZero(512 << 20); got != 512<<20 {
		t.Fatalf("a real limit changed to %d", got)
	}
}

func TestParseNetDevSumsEveryInterfaceButLoopback(t *testing.T) {
	body := `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo:    9999      10    0    0    0     0          0         0     9999      10    0    0    0     0       0          0
  eth0:    1000       5    0    0    0     0          0         0      300       3    0    0    0     0       0          0
  eth1:      24       1    0    0    0     0          0         0       76       1    0    0    0     0       0          0
`
	rx, tx := parseNetDev(body)
	if rx != 1024 || tx != 376 {
		t.Fatalf("rx=%d tx=%d, want 1024 and 376 without loopback", rx, tx)
	}
}

func TestParseCgroupLimitReadsMemoryHigh(t *testing.T) {
	cases := map[string]uint64{
		"241591905\n": 241591905,
		"max\n":       0,
		"":            0,
		"garbage":     0,
	}
	for body, want := range cases {
		if got := parseCgroupLimit(body); got != want {
			t.Errorf("%q = %d, want %d", body, got, want)
		}
	}
}

func TestReportedMemoryHighIsTheConfiguredValueTheKernelRounded(t *testing.T) {
	cases := []struct {
		name                string
		configured, inForce uint64
		want                uint64
	}{
		{"page-rounded by the kernel", 241591905, 241590272, 241591905},
		{"exactly a page multiple", 241590272, 241590272, 241590272},
		{"not applied", 241591905, 0, 0},
		{"changed outside McHarbor", 241591905, 100 << 20, 100 << 20},
		{"none configured", 0, 100 << 20, 100 << 20},
	}
	for _, c := range cases {
		if got := reportedMemoryHigh(c.configured, c.inForce, 4096); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}
