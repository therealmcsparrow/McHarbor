// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

// Package hostdev enumerates the devices on a host that can be handed to a
// guest, so an operator picks one from a list instead of typing a PCI address
// from memory.
//
// Everything here is read from sysfs. Nothing is rebound, unbound or otherwise
// changed: deciding a device is attachable is a report, and taking it from the
// host is a separate, deliberate act.
package hostdev

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Device is one attachable host device.
type Device struct {
	Kind        string
	Address     string
	Description string
	Vendor      string
	Product     string
	Driver      string
	Class       string
	IOMMUGroup  string
	Assignable  bool
	Reason      string
}

// maxDevices bounds the sweep. A host with more entries than this has
// something wrong with it, and the picker would be unusable anyway.
const maxDevices = 512

// hostDrivers are the bindings that mean the host is actively using a device.
// vfio-pci is the opposite: a device already bound to it has been set aside
// for passthrough, so it is the one binding that makes a device ready.
var hostDrivers = map[string]bool{
	"nvme": true, "ahci": true, "xhci_hcd": true, "ehci-pci": true,
	"e1000e": true, "igb": true, "ixgbe": true, "r8169": true, "iwlwifi": true,
	"nouveau": true, "amdgpu": true, "i915": true, "radeon": true,
}

// List returns every PCI and USB device on this host, each marked with whether
// it can be assigned and, when it cannot, why.
func List() ([]Device, error) {
	out := make([]Device, 0, 64)
	out = append(out, listPCI()...)
	out = append(out, listUSB()...)

	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Address < out[j].Address
	})
	if len(out) > maxDevices {
		out = out[:maxDevices]
	}
	return out, nil
}

// listPCI walks /sys/bus/pci/devices.
func listPCI() []Device {
	const root = "/sys/bus/pci/devices"
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}

	devices := make([]Device, 0, len(entries))
	for _, entry := range entries {
		base := filepath.Join(root, entry.Name())
		device := Device{
			Kind:    "pci",
			Address: entry.Name(),
			Vendor:  readID(filepath.Join(base, "vendor")),
			Product: readID(filepath.Join(base, "device")),
			Class:   readID(filepath.Join(base, "class")),
			Driver:  driverName(filepath.Join(base, "driver")),
		}
		if group, err := filepath.EvalSymlinks(filepath.Join(base, "iommu_group")); err == nil {
			device.IOMMUGroup = filepath.Base(group)
		}
		device.Description = strings.TrimSpace(fmt.Sprintf("%s %s:%s",
			pciClassName(device.Class), device.Vendor, device.Product))
		device.Assignable, device.Reason = pciAssignable(device)
		devices = append(devices, device)
	}
	return devices
}

// pciAssignable applies the two rules that decide a PCI device's fate.
//
// No IOMMU group means the kernel cannot isolate it, so passthrough is not
// possible at all — that is a firmware or boot-parameter problem, not something
// the UI can work around. A host driver means the device is in use; it can be
// freed, but doing so is an act the operator has to take deliberately.
func pciAssignable(device Device) (bool, string) {
	if device.IOMMUGroup == "" {
		return false, "no_iommu_group"
	}
	if device.Driver == "vfio-pci" {
		return true, ""
	}
	if device.Driver != "" && hostDrivers[device.Driver] {
		return false, "bound_to_host_driver"
	}
	return true, ""
}

// listUSB walks /sys/bus/usb/devices, skipping the hubs and the per-interface
// entries — neither is something anyone passes through.
func listUSB() []Device {
	const root = "/sys/bus/usb/devices"
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}

	devices := make([]Device, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "usb") || strings.Contains(name, ":") {
			continue
		}
		base := filepath.Join(root, name)
		vendor := readTrimmed(filepath.Join(base, "idVendor"))
		product := readTrimmed(filepath.Join(base, "idProduct"))
		if vendor == "" || product == "" {
			continue
		}
		// A root hub reports class 09; passing one through would take every
		// port with it, including whatever the host is booting from.
		if readTrimmed(filepath.Join(base, "bDeviceClass")) == "09" {
			continue
		}

		label := strings.TrimSpace(strings.Join([]string{
			readTrimmed(filepath.Join(base, "manufacturer")),
			readTrimmed(filepath.Join(base, "product")),
		}, " "))
		if label == "" {
			label = vendor + ":" + product
		}
		devices = append(devices, Device{
			Kind:        "usb",
			Address:     vendor + ":" + product,
			Description: label,
			Vendor:      vendor,
			Product:     product,
			Assignable:  true,
		})
	}
	return devices
}

// driverName resolves the driver a device is bound to, empty when unbound.
func driverName(link string) string {
	resolved, err := filepath.EvalSymlinks(link)
	if err != nil {
		return ""
	}
	return filepath.Base(resolved)
}

// readID reads a sysfs hex id and strips the 0x sysfs writes in front of it.
func readID(path string) string {
	return strings.TrimPrefix(readTrimmed(path), "0x")
}

// readTrimmed reads a sysfs attribute, empty for anything unreadable.
func readTrimmed(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// pciClassName names the handful of classes worth passing through. Anything
// else keeps its raw class code, which is still more use than nothing.
func pciClassName(class string) string {
	if len(class) < 4 {
		return class
	}
	switch class[:2] {
	case "03":
		return "VGA"
	case "01":
		return "Storage"
	case "02":
		return "Network"
	case "0c":
		return "USB controller"
	case "04":
		return "Multimedia"
	default:
		return "Class " + class[:2]
	}
}
