// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package vm

import (
	"fmt"
	"os"
	"strings"
)

// The hot-plug layout. q35's root bus cannot unplug anything, so every device
// that may come or go while a guest runs sits behind a pcie-root-port of its
// own. Each port belongs to one slot number and is emitted at every boot,
// whether or not that slot holds a device: a device plugged in live lands
// exactly where the next boot puts it from its row, so the running guest and
// its definition never describe two different machines.
const (
	// NICPorts is how many NIC slots get a port. A NIC in a higher slot falls
	// back to the root bus and cannot be unplugged live.
	NICPorts = 8
	// DiskPorts is how many disk slots get a port. Disk slots are shared with
	// the boot disk and the disc drives, so this covers more data disks than
	// a guest realistically carries.
	DiskPorts = 16

	// nicPortSlot and diskPortSlots are the pcie.0 slots the ports occupy, eight
	// functions each. They sit just below the ICH9 functions at 0x1f, far above
	// anything QEMU allocates on its own, so no existing device moves. The NIC
	// ports come first: firmware numbers buses in slot order, which keeps NIC k
	// on bus k+1 however many disks the guest has.
	nicPortSlot = 0x1c
)

var diskPortSlots = []int{0x1d, 0x1e}

// NICPortID names the root port a NIC slot plugs into.
func NICPortID(slot int) string { return fmt.Sprintf("rpn%d", slot) }

// DiskPortID names the root port a disk slot plugs into.
func DiskPortID(slot int) string { return fmt.Sprintf("rpd%d", slot) }

// isQ35 reports whether a machine type is a PCIe one. i440fx guests keep every
// device on pci.0, which ACPI hot-plug already serves.
func isQ35(machineType string) bool {
	return strings.Contains(strings.ToLower(machineType), "q35")
}

// usesHotplugLayout reports whether this boot gets the root ports.
//
// A NIC pinned to one of the port slots by an earlier boot would collide with
// them; that guest keeps the old layout rather than failing to start.
func usesHotplugLayout(spec Spec) bool {
	if spec.LegacyLayout || !isQ35(spec.MachineType) {
		return false
	}
	reserved := map[string]bool{fmt.Sprintf("0x%02x", nicPortSlot): true}
	for _, slot := range diskPortSlots {
		reserved[fmt.Sprintf("0x%02x", slot)] = true
	}
	for _, nic := range spec.NICs {
		if reserved[strings.ToLower(nic.PCIAddr)] {
			return false
		}
	}
	return true
}

// nicHasPort reports whether a NIC goes behind its port. A NIC already pinned
// to pcie.0 stays there: moving it would rename the guest's interface.
func nicHasPort(nic NIC) bool {
	return nic.PCIAddr == "" && nic.Slot >= 0 && nic.Slot < NICPorts
}

// diskHasPort reports whether a disk goes behind its port. The boot disk never
// does, because it is never unplugged.
func diskHasPort(disk Disk) bool {
	return !disk.IsBoot && !disk.IsCDROM && disk.Slot >= 0 && disk.Slot < DiskPorts
}

// rootPortArgs emits every port with a fixed address.
//
// io-reserve=0 because the I/O space is 64 KiB and the firmware reserves 4 KiB
// per bridge by default; 24 bridges would exhaust it. Every device placed here
// is modern virtio or PCIe and needs no I/O window.
func rootPortArgs() []string {
	var argv []string
	chassis := 1
	emit := func(id string, slot, function int) {
		opts := fmt.Sprintf("pcie-root-port,id=%s,bus=pcie.0,addr=0x%02x.0x%x,chassis=%d,port=0x%x,io-reserve=0",
			id, slot, function, chassis, 0x10+chassis-1)
		if function == 0 {
			opts += ",multifunction=on"
		}
		argv = append(argv, "-device", opts)
		chassis++
	}
	for i := 0; i < NICPorts; i++ {
		emit(NICPortID(i), nicPortSlot, i)
	}
	for i := 0; i < DiskPorts; i++ {
		emit(DiskPortID(i), diskPortSlots[i/8], i%8)
	}
	return argv
}

// HasHotplugLayout reports whether the guest whose args.json this is was booted
// with its root ports. A live-migration source answers it so the target is
// built to match.
func HasHotplugLayout(argsFile string) bool {
	raw, err := os.ReadFile(argsFile)
	if err != nil {
		return false
	}
	return strings.Contains(string(raw), "id="+NICPortID(0)+",")
}

// VhostEnabled reports whether NICs use vhost-net, the same switch the command
// line reads, so a NIC plugged in live takes the same RX path as its siblings.
func VhostEnabled() bool { return vhostOption() == "vhost=on" }
