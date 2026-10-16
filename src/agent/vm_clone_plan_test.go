// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import "testing"

func cloneTestSource() []VMDiskSpec {
	return []VMDiskSpec{
		{DiskID: "d0", Slot: 0, VolumePath: "/pool/src/disk-0", Format: "qcow2", IsBoot: true},
		{DiskID: "cd", Slot: 1, VolumePath: "/images/debian-12.iso", Format: "raw", IsCDROM: true},
		{DiskID: "d2", Slot: 2, VolumePath: "/pool/src/disk-2", Format: "qcow2"},
	}
}

func TestCloneRewritesDisksButNeverTheDisc(t *testing.T) {
	copies, err := planVMClone(cloneTestSource(), "clone1")
	if err != nil {
		t.Fatal(err)
	}
	if len(copies) != 2 {
		t.Fatalf("copies = %+v, want the two volumes and not the disc", copies)
	}
	for _, c := range copies {
		if c.Source.IsCDROM {
			t.Fatalf("the CD-ROM was planned for a copy: %+v", c)
		}
	}
	if copies[0].TargetName != "clone1/disk-0" || copies[1].TargetName != "clone1/disk-2" {
		t.Fatalf("target names = %q, %q", copies[0].TargetName, copies[1].TargetName)
	}

	made := map[int]clonedVolume{
		0: {Path: "/pool/clone1/disk-0", Format: "qcow2", Backed: true},
		2: {Path: "/pool/clone1/disk-2", Format: "qcow2", Backed: true},
	}
	disks := clonedDisks(cloneTestSource(), made)
	if len(disks) != 3 {
		t.Fatalf("clone has %d disks, want 3", len(disks))
	}
	if disks[1].VolumePath != "/images/debian-12.iso" || disks[1].BackingPath != "" {
		t.Fatalf("the disc moved or gained a backing file: %+v", disks[1])
	}
	if disks[0].VolumePath != "/pool/clone1/disk-0" || disks[0].BackingPath != "/pool/src/disk-0" {
		t.Fatalf("linked disk = %+v", disks[0])
	}
}

func TestFullCloneHasNoBackingFile(t *testing.T) {
	disks := clonedDisks(cloneTestSource(), map[int]clonedVolume{
		0: {Path: "/pool/clone1/disk-0"},
		2: {Path: "/pool/clone1/disk-2"},
	})
	for _, disk := range disks {
		if disk.BackingPath != "" {
			t.Fatalf("a full clone disk reads through its source: %+v", disk)
		}
	}
}

func TestClonePlanRefusesAHostileTargetID(t *testing.T) {
	if _, err := planVMClone(cloneTestSource(), "../etc"); err == nil {
		t.Fatal("a target id with a path in it was accepted")
	}
	if _, err := planVMClone([]VMDiskSpec{{Slot: 0}}, "clone1"); err == nil {
		t.Fatal("a disk with no volume was accepted")
	}
}
