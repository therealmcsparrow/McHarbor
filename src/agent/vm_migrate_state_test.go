// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"errors"
	"strings"
	"testing"
)

func TestAPreparedMigrationOnlyAnswersItsOwnToken(t *testing.T) {
	targets := newMigrationTargets()
	token := strings.Repeat("a", 40)
	if err := targets.add("mig1", &migrationTarget{VMID: "vm1", Token: token}); err != nil {
		t.Fatal(err)
	}
	if err := targets.add("mig1", &migrationTarget{VMID: "vm1", Token: token}); err == nil {
		t.Fatal("the same migration was prepared twice")
	}
	if _, err := targets.take("mig1", strings.Repeat("b", 40)); !errors.Is(err, errMigrationUnknown) {
		t.Fatalf("wrong token = %v, want errMigrationUnknown", err)
	}
	if _, err := targets.take("mig1", ""); !errors.Is(err, errMigrationUnknown) {
		t.Fatalf("empty token = %v, want errMigrationUnknown", err)
	}
	target, err := targets.take("mig1", token)
	if err != nil || target.VMID != "vm1" {
		t.Fatalf("take = %+v, %v", target, err)
	}
	// One-time: a replayed finish or abort finds nothing.
	if _, err := targets.take("mig1", token); !errors.Is(err, errMigrationUnknown) {
		t.Fatalf("second take = %v, want errMigrationUnknown", err)
	}
}

func TestMigrationTokensMustBeLong(t *testing.T) {
	if err := newMigrationTargets().add("mig1", &migrationTarget{Token: "short"}); err == nil {
		t.Fatal("a short token was accepted")
	}
}

func TestMigrationPeerValidation(t *testing.T) {
	for _, ok := range []string{"192.168.1.10", "fd00::1", "host-2.lan"} {
		if err := validateMigrationPeer(ok, 49152); err != nil {
			t.Fatalf("%q refused: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "host;rm -rf /", "-oProxy", "a b", "exec:cat"} {
		if err := validateMigrationPeer(bad, 49152); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
	if err := validateMigrationPeer("10.0.0.1", 70000); err == nil {
		t.Fatal("port 70000 accepted")
	}
	if got := migrationURI("fd00::1", 4444); got != "tcp:[fd00::1]:4444" {
		t.Fatalf("migrationURI = %q", got)
	}
}

func TestMigrationSendsOnlyTheGuestsOwnDisks(t *testing.T) {
	spec := &VMSpec{Disks: []VMDiskSpec{
		{Slot: 0, VolumePath: "/pool/vm1/disk-0"},
		{Slot: 1, VolumePath: "/iso/installer.iso", IsCDROM: true},
	}}
	if _, err := sourceDiskFor(spec, VMMigrateDisk{Slot: 0, SourcePath: "/pool/vm1/disk-0"}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []VMMigrateDisk{
		{Slot: 0, SourcePath: "/etc/shadow"},
		{Slot: 1, SourcePath: "/iso/installer.iso"},
		{Slot: 7, SourcePath: "/pool/vm1/disk-0"},
	} {
		if _, err := sourceDiskFor(spec, bad); err == nil {
			t.Fatalf("%+v accepted", bad)
		}
	}
}
