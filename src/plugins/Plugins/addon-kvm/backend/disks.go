// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import "fmt"

// diskBuses are the buses the agent knows how to wire. Anything else reaches
// QEMU as an unknown device and fails the start rather than the request.
var diskBuses = map[string]bool{
	"virtio-blk":  true,
	"virtio-scsi": true,
	"scsi":        true,
	"sata":        true,
	"ide":         true,
}

// defaultDiskBus picks the bus a guest of this family can boot from unaided.
//
// Windows Setup ships no virtio storage driver, so a virtio-blk disk simply
// does not appear in the installer's disk list — the guest boots the ISO and
// then reports that there is nowhere to install. SATA is slower and visible,
// which is the right trade for a disk nobody has installed onto yet; the bus
// can be changed afterwards, once the drivers are in the guest.
func defaultDiskBus(osFamily string) string {
	if osFamily == OSFamilyWindows {
		return "sata"
	}
	return "virtio-blk"
}

// validateDiskBus rejects a bus the agent would refuse at start time.
func validateDiskBus(bus string) error {
	if !diskBuses[bus] {
		return invalid(codeDiskBusInvalid, fmt.Sprintf("%q is not a disk bus", bus))
	}
	return nil
}
