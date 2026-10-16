// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package vm

import (
	"slices"
	"strings"
	"testing"
)

func displayTestSpec(family, display string, spice bool) Spec {
	return Spec{
		ID: "vm1", Name: "guest", UUID: "00000000-0000-0000-0000-000000000001",
		VCPUs: 1, MemoryMB: 512, MachineType: "pc-q35-8.2", Firmware: "bios",
		GuestOSFamily: family, DisplayType: display, SPICE: spice,
	}
}

func buildDisplayArgv(t *testing.T, spec Spec) []string {
	t.Helper()
	argv, err := BuildArgv("/usr/bin/qemu-system-x86_64", spec, PathsFor("vm1"))
	if err != nil {
		t.Fatalf("BuildArgv: %v", err)
	}
	return argv
}

// The console can only resize the guest when the adapter implements QEMU's
// ui_info; std VGA does not, so a family defaulting to it can never follow
// the browser window.
func TestDefaultDisplayIsAResizableAdapter(t *testing.T) {
	for _, family := range []string{OSFamilyLinux, OSFamilyBSD, OSFamilyOther, ""} {
		argv := buildDisplayArgv(t, displayTestSpec(family, "", false))
		if !slices.Contains(argv, "virtio-vga,id=video0") {
			t.Errorf("family %q: want virtio-vga, got %v", family, argv)
		}
		if vga, _ := argAfter(argv, "-vga"); vga != "none" {
			t.Errorf("family %q: -vga = %q, want none beside virtio-vga", family, vga)
		}
	}

	windows := buildDisplayArgv(t, displayTestSpec(OSFamilyWindows, "", false))
	if !slices.Contains(windows, "qxl-vga,id=video0") {
		t.Errorf("windows: want qxl-vga (no virtio-gpu driver ships with Windows), got %v", windows)
	}
}

func TestStoredDisplayTypeOverridesTheDefault(t *testing.T) {
	argv := buildDisplayArgv(t, displayTestSpec(OSFamilyLinux, "std", false))
	if vga, _ := argAfter(argv, "-vga"); vga != "std" {
		t.Fatalf("an operator's std choice was replaced: -vga = %q", vga)
	}
	if slices.Contains(argv, "virtio-vga,id=video0") {
		t.Fatalf("std guest also got virtio-vga: %v", argv)
	}
}

// spice-vdagent is what turns the client's monitor config into a guest
// resolution, so every SPICE guest with a display needs its port.
func TestVDAgentPortExistsWheneverSPICEIsOn(t *testing.T) {
	for _, display := range []string{"", "virtio-gpu", "qxl", "std"} {
		joined := strings.Join(buildDisplayArgv(t, displayTestSpec(OSFamilyLinux, display, true)), " ")
		if !strings.Contains(joined, "virtserialport,bus=vserial0.0,chardev=vdagent0,name=com.redhat.spice.0") {
			t.Errorf("display %q: SPICE guest has no vdagent port: %s", display, joined)
		}
	}
}

// A boot resolution reaches the adapters that take one, and only when both
// sides are set; std and an unset size keep QEMU's default.
func TestDisplayArgsCarryBootResolution(t *testing.T) {
	for display, device := range map[string]string{"virtio-gpu": "virtio-vga", "qxl": "qxl-vga"} {
		spec := displayTestSpec("linux", display, false)
		spec.DisplayWidth, spec.DisplayHeight = 1920, 1080
		if argv := buildDisplayArgv(t, spec); !slices.Contains(argv, device+",id=video0,xres=1920,yres=1080") {
			t.Errorf("%s: want xres/yres on %s, got %v", display, device, argv)
		}
	}

	half := displayTestSpec("linux", "virtio-gpu", false)
	half.DisplayWidth = 1920
	if argv := buildDisplayArgv(t, half); !slices.Contains(argv, "virtio-vga,id=video0") {
		t.Errorf("width alone must not set a resolution, got %v", argv)
	}

	std := displayTestSpec("linux", "std", false)
	std.DisplayWidth, std.DisplayHeight = 1920, 1080
	if argv := buildDisplayArgv(t, std); strings.Contains(strings.Join(argv, " "), "xres=") {
		t.Errorf("std VGA takes no resolution here, got %v", argv)
	}
}
