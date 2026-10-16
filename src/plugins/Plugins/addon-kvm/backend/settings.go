// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"fmt"
	"strings"
)

// hotSettings can be changed at any time. None of them reaches the running QEMU
// process: they are McHarbor's own bookkeeping and the host's autostart policy.
var hotSettings = []string{
	"name",
	"description",
	"autostart",
	"autostartOrder",
	"autostartDelaySeconds",
	// Read only when the guest starts, so a running guest keeps the profile it
	// booted with and the UI says the change applies at the next start.
	"displayProfileId",
}

// coldSettings are read while building the QEMU command line, so changing one
// on a running guest would leave the UI describing a machine that is not the
// one executing. They apply at the next start.
var coldSettings = []string{
	"vcpus",
	"memoryMb",
	"memoryMaxMb",
	"cpuModel",
	"machineType",
	"firmware",
	"secureBoot",
	"tpmEnabled",
	"displayType",
	"displayWidth",
	"displayHeight",
	"vcpusMax",
	"virtioMem",
	"hugePages",
	"numaPolicy",
	"ioThread",
	"vsockEnabled",
}

// numaPolicies are the three the agent knows how to emit. Anything else would
// fall through to no topology at all, silently ignoring what was chosen.
var numaPolicies = map[string]bool{"": true, "none": true, "pin": true, "mirror": true}

// editableSettings lists what may be changed for a guest in this state.
//
// Anything but a confirmed stop gets the hot list: a guest whose host is
// unreachable reports its state as unknown, and unknown is not permission to
// assume it is stopped.
func editableSettings(state string) []string {
	if state != StateStopped {
		return append([]string(nil), hotSettings...)
	}
	return append(append([]string(nil), hotSettings...), coldSettings...)
}

// displayTypes are the adapters displayArgs knows how to wire on the agent. An
// unknown value there falls through to plain VGA, which would silently ignore
// what the operator picked.
var displayTypes = map[string]bool{"": true, "std": true, "qxl": true, "virtio-gpu": true, "none": true}

// validateVCPUs bounds the processor count for both create and update.
func validateVCPUs(vcpus int) error {
	if vcpus < 1 || vcpus > 512 {
		return invalid(codeVCPUsInvalid, fmt.Sprintf("%d is not a usable number of processors", vcpus))
	}
	return nil
}

// validateMemory checks the starting memory and the ceiling together, which is
// the only way the ordering between them can be seen.
func validateMemory(memoryMB, memoryMaxMB int64) error {
	if memoryMB < 128 {
		return invalid(codeMemoryTooSmall, "a virtual machine needs at least 128 MB of memory")
	}
	if memoryMaxMB != 0 && memoryMaxMB < memoryMB {
		return invalid(codeMemoryCeiling, "the memory ceiling cannot be below the starting memory")
	}
	return nil
}

// validateFirmware checks the firmware and Secure Boot as one fact: Secure Boot
// is a UEFI feature and means nothing under BIOS.
func validateFirmware(firmware string, secureBoot bool) error {
	switch firmware {
	case "uefi", "bios":
	default:
		return invalid(codeFirmwareInvalid, fmt.Sprintf("%q is not a firmware", firmware))
	}
	if secureBoot && firmware != "uefi" {
		return invalid(codeSecureBootNeedsUEFI, "Secure Boot needs UEFI firmware")
	}
	return nil
}

// validateDisplayType rejects an adapter the agent would silently replace.
func validateDisplayType(display string) error {
	if !displayTypes[display] {
		return invalid(codeDisplayInvalid, fmt.Sprintf("%q is not a display adapter", display))
	}
	return nil
}

// Boot resolution bounds: VGA's smallest mode up to 8K, the largest a virtio-gpu
// scanout or a QXL surface is worth asking for.
const (
	minDisplayWidth  = 640
	minDisplayHeight = 480
	maxDisplayWidth  = 7680
	maxDisplayHeight = 4320
)

// validateDisplayResolution accepts QEMU's default (both zero) or a size inside
// the bounds. One side alone is refused rather than paired with a default the
// operator never saw.
func validateDisplayResolution(width, height int) error {
	if width == 0 && height == 0 {
		return nil
	}
	if width < minDisplayWidth || width > maxDisplayWidth ||
		height < minDisplayHeight || height > maxDisplayHeight {
		return invalid(codeDisplayResolution, fmt.Sprintf("%dx%d is not a usable boot resolution", width, height))
	}
	return nil
}

// validateCPUModel keeps the value to something that can sit on a QEMU command
// line without the escaping having to be trusted.
func validateCPUModel(model string) error {
	if model == "" {
		return nil
	}
	if len(model) > 64 {
		return invalid(codeCPUModelInvalid, "that CPU model name is too long")
	}
	for _, r := range model {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.', r == '+':
		default:
			return invalid(codeCPUModelInvalid, fmt.Sprintf("a CPU model cannot contain %q", r))
		}
	}
	return nil
}

// validateMachineType accepts the QEMU machine names, which carry digits, dots
// and dashes: pc-q35-9.0, virt-8.2.
func validateMachineType(machine string) error {
	if strings.TrimSpace(machine) == "" {
		return invalid(codeMachineInvalid, "a machine type is required")
	}
	if len(machine) > 64 {
		return invalid(codeMachineInvalid, "that machine type is too long")
	}
	for _, r := range machine {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.':
		default:
			return invalid(codeMachineInvalid, fmt.Sprintf("a machine type cannot contain %q", r))
		}
	}
	return nil
}
