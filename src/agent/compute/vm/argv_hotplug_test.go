// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package vm

import (
	"strings"
	"testing"
)

func hotplugSpec() Spec {
	return Spec{
		ID: "vm1", Name: "web", UUID: "u", VCPUs: 2, MemoryMB: 2048, MachineType: "pc-q35-8.2",
		Disks: []Disk{
			{Slot: 0, Path: "/pool/boot.qcow2", IsBoot: true},
			{Slot: 2, Path: "/pool/data.qcow2"},
			{Slot: 3, IsCDROM: true},
		},
		NICs: []NIC{
			{Slot: 0, MAC: "52:54:00:00:00:01", TapName: "mch-t0", PCIAddr: "0x04"},
			{Slot: 1, MAC: "52:54:00:00:00:02", TapName: "mch-t1"},
		},
	}
}

func devicesOf(t *testing.T, spec Spec) []string {
	t.Helper()
	argv, err := BuildArgv("/usr/bin/qemu-system-x86_64", spec, PathsFor(spec.ID))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for i, arg := range argv {
		if arg == "-device" && i+1 < len(argv) {
			out = append(out, argv[i+1])
		}
	}
	return out
}

func deviceWith(devices []string, prefix string) string {
	for _, d := range devices {
		if strings.HasPrefix(d, prefix) || strings.Contains(d, ","+prefix) {
			return d
		}
	}
	return ""
}

func TestQ35GuestsGetADedicatedPortPerSlot(t *testing.T) {
	devices := devicesOf(t, hotplugSpec())

	ports := 0
	for _, d := range devices {
		if strings.HasPrefix(d, "pcie-root-port,") {
			ports++
			if !strings.Contains(d, "io-reserve=0") {
				t.Errorf("port reserves I/O space: %s", d)
			}
		}
	}
	if ports != NICPorts+DiskPorts {
		t.Fatalf("emitted %d root ports, want %d", ports, NICPorts+DiskPorts)
	}
	if d := deviceWith(devices, "pcie-root-port,id=rpn0,"); !strings.Contains(d, "addr=0x1c.0x0") || !strings.Contains(d, "multifunction=on") {
		t.Fatalf("rpn0 = %q, want slot 0x1c function 0 with multifunction", d)
	}

	if d := deviceWith(devices, "id=disk2"); !strings.Contains(d, "bus=rpd2") {
		t.Fatalf("data disk = %q, want it behind rpd2", d)
	}
	if d := deviceWith(devices, "id=disk0"); strings.Contains(d, "bus=") {
		t.Fatalf("boot disk = %q, want it on pcie.0", d)
	}
	if d := deviceWith(devices, "id=nic1"); !strings.Contains(d, "bus=rpn1") {
		t.Fatalf("unpinned NIC = %q, want it behind rpn1", d)
	}
	if d := deviceWith(devices, "id=nic0"); strings.Contains(d, "bus=") || !strings.Contains(d, "addr=0x04") {
		t.Fatalf("pinned NIC = %q, want it kept at its pinned pcie.0 slot", d)
	}
	if d := deviceWith(devices, "ide-cd,id=disk3"); d == "" || strings.Contains(d, "drive=") || strings.Contains(d, "bootindex") {
		t.Fatalf("empty drive = %q, want an ide-cd with no medium and no boot index", d)
	}
}

func TestLayoutFallsBackWhenItCannotApply(t *testing.T) {
	cases := map[string]func(*Spec){
		"migration target of an old boot": func(s *Spec) { s.LegacyLayout = true },
		"i440fx":                          func(s *Spec) { s.MachineType = "pc-i440fx-8.2" },
		"NIC pinned on a port slot":       func(s *Spec) { s.NICs[0].PCIAddr = "0x1d" },
	}
	for name, adjust := range cases {
		t.Run(name, func(t *testing.T) {
			spec := hotplugSpec()
			adjust(&spec)
			for _, d := range devicesOf(t, spec) {
				if strings.HasPrefix(d, "pcie-root-port") || strings.Contains(d, "bus=rp") {
					t.Fatalf("legacy layout emitted %q", d)
				}
			}
		})
	}
}

func TestHasHotplugLayoutReadsTheArgsFile(t *testing.T) {
	dir := t.TempDir()
	argv, err := BuildArgv("/usr/bin/qemu", hotplugSpec(), PathsFor("vm1"))
	if err != nil {
		t.Fatal(err)
	}
	path := dir + "/args.json"
	if err := WriteArgsFile(path, argv); err != nil {
		t.Fatal(err)
	}
	if !HasHotplugLayout(path) {
		t.Fatal("a ported boot read as legacy")
	}
	legacy := hotplugSpec()
	legacy.LegacyLayout = true
	argv, err = BuildArgv("/usr/bin/qemu", legacy, PathsFor("vm1"))
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteArgsFile(path, argv); err != nil {
		t.Fatal(err)
	}
	if HasHotplugLayout(path) {
		t.Fatal("a legacy boot read as ported")
	}
}
