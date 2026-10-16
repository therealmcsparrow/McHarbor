// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package main

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/therealmcsparrow/mcharbor-agent/compute/capability"
)

// handleQEMUCapability answers vm_qemu_capability from the cached probe.
//
// It never starts a guest and never re-probes on its own: the probe is already
// refreshed on connect, on demand and on a six-hour timer, so answering from
// the cache keeps a UI that asks per page-load from running a subprocess per
// page-load.
func (rt *computeRuntime) handleQEMUCapability(ctx context.Context, reply *VMPayload) error {
	report := rt.Capability(ctx, false)
	binary, ok := hostQEMUBinary(report)
	if !ok {
		return fmt.Errorf("compute: no qemu-system binary for %s on this host", runtime.GOARCH)
	}

	out := &VMQEMUCapability{
		Binary:       binary.Path,
		Arch:         binary.Arch,
		Version:      binary.Version,
		MachineTypes: binary.MachineTypes,
		Devices:      binary.Devices,
		CPUModels:    binary.CPUModels,
		QMPCommands:  binary.QMPCommands,
	}
	if report.HugePages != nil {
		out.HugePages = &ComputeHugePages{
			Total:    report.HugePages.Total,
			Free:     report.HugePages.Free,
			SizeKB:   report.HugePages.SizeKB,
			Surplus:  report.HugePages.Surplus,
			Reserved: report.HugePages.Reserved,
		}
	}
	for _, node := range report.NUMANodes {
		out.NUMANodes = append(out.NUMANodes, ComputeNUMANode{
			ID:          node.ID,
			CPUs:        node.CPUs,
			MemoryBytes: node.MemoryBytes,
			FreeBytes:   node.FreeBytes,
		})
	}
	out.Features = deriveVMFeatures(out)
	reply.QEMUCapability = out
	return nil
}

// hostQEMUBinary picks the emulator for the host's own architecture.
//
// A foreign-architecture binary can only do TCG, and McHarbor refuses TCG
// rather than offering a guest that runs at a fiftieth of the speed — so its
// capabilities would describe something the operator can never start.
func hostQEMUBinary(report capability.Report) (capability.QEMUBinary, bool) {
	for _, binary := range report.QEMUBinaries {
		if binary.KVMAccel {
			return binary, true
		}
	}
	return capability.QEMUBinary{}, false
}

// vmFeatureDisplayResize is the capability key for "the console can ask this
// host's guests to change resolution". It travels in the existing Features map
// rather than as a protocol.go constant, so adding it changes no shared type and
// no plugin binary that links the host's agent package.
const vmFeatureDisplayResize = "display_resize"

// deriveVMFeatures answers the questions the UI actually asks.
//
// A feature is almost never one capability: virtio-mem needs the device and
// the qom-set verb, and hugepages need a host pool as well as a QEMU that can
// back memory with a file. Collapsing them here keeps that knowledge on the
// side that can see the host, instead of in six frontend conditionals.
func deriveVMFeatures(in *VMQEMUCapability) map[string]bool {
	devices := indexOf(in.Devices)
	commands := indexOf(in.QMPCommands)

	hugePages := in.HugePages != nil && in.HugePages.Total > 0
	return map[string]bool{
		VMFeatureVirtioMem:   devices["virtio-mem-pci"] && commands["qom-set"],
		VMFeatureCPUHotplug:  commands["query-hotpluggable-cpus"] && commands["device_add"],
		VMFeatureHugePages:   hugePages,
		VMFeatureNUMA:        len(in.NUMANodes) > 1,
		VMFeatureVirtioFS:    devices["vhost-user-fs-pci"],
		VMFeatureVSock:       devices["vhost-vsock-pci"],
		VMFeatureIOThrottle:  commands["block_set_io_throttle"],
		VMFeatureIOThread:    devices["virtio-blk-pci"] || devices["virtio-scsi-pci"],
		VMFeatureDirtyBitmap: commands["block-dirty-bitmap-add"],
		VMFeatureBlockdevBkp: commands["blockdev-backup"],
		VMFeatureVirtioGPU:   devices["virtio-gpu-pci"] || devices["virtio-vga"],
		VMFeatureVirtioSnd:   devices["virtio-sound-pci"] || devices["virtio-snd-pci"],
		VMFeatureUSBU2F:      devices["u2f-passthru"] || devices["u2f-emulated"],
		// Whether the console can resize a guest at all. Which guest can is
		// then its own adapter's answer, which the plugin already stores.
		vmFeatureDisplayResize: displayResizeSupported(in.Version, devices),

		// query-spice exists only in a QEMU built with SPICE, which is the one
		// thing that decides whether -spice would start or refuse to.
		VMFeatureSPICE: commands["query-spice"],
		// Answered by this agent's own code rather than by QEMU, so they are
		// true on every host this build runs on. Their presence is the probe.
		VMFeatureClone:        true,
		VMFeatureMigrateCold:  true,
		VMFeatureSerialResize: true,
		VMFeatureMigrateLive: commands["migrate"] && commands["migrate-incoming"] &&
			commands["nbd-server-start"] && commands["blockdev-mirror"] && commands["object-add"],
		// Live disk, disc and NIC changes: every verb the hot-plug engine sends,
		// and the root port the q35 layout plugs into.
		VMFeatureHotplug: devices["pcie-root-port"] && commands["device_add"] && commands["device_del"] &&
			commands["blockdev-add"] && commands["blockdev-del"] && commands["block_resize"] &&
			commands["blockdev-change-medium"] && commands["eject"] &&
			commands["netdev_add"] && commands["netdev_del"] && commands["set_link"] &&
			commands["qom-list"] && commands["query-pci"],
	}
}

// spiceSupported reports whether this host's QEMU can serve SPICE. The probe
// is cached, so asking on every start costs nothing.
func (rt *computeRuntime) spiceSupported(ctx context.Context) bool {
	binary, ok := hostQEMUBinary(rt.Capability(ctx, false))
	return ok && indexOf(binary.QMPCommands)["query-spice"]
}

// indexOf turns a sorted capability list into a set, lowercased so a QEMU that
// changes a name's case does not silently disable a feature.
func indexOf(list []string) map[string]bool {
	out := make(map[string]bool, len(list))
	for _, item := range list {
		out[strings.ToLower(item)] = true
	}
	return out
}
