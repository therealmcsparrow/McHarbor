// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package vm

import (
	"strings"
	"testing"
)

func spiceTestSpec(spice bool) Spec {
	return Spec{
		ID: "vm1", Name: "guest", UUID: "00000000-0000-0000-0000-000000000001",
		VCPUs: 1, MemoryMB: 512, MachineType: "pc-q35-8.2", Firmware: "bios",
		DisplayType: "qxl", SPICE: spice,
	}
}

func argAfter(argv []string, flag string) (string, bool) {
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == flag {
			return argv[i+1], true
		}
	}
	return "", false
}

func TestSPICEListensOnTheGuestSocketWithTicketingOff(t *testing.T) {
	paths := PathsFor("vm1")
	argv, err := BuildArgv("/usr/bin/qemu-system-x86_64", spiceTestSpec(true), paths)
	if err != nil {
		t.Fatalf("BuildArgv: %v", err)
	}
	spice, ok := argAfter(argv, "-spice")
	if !ok {
		t.Fatalf("no -spice in %v", argv)
	}
	want := "unix=on,addr=" + paths.SPICESocket + ",disable-ticketing=on"
	if spice != want {
		t.Fatalf("-spice = %q, want %q", spice, want)
	}
	// A TCP SPICE port would be reachable by anyone on the LAN, bypassing both
	// the console gateway and the relay's password check.
	if strings.Contains(spice, "port=") || strings.Contains(spice, "tls-port=") {
		t.Fatalf("SPICE must never listen on TCP: %q", spice)
	}
	joined := strings.Join(argv, " ")
	if !strings.Contains(joined, "spicevmc,id=vdagent0,name=vdagent") ||
		!strings.Contains(joined, "chardev=vdagent0,name=com.redhat.spice.0") {
		t.Fatalf("vdagent channel missing: %s", joined)
	}
	// The vdagent port hangs off the controller the guest agent creates.
	if strings.Index(joined, "virtio-serial-pci,id=vserial0") > strings.Index(joined, "com.redhat.spice.0") {
		t.Fatalf("vdagent port emitted before its virtio-serial controller: %s", joined)
	}
}

func TestSPICEIsOffUnlessTheAgentTurnsItOn(t *testing.T) {
	argv, err := BuildArgv("/usr/bin/qemu-system-x86_64", spiceTestSpec(false), PathsFor("vm1"))
	if err != nil {
		t.Fatalf("BuildArgv: %v", err)
	}
	if _, ok := argAfter(argv, "-spice"); ok {
		t.Fatalf("SPICE emitted for a guest that did not ask: %v", argv)
	}
	headless := spiceTestSpec(true)
	headless.DisplayType = "none"
	if got := spiceArgs(headless, PathsFor("vm1")); got != nil {
		t.Fatalf("a headless guest got SPICE: %v", got)
	}
}
