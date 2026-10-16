// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lxc

import "testing"

func TestRootfsOfFindsOnlyEntriesStrictlyInsideAContainerRootfs(t *testing.T) {
	for _, tc := range []struct {
		rel    string
		id     string
		rootfs string
		ok     bool
	}{
		{"containers/ct1/rootfs/root/a.txt", "ct1", "containers/ct1/rootfs", true},
		{"containers/ct1/rootfs/etc", "ct1", "containers/ct1/rootfs", true},
		{"containers/ct1/rootfs/root/../etc/x", "ct1", "containers/ct1/rootfs", true},
		{"containers/ct1/rootfs", "", "", false},
		{"containers/ct1/notes.txt", "", "", false},
		{"containers/ct1", "", "", false},
		{"images/alpine/rootfs/etc", "", "", false},
		{"guests/containers/ct1/rootfs/etc", "", "", false},
		{"/containers/ct1/rootfs/etc", "", "", false},
		{"containers/ct1/rootfs/..", "", "", false},
		{"../containers/ct1/rootfs/etc", "", "", false},
		{"", "", "", false},
	} {
		id, rootfs, ok := RootfsOf(tc.rel)
		if id != tc.id || rootfs != tc.rootfs || ok != tc.ok {
			t.Errorf("RootfsOf(%q) = %q, %q, %v; want %q, %q, %v", tc.rel, id, rootfs, ok, tc.id, tc.rootfs, tc.ok)
		}
	}
}

func TestRootfsOwnerOwnsOnlyItsOwnRange(t *testing.T) {
	unpriv := RootfsOwner{UID: 1000000, GID: 1000000, Shift: IDShift{
		UIDStart: 1000000, UIDCount: 65536, GIDStart: 1000000, GIDCount: 65536,
	}}
	for _, tc := range []struct {
		uid, gid int64
		want     bool
	}{
		{1000000, 1000000, true},
		{1065535, 1065535, true},
		{1001000, 1000000, true},
		{0, 0, false},
		{1065536, 1000000, false},
		{1000000, 999999, false},
	} {
		if got := unpriv.Owns(tc.uid, tc.gid); got != tc.want {
			t.Errorf("unprivileged Owns(%d, %d) = %v, want %v", tc.uid, tc.gid, got, tc.want)
		}
	}
	if !(RootfsOwner{}).Owns(0, 0) || !(RootfsOwner{}).Owns(1000, 1000) {
		t.Error("a privileged container must own every host id")
	}
}
