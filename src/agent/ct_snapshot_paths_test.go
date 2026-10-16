// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import "testing"

func TestContainerSnapshotsSitBesideTheContainerInsideThePool(t *testing.T) {
	got, err := ctSnapshotRootfs("/pools/fast/containers/ct1/rootfs", "/pools/fast", "ct1", "snap1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/pools/fast/containers/ct1.snap-snap1/rootfs" {
		t.Fatalf("snapshot rootfs = %q", got)
	}
	if glob := ctSnapshotGlob("/pools/fast/containers/ct1/rootfs", "ct1"); glob != "/pools/fast/containers/ct1.snap-*" {
		t.Fatalf("snapshot glob = %q", glob)
	}
}

func TestContainerSnapshotPathsRefuseAnythingElse(t *testing.T) {
	for _, tc := range []struct {
		name, rootfs, pool, ctID, snapID string
	}{
		{"hostile snapshot id", "/pools/fast/containers/ct1/rootfs", "/pools/fast", "ct1", "../../etc"},
		{"another container's rootfs", "/pools/fast/containers/ct2/rootfs", "/pools/fast", "ct1", "snap1"},
		{"rootfs outside the pool", "/etc/containers/ct1/rootfs", "/pools/fast", "ct1", "snap1"},
		{"not a rootfs", "/pools/fast/containers/ct1/data", "/pools/fast", "ct1", "snap1"},
		{"pool root", "/pools/fast", "/pools/fast", "ct1", "snap1"},
	} {
		if got, err := ctSnapshotRootfs(tc.rootfs, tc.pool, tc.ctID, tc.snapID); err == nil {
			t.Fatalf("%s: accepted, snapshot at %q", tc.name, got)
		}
	}
}
