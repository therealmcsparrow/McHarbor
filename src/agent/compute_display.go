// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"strconv"
	"strings"
)

// displayResizeMinMajor is the first QEMU release whose VNC server accepts a
// client's SetDesktopSize; older ones keep the guest at its own resolution.
const displayResizeMinMajor = 6

// displayResizeSupported reports whether a guest booted with this binary's
// default adapter can follow the console window. virtio-vga is the adapter that
// implements QEMU's ui_info, which both VNC and SPICE resize requests end in.
func displayResizeSupported(version string, devices map[string]bool) bool {
	if !devices["virtio-vga"] && !devices["virtio-gpu-pci"] {
		return false
	}
	major, err := strconv.Atoi(strings.SplitN(version, ".", 2)[0])
	return err == nil && major >= displayResizeMinMajor
}
