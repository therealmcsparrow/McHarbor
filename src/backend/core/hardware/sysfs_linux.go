// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package hardware

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	pciDevicesDir   = "/sys/bus/pci/devices"
	usbDevicesDir   = "/sys/bus/usb/devices"
	blockDir        = "/sys/block"
	netClassDir     = "/sys/class/net"
	vmbusDevicesDir = "/sys/bus/vmbus/devices"
)

// vmbusNetVSCClassID is Hyper-V's synthetic network adapter.
//
// It matters more than its obscurity suggests: on Docker Desktop and on
// any Hyper-V guest the host's NIC is a vmbus device, not a PCI one, and
// `/sys/class/net` is namespaced away from us — so without this the
// adapters list is empty on a very large share of installs. The class id
// is a stable kernel constant, not a value we chose.
const vmbusNetVSCClassID = "f8615163-df3e-46c5-913f-f2d2f965ed0e"

// vmbusNetVSCName is the device's product name. Not translated, for the
// same reason "Intel Corporation" is not: it names a piece of hardware.
const vmbusNetVSCName = "Hyper-V Network Adapter"

// pciAddressPattern matches a sysfs PCI address (`0000:03:00.0`). Used to
// tell a network interface sitting on PCI — which the PCI walk already
// found — from one on another bus, which it could not have.
var pciAddressPattern = regexp.MustCompile(`^[0-9a-fA-F]{4}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}\.[0-9a-fA-F]+$`)

// PCI base classes, the first byte of the `class` attribute.
const (
	pciClassNetwork = "02"
	pciClassDisplay = "03"
)

// maxDevicesPerGroup bounds each list. A host with a large USB hub tree or
// a storage shelf can enumerate hundreds of nodes, and neither the JSON
// nor the panel that renders it is the right place to find that out. The
// cap is generous enough that no ordinary machine reaches it.
const maxDevicesPerGroup = 128

// readInventory walks the three sysfs subsystems.
//
// Each subsystem is independent: a kernel built without USB support, or a
// container whose `/sys` is masked in part, must not cost us the parts
// that did read. Availability is therefore the OR of the three, not the
// AND.
func readInventory(ctx context.Context) Inventory {
	inv := emptyInventory()

	interfaceByPCI, offBusNICs, netOK := readNetInterfaces()

	gpus, nics, pciOK := readPCIDevices(interfaceByPCI)
	adapters := append(nics, offBusNICs...)
	adapters = append(adapters, unseenVMBusNICs(adapters)...)

	inv.GPUs = gpus
	inv.NetworkAdapters = adapters
	capGroup(&inv.NetworkAdapters)

	if ctx.Err() != nil {
		inv.Available = pciOK || netOK
		return inv
	}

	usb, usbOK := readUSBDevices()
	attached := usb

	if ctx.Err() == nil {
		disks, diskOK := readBlockDevices()
		attached = append(attached, disks...)
		usbOK = usbOK || diskOK
	}

	if len(attached) > maxDevicesPerGroup {
		attached = attached[:maxDevicesPerGroup]
	}
	inv.AttachedDevices = attached
	inv.Available = pciOK || netOK || usbOK
	return inv
}

// readPCIDevices returns the display and network adapters on the PCI bus.
//
// The network adapters come from here rather than from `/sys/class/net`
// on purpose: net devices are network-namespaced, so inside a container
// that directory lists the container's own veth pair and loopback — the
// exact opposite of "physical adapter". The PCI bus is not namespaced.
//
// interfaceByPCI supplies the kernel interface names, which PCI alone
// cannot know; it is empty when the process cannot see the host's netns,
// and every adapter is still listed without one.
func readPCIDevices(interfaceByPCI map[string]string) (gpus, nics []Device, ok bool) {
	entries, err := os.ReadDir(pciDevicesDir)
	if err != nil {
		return []Device{}, []Device{}, false
	}

	gpus = make([]Device, 0, 2)
	nics = make([]Device, 0, 4)

	for _, entry := range entries {
		address := entry.Name()
		dir := filepath.Join(pciDevicesDir, address)

		class := trimHex(readAttr(dir, "class"))
		if len(class) < 2 {
			continue
		}
		baseClass := class[:2]
		if baseClass != pciClassDisplay && baseClass != pciClassNetwork {
			continue
		}

		vendorID := trimHex(readAttr(dir, "vendor"))
		deviceID := trimHex(readAttr(dir, "device"))

		name := pciVendorName(vendorID)
		if name == "" {
			// Unmapped vendor: the address is a worse label than the ids,
			// but it is at least unique, and Model still carries the ids.
			name = "PCI " + address
		}

		device := Device{
			ID:     address,
			Bus:    BusPCI,
			Name:   name,
			Model:  hexPair(vendorID, deviceID),
			Driver: linkTarget(filepath.Join(dir, "driver")),
		}

		if baseClass == pciClassDisplay {
			device.Kind = KindGPU
			gpus = append(gpus, device)
			continue
		}
		device.Kind = KindNetwork
		device.Interface = interfaceByPCI[address]
		nics = append(nics, device)
	}

	capGroup(&gpus)
	capGroup(&nics)
	return gpus, nics, true
}

// readNetInterfaces reads `/sys/class/net` for what only it knows.
//
// Two answers come back. The map gives the kernel interface name for each
// PCI address, so a card the PCI walk found can be labelled `enp3s0`
// rather than by address alone. The slice carries adapters that are *not*
// on PCI — Hyper-V vmbus NICs, SoC platform NICs, USB ethernet — which
// the PCI walk cannot see at all.
//
// The filter is the `device` symlink. Every virtual interface (lo, veth,
// bridges, docker0, tun) lacks one; every hardware-backed interface has
// one. That single test is what keeps a container's own veth out of a
// list titled "physical network adapters".
func readNetInterfaces() (interfaceByPCI map[string]string, offBus []Device, ok bool) {
	entries, err := os.ReadDir(netClassDir)
	if err != nil {
		return map[string]string{}, []Device{}, false
	}

	interfaceByPCI = make(map[string]string, len(entries))
	offBus = make([]Device, 0, 1)

	for _, entry := range entries {
		iface := entry.Name()
		dir := filepath.Join(netClassDir, iface)

		target, err := os.Readlink(filepath.Join(dir, "device"))
		if err != nil {
			continue
		}
		deviceID := filepath.Base(target)

		if pciAddressPattern.MatchString(deviceID) {
			interfaceByPCI[deviceID] = iface
			continue
		}

		offBus = append(offBus, Device{
			ID:        iface,
			Kind:      KindNetwork,
			Bus:       linkTarget(filepath.Join(dir, "device/subsystem")),
			Name:      iface,
			Model:     deviceID,
			Interface: iface,
			Driver:    linkTarget(filepath.Join(dir, "device/driver")),
		})
	}

	return interfaceByPCI, offBus, true
}

// unseenVMBusNICs returns the Hyper-V synthetic network adapters that
// `/sys/class/net` did not already account for.
//
// On a Hyper-V guest where we *can* see the host netns, the interface walk
// finds the same adapter and knows its kernel name, which is the better
// row — so those are filtered out here by the vmbus GUID it recorded as
// Model. Inside a container, that walk sees nothing and these are the only
// evidence the machine has a NIC at all.
func unseenVMBusNICs(known []Device) []Device {
	entries, err := os.ReadDir(vmbusDevicesDir)
	if err != nil {
		return nil
	}

	seen := make(map[string]bool, len(known))
	for _, device := range known {
		seen[device.ID] = true
		seen[device.Model] = true
	}

	var adapters []Device
	for _, entry := range entries {
		node := entry.Name()
		if seen[node] {
			continue
		}

		dir := filepath.Join(vmbusDevicesDir, node)
		// The attribute is brace-wrapped: `{f8615163-...}`.
		classID := strings.Trim(strings.ToLower(readAttr(dir, "class_id")), "{}")
		if classID != vmbusNetVSCClassID {
			continue
		}

		adapters = append(adapters, Device{
			ID:     node,
			Kind:   KindNetwork,
			Bus:    BusVMBus,
			Name:   vmbusNetVSCName,
			Model:  node,
			Driver: linkTarget(filepath.Join(dir, "driver")),
		})
	}
	return adapters
}

// readUSBDevices returns the attached USB peripherals.
func readUSBDevices() ([]Device, bool) {
	entries, err := os.ReadDir(usbDevicesDir)
	if err != nil {
		return []Device{}, false
	}

	devices := make([]Device, 0, 8)
	for _, entry := range entries {
		node := entry.Name()

		// `1-4:1.0` is an interface — one function of a device already
		// listed under `1-4`. `usb1` is a root hub, which is the host
		// controller rather than anything a person plugged in.
		if strings.Contains(node, ":") || strings.HasPrefix(node, "usb") {
			continue
		}

		dir := filepath.Join(usbDevicesDir, node)
		vendorID := trimHex(readAttr(dir, "idVendor"))
		productID := trimHex(readAttr(dir, "idProduct"))
		if vendorID == "" {
			continue
		}

		name := strings.TrimSpace(strings.Join(
			nonEmpty(readAttr(dir, "manufacturer"), readAttr(dir, "product")), " "))
		if name == "" {
			name = "USB " + hexPair(vendorID, productID)
		}

		devices = append(devices, Device{
			ID:        node,
			Kind:      KindUSB,
			Bus:       BusUSB,
			Name:      name,
			Model:     hexPair(vendorID, productID),
			SpeedMbps: parseFloat(readAttr(dir, "speed")),
			// USB reports `removable` as one of removable/fixed/unknown.
			// Only the first is a claim; the other two are not.
			Removable: readAttr(dir, "removable") == "removable",
		})
	}

	sortByID(devices)
	return devices, true
}

// readBlockDevices returns the host's real storage devices.
//
// The virtual ones are dropped: loop devices are mounted image files,
// ram/zram are memory, dm-* are mapper targets over something already
// listed, and md/nbd are assemblies rather than attachments. Listing any
// of them would double-count the same physical disk.
func readBlockDevices() ([]Device, bool) {
	entries, err := os.ReadDir(blockDir)
	if err != nil {
		return []Device{}, false
	}

	devices := make([]Device, 0, 4)
	for _, entry := range entries {
		node := entry.Name()
		if isVirtualBlockDevice(node) {
			continue
		}

		dir := filepath.Join(blockDir, node)
		name := strings.TrimSpace(strings.Join(
			nonEmpty(readAttr(dir, "device/vendor"), readAttr(dir, "device/model")), " "))
		if name == "" {
			name = node
		}

		devices = append(devices, Device{
			ID:   node,
			Kind: KindDisk,
			Bus:  BusBlock,
			Name: name,
			// The kernel name is the secondary identifier here: it is what
			// `lsblk`, fstab and every mount error message call this disk.
			Model: node,
			// `size` is always in 512-byte sectors, whatever the device's
			// own logical block size is.
			Capacity:  parseInt(readAttr(dir, "size")) * 512,
			Removable: readAttr(dir, "removable") == "1",
		})
	}

	sortByID(devices)
	return devices, true
}

// virtualBlockPrefixes are kernel names that never denote attached
// hardware. `sr` (optical) is deliberately absent — a drive is a drive.
var virtualBlockPrefixes = []string{"loop", "ram", "zram", "dm-", "md", "nbd", "fd", "zd"}

func isVirtualBlockDevice(node string) bool {
	for _, prefix := range virtualBlockPrefixes {
		if strings.HasPrefix(node, prefix) {
			return true
		}
	}
	return false
}

// readAttr reads one sysfs attribute. Missing and unreadable are the same
// answer here — "" — because sysfs omits attributes a driver does not
// implement, and that is not a failure worth propagating.
func readAttr(dir, name string) string {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// linkTarget returns the base name of a sysfs symlink, e.g. the bound
// kernel driver `igc` behind `<device>/driver`.
func linkTarget(path string) string {
	target, err := os.Readlink(path)
	if err != nil {
		return ""
	}
	return filepath.Base(target)
}

// trimHex normalises a sysfs hex attribute (`0x8086`) to bare lowercase.
func trimHex(raw string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(raw), "0x"))
}

// hexPair renders the `vendor:device` identifier, tolerating a missing
// half rather than emitting a dangling colon.
func hexPair(vendorID, deviceID string) string {
	switch {
	case vendorID == "":
		return deviceID
	case deviceID == "":
		return vendorID
	default:
		return vendorID + ":" + deviceID
	}
}

func nonEmpty(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func parseFloat(raw string) float64 {
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	return value
}

func parseInt(raw string) int64 {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0
	}
	return value
}

func sortByID(devices []Device) {
	sort.Slice(devices, func(i, j int) bool { return devices[i].ID < devices[j].ID })
}

func capGroup(devices *[]Device) {
	sortByID(*devices)
	if len(*devices) > maxDevicesPerGroup {
		*devices = (*devices)[:maxDevicesPerGroup]
	}
}
