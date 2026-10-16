// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package lxc

import "testing"

func TestMemoryHighIsTheThrottleNotAProtectionFloor(t *testing.T) {
	cg := buildCgroups(Spec{ID: "ct1", Limits: Limits{MemoryMaxBytes: 268435456, MemoryHighBytes: 241591905}})
	if got := cg.Unified["memory.high"]; got != "241591905" {
		t.Fatalf("memory.high = %q, want 241591905", got)
	}
	if cg.MemoryReservation != 0 {
		t.Fatalf("MemoryReservation = %d: on cgroup v2 that is memory.low, a floor the kernel protects",
			cg.MemoryReservation)
	}
	if cg.MemorySwap != 268435456 {
		t.Fatalf("memory+swap = %d, want the memory limit with no swap", cg.MemorySwap)
	}

	none := buildCgroups(Spec{ID: "ct2"})
	if _, set := none.Unified["memory.high"]; set {
		t.Fatal("memory.high was written for a container with no throttle")
	}
}
