// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package hardware enumerates the physical devices of the host McHarbor
// runs on: graphics adapters, physical network adapters, and attached
// devices (USB peripherals and block storage).
//
// # Why sysfs and not a library
//
// Docker's `/info` carries none of this — there is no GPU field, no NIC
// field, no device list, on any API version. The usual Go answer would be
// gopsutil or a udev binding, but both are heavier than the read this
// needs and udev wants cgo, which this project does not use.
//
// sysfs is already there. Docker mounts the host's `/sys` read-only into
// every container it starts, and the PCI, USB and block subsystems are
// host-global rather than namespaced — so a plain unprivileged McHarbor
// container can read them with no extra bind mount, no `--privileged`,
// and no host package. `/sys/class/net` is the one subsystem that *is*
// network-namespaced, which is why the network adapters are read from the
// PCI bus first: inside a container that directory lists the container's
// own veth pair, which is not what an operator means by "physical network
// adapter". It is still consulted afterwards, because it is the only
// place the kernel interface name lives and the only way an adapter that
// is not on PCI (vmbus, platform, USB) can be seen at all — but only for
// entries with a `device` symlink, which every virtual interface lacks.
//
// # Names
//
// PCI reports vendor and device as bare hex ids. Resolving them to full
// marketing names needs `pci.ids`, a ~1.5 MB table that would have to be
// shipped and kept fresh in the image. Instead a small embedded map (see
// pci_vendors.go) names the vendors an operator is likely to have, and
// anything unmapped degrades to `vendor:device` alongside the bound
// kernel driver — `8086:15f3 · igc` still identifies a card. USB does
// better without any table: the descriptors carry real manufacturer and
// product strings.
//
// # Scope
//
// This is the hardware of the machine the McHarbor *process* runs on, not
// of the selected environment. A remote Docker host's adapters are not
// visible here, exactly as in core/gpu. Callers must label it as host
// scope rather than presenting it as environment telemetry.
package hardware

import "context"

// Kind classifies a device for the UI, which groups by it.
const (
	KindGPU     = "gpu"
	KindNetwork = "network"
	KindUSB     = "usb"
	KindDisk    = "disk"
)

// Bus names where a device was read from, so the UI can say "usb" next to
// a peripheral and "pci" next to an adapter without inferring it. Not an
// exhaustive enum: an adapter found through `/sys/class/net` reports
// whatever subsystem sysfs names for it (`vmbus`, `platform`, `xen`), and
// inventing a bucket for those would lose information the kernel gave us.
const (
	BusPCI   = "pci"
	BusUSB   = "usb"
	BusBlock = "block"
	BusVMBus = "vmbus"
)

// Device is one piece of host hardware.
//
// Every field past Name and Kind is optional: sysfs is inconsistent about
// what a given subsystem exposes, and a card that reports no driver is
// still a card worth listing. Omitted rather than zeroed where the
// difference matters — a nil Utilization means "not measured", which is
// not the same claim as 0%.
type Device struct {
	// ID is stable for the boot: a PCI address (`0000:01:00.0`), a USB
	// bus path (`1-4.2`), or a block device name (`nvme0n1`).
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Bus  string `json:"bus"`

	// Name is the best human label available: a USB product string, a
	// disk model, a PCI vendor name, or the id when nothing better exists.
	Name string `json:"name"`

	// Model is the secondary identifier shown next to the name —
	// `10de:2484` for PCI and USB, the kernel name for a block device.
	Model  string `json:"model,omitempty"`
	Driver string `json:"driver,omitempty"`

	// Interface is the kernel name of a network adapter — `enp3s0`,
	// `eth0`. It is the identifier an operator actually types, so it is
	// worth carrying even though the PCI walk alone cannot supply it.
	Interface string `json:"interface,omitempty"`

	// SpeedMbps is the negotiated link speed. USB only; 0 when unknown.
	SpeedMbps float64 `json:"speedMbps,omitempty"`

	// Capacity is the size of a block device in bytes.
	Capacity int64 `json:"capacity,omitempty"`

	Removable bool `json:"removable,omitempty"`

	// GPU telemetry, filled in by the caller from nvidia-smi when it is
	// reachable. sysfs alone cannot report any of it.
	MemoryTotal int64    `json:"memoryTotal,omitempty"`
	Utilization *float64 `json:"utilization,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
}

// Inventory is the whole read, split by what an operator asks for.
//
// Available reports whether the enumeration ran at all. It is reported
// separately from three empty lists because the two mean different
// things: "this platform exposes no sysfs" is not "this machine has no
// hardware", and only the first is worth explaining in the UI.
type Inventory struct {
	Available       bool     `json:"available"`
	GPUs            []Device `json:"gpus"`
	NetworkAdapters []Device `json:"networkAdapters"`
	AttachedDevices []Device `json:"attachedDevices"`
}

// Reader enumerates host hardware.
//
// Stateless — the read is a directory walk of a few dozen small files,
// costing well under a millisecond, so there is nothing worth caching and
// no lock to hold. Safe for concurrent use.
type Reader struct{}

// NewReader builds a Reader.
func NewReader() *Reader { return &Reader{} }

// Inventory returns the host's devices.
//
// Never returns an error: an unreadable or absent sysfs is the ordinary
// case on a non-Linux host, and the honest answer there is
// `Available: false` with empty lists rather than a 500 on a page whose
// other twelve sections rendered fine.
func (r *Reader) Inventory(ctx context.Context) Inventory {
	return readInventory(ctx)
}

// emptyInventory is the shape returned when nothing could be read. The
// slices are non-nil so the JSON carries `[]` rather than `null`, which
// the frontend would have to guard separately.
func emptyInventory() Inventory {
	return Inventory{
		Available:       false,
		GPUs:            []Device{},
		NetworkAdapters: []Device{},
		AttachedDevices: []Device{},
	}
}
