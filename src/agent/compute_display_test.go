// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import "testing"

func TestDisplayResizeNeedsVirtioVGAAndQEMU6(t *testing.T) {
	gpu := map[string]bool{"virtio-vga": true}
	cases := []struct {
		name    string
		version string
		devices map[string]bool
		want    bool
	}{
		{"modern qemu with virtio-vga", "8.2.2", gpu, true},
		{"first release with SetDesktopSize", "6.0.0", gpu, true},
		{"qemu 5 ignores SetDesktopSize", "5.2.0", gpu, false},
		{"no virtio-gpu device built in", "9.0.0", map[string]bool{"qxl-vga": true}, false},
		{"unknown version", "", gpu, false},
	}
	for _, tc := range cases {
		if got := displayResizeSupported(tc.version, tc.devices); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
