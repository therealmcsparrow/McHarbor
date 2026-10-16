// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package vm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func seedPaths(t *testing.T, withSeed bool) Paths {
	t.Helper()
	dir := t.TempDir()
	paths := Paths{Dir: dir, SeedISO: filepath.Join(dir, "seed.iso"), SeedMarker: filepath.Join(dir, "seed.delivered")}
	if withSeed {
		if err := os.WriteFile(paths.SeedISO, []byte("CD001"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return paths
}

func seedDevice(t *testing.T, spec Spec) (device string, argv []string) {
	t.Helper()
	argv, err := buildDiskArgs(spec, false)
	if err != nil {
		t.Fatalf("buildDiskArgs: %v", err)
	}
	for _, arg := range argv {
		if strings.Contains(arg, "drive=blk99") {
			return arg, argv
		}
	}
	return "", argv
}

// TestSeedIsAttachedOnTheFirstBoot is the AlmaLinux failure: the seed was
// written at define but no start ever handed it to QEMU, so cloud-init never
// created the wizard's account.
func TestSeedIsAttachedOnTheFirstBoot(t *testing.T) {
	paths := seedPaths(t, true)
	spec := Spec{Disks: []Disk{{Slot: 0, Path: "/pool/vm/disk0.qcow2", Format: "qcow2", IsBoot: true, Bus: "virtio-blk"}}}

	if !AttachSeed(&spec, paths) {
		t.Fatal("a guest with a seed and no delivery marker was started without it")
	}
	device, argv := seedDevice(t, spec)
	if device == "" {
		t.Fatalf("no device for the seed drive in %v", argv)
	}
	if !strings.HasPrefix(device, "ide-cd,") {
		t.Fatalf("seed device = %s, want a CD-ROM", device)
	}
	if strings.Contains(device, "bootindex") {
		t.Fatalf("seed device %s takes a boot index; it must never be booted from", device)
	}
	if !strings.Contains(strings.Join(argv, " "), paths.SeedISO) {
		t.Fatalf("the seed image path is not on the command line: %v", argv)
	}
	if !strings.Contains(strings.Join(argv, " "), "read-only=on") {
		t.Fatalf("the seed is not attached read-only: %v", argv)
	}
}

func TestSeedIsLeftOffAfterDeliveryAndBackAfterARedefine(t *testing.T) {
	paths := seedPaths(t, true)
	if err := MarkSeedDelivered(paths); err != nil {
		t.Fatal(err)
	}
	spec := Spec{}
	if AttachSeed(&spec, paths) || len(spec.Disks) != 0 {
		t.Fatal("a delivered seed was attached again, which would block live migration")
	}

	if err := ResetSeedDelivery(paths); err != nil {
		t.Fatal(err)
	}
	if !AttachSeed(&spec, paths) {
		t.Fatal("a newly written seed was not attached on the next start")
	}
}

func TestSeedIsNeverAttachedToAMigrationTargetOrWithoutAnImage(t *testing.T) {
	incoming := Spec{IncomingURI: "defer"}
	if AttachSeed(&incoming, seedPaths(t, true)) {
		t.Fatal("the seed was attached to an incoming migration, whose source has no such device")
	}
	plain := Spec{}
	if AttachSeed(&plain, seedPaths(t, false)) {
		t.Fatal("a guest without a seed image got a seed drive")
	}
}

// TestSeedLeavesTheInstallerDiscBootable: two CD-ROMs, and only the installer
// keeps its boot index.
func TestSeedLeavesTheInstallerDiscBootable(t *testing.T) {
	paths := seedPaths(t, true)
	spec := Spec{Disks: []Disk{{Slot: 1, Path: "/iso/alma.iso", Format: "raw", IsCDROM: true, ReadOnly: true, Bus: "sata"}}}
	AttachSeed(&spec, paths)
	argv, err := buildDiskArgs(spec, false)
	if err != nil {
		t.Fatal(err)
	}
	boots := 0
	for _, arg := range argv {
		if strings.HasPrefix(arg, "ide-cd,") && strings.Contains(arg, "bootindex=") {
			boots++
			if !strings.Contains(arg, "drive=blk1") {
				t.Fatalf("the boot index went to %s, not the installer disc", arg)
			}
		}
	}
	if boots != 1 {
		t.Fatalf("%d CD-ROMs carry a boot index, want only the installer: %v", boots, argv)
	}
}
