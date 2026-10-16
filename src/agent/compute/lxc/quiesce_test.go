// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lxc

import (
	"errors"
	"testing"
)

func TestDecideQuiesceRefusesALiveRootfsWithNeitherMechanism(t *testing.T) {
	// The case this whole function exists for. A copy taken here would hold
	// whatever was mid-write and would be indistinguishable from a good one.
	_, _, err := decideQuiesce(quiesceFacts{Live: true, RootfsPath: "/pool/ct-1"})

	if !errors.Is(err, ErrNoQuiesce) {
		t.Fatalf("expected ErrNoQuiesce, got %v", err)
	}
}

func TestDecideQuiescePrefersASnapshotOverAFreeze(t *testing.T) {
	source, consistency, err := decideQuiesce(quiesceFacts{
		Live:         true,
		RootfsPath:   "/pool/ct-1",
		SnapshotPath: "/pool/ct-1.snap.run",
		FreezeOK:     true,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source != "/pool/ct-1.snap.run" {
		t.Fatalf("expected the snapshot to be read, got %q", source)
	}
	if consistency != ConsistencySnapshot {
		t.Fatalf("expected %q, got %q", ConsistencySnapshot, consistency)
	}
}

func TestDecideQuiesceFreezesWhenThePoolCannotSnapshot(t *testing.T) {
	source, consistency, err := decideQuiesce(quiesceFacts{
		Live: true, RootfsPath: "/pool/ct-1", FreezeOK: true,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source != "/pool/ct-1" || consistency != ConsistencyFrozen {
		t.Fatalf("expected the live tree read frozen, got %q / %q", source, consistency)
	}
}

func TestDecideQuiesceLeavesAnOperatorFrozenContainerAlone(t *testing.T) {
	// FreezeOK is false because nothing was asked of the freezer: the container
	// was already frozen, and the copy is consistent without touching it.
	source, consistency, err := decideQuiesce(quiesceFacts{
		Live: true, AlreadyFrozen: true, RootfsPath: "/pool/ct-1",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if consistency != ConsistencyFrozen || source != "/pool/ct-1" {
		t.Fatalf("expected the live tree read frozen, got %q / %q", source, consistency)
	}
}

func TestDecideQuiesceNeedsNothingForAStoppedContainer(t *testing.T) {
	source, consistency, err := decideQuiesce(quiesceFacts{RootfsPath: "/pool/ct-1"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if consistency != ConsistencyFilesystem || source != "/pool/ct-1" {
		t.Fatalf("expected the tree read at rest, got %q / %q", source, consistency)
	}
}

func TestDecideQuiesceDecisionTable(t *testing.T) {
	cases := []struct {
		name        string
		facts       quiesceFacts
		source      string
		consistency string
		refused     bool
	}{
		{"stopped", quiesceFacts{RootfsPath: "/r"}, "/r", ConsistencyFilesystem, false},
		{"stopped ignores a snapshot", quiesceFacts{RootfsPath: "/r", SnapshotPath: "/s"}, "/r", ConsistencyFilesystem, false},
		{"running with a snapshot", quiesceFacts{Live: true, RootfsPath: "/r", SnapshotPath: "/s"}, "/s", ConsistencySnapshot, false},
		{"operator-frozen with a snapshot", quiesceFacts{Live: true, AlreadyFrozen: true, RootfsPath: "/r", SnapshotPath: "/s"}, "/s", ConsistencySnapshot, false},
		{"running, freezer accepted", quiesceFacts{Live: true, RootfsPath: "/r", FreezeOK: true}, "/r", ConsistencyFrozen, false},
		{"operator-frozen, no snapshot", quiesceFacts{Live: true, AlreadyFrozen: true, RootfsPath: "/r"}, "/r", ConsistencyFrozen, false},
		{"running, neither", quiesceFacts{Live: true, RootfsPath: "/r"}, "", "", true},
		{"running dir copy is a live copy", quiesceFacts{Live: true, RootfsPath: "/r", SnapshotPath: "/c", SnapshotIsCopy: true}, "/c", ConsistencyLiveCopy, false},
		{"operator-frozen dir copy is point-in-time", quiesceFacts{Live: true, AlreadyFrozen: true, RootfsPath: "/r", SnapshotPath: "/c", SnapshotIsCopy: true}, "/c", ConsistencySnapshot, false},
	}
	for _, c := range cases {
		source, consistency, err := decideQuiesce(c.facts)
		if c.refused {
			if !errors.Is(err, ErrNoQuiesce) {
				t.Errorf("%s: expected ErrNoQuiesce, got %q / %q / %v", c.name, source, consistency, err)
			}
			continue
		}
		if err != nil || source != c.source || consistency != c.consistency {
			t.Errorf("%s: got %q / %q / %v, want %q / %q", c.name, source, consistency, err, c.source, c.consistency)
		}
	}
}

func TestPlanSnapshotTakesOneOnlyForALiveContainerWithASnapshotter(t *testing.T) {
	cases := []struct {
		name                  string
		live, haveSnapshotter bool
		want                  snapshotPlan
	}{
		{"stopped never snapshots", false, true, snapshotPlan{}},
		{"no snapshotter", true, false, snapshotPlan{}},
		{"live with a snapshotter", true, true, snapshotPlan{Take: true}},
	}
	for _, c := range cases {
		if got := planSnapshot(c.live, c.haveSnapshotter); got != c.want {
			t.Errorf("%s: planSnapshot = %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestReadsCopyOnlyForATreeCopyTheArchiveActuallyReads(t *testing.T) {
	cases := []struct {
		name  string
		facts quiesceFacts
		want  bool
	}{
		{"running dir copy", quiesceFacts{Live: true, RootfsPath: "/r", SnapshotPath: "/c", SnapshotIsCopy: true}, true},
		{"operator-frozen dir copy", quiesceFacts{Live: true, AlreadyFrozen: true, RootfsPath: "/r", SnapshotPath: "/c", SnapshotIsCopy: true}, true},
		{"stopped dir container reads the rootfs", quiesceFacts{RootfsPath: "/r", SnapshotPath: "/c", SnapshotIsCopy: true}, false},
		{"pool snapshot", quiesceFacts{Live: true, RootfsPath: "/r", SnapshotPath: "/s"}, false},
		{"frozen rootfs", quiesceFacts{Live: true, RootfsPath: "/r", FreezeOK: true}, false},
	}
	for _, c := range cases {
		source, _, err := decideQuiesce(c.facts)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := readsCopy(c.facts, source); got != c.want {
			t.Errorf("%s: readsCopy = %v, want %v", c.name, got, c.want)
		}
	}
}
