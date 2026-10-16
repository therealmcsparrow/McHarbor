// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package vm

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/qmp"
)

// complyWindow is how long a guest is given to act on a plug or unplug.
//
// Both are requests, not commands. A Linux guest with the right udev rules
// takes a moment; a Windows guest often declines outright, and waiting longer
// would not change that — what matters is reporting the refusal rather than
// timing out silently.
const complyWindow = 20 * time.Second

// MemoryResult is what a memory resize actually achieved.
type MemoryResult struct {
	RequestedMB int64
	PluggedMB   int64
	BlockSizeMB int64
	// Complied is false when the guest has not reached the requested size
	// within the wait. On a shrink this is the common outcome and not an
	// error: the guest frees the blocks or it does not.
	Complied bool
}

// ResizeMemory changes how much memory a running guest has.
//
// totalMB is the guest's target total, of which the boot size is fixed: only
// the difference is plugged. The result reports what the device ended up
// holding, rounded to the host's block size, because the requested figure and
// the real one differ and showing only the request makes a refused resize look
// successful.
func (m *Manager) ResizeMemory(ctx context.Context, vmID string, bootMB, totalMB int64) (MemoryResult, error) {
	if totalMB < bootMB {
		return MemoryResult{}, fmt.Errorf(
			"vm: %s cannot go below its boot memory of %d MB", vmID, bootMB)
	}

	monitor, err := m.Monitor(ctx, vmID)
	if err != nil {
		return MemoryResult{}, err
	}

	const mib = 1024 * 1024
	added := (totalMB - bootMB) * mib
	state, err := monitor.ResizeMemory(ctx, added)
	if err != nil {
		return MemoryResult{}, err
	}

	deadline := time.Now().Add(complyWindow)
	for state.PluggedBytes != state.RequestedBytes && time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return MemoryResult{}, ctx.Err()
		case <-time.After(time.Second):
		}
		if state, err = monitor.MemoryState(ctx); err != nil {
			return MemoryResult{}, err
		}
	}

	return MemoryResult{
		RequestedMB: bootMB + state.RequestedBytes/mib,
		PluggedMB:   bootMB + state.PluggedBytes/mib,
		BlockSizeMB: state.BlockSizeBytes / mib,
		Complied:    state.PluggedBytes == state.RequestedBytes,
	}, nil
}

// CPUResult is what a vCPU change actually achieved.
type CPUResult struct {
	RequestedVCPUs int
	ActualVCPUs    int
	Complied       bool
}

// SetVCPUs plugs or unplugs vCPUs to reach a target count.
//
// Unplug is where this gets interesting: device_del only asks, and a Windows
// guest or a Linux one without the right udev rules routinely declines. The
// result reports the count the guest actually ended up with, so a refusal is a
// visible state rather than a silent no-op.
func (m *Manager) SetVCPUs(ctx context.Context, vmID string, target int) (CPUResult, error) {
	if target < 1 {
		return CPUResult{}, fmt.Errorf("vm: %s needs at least one vcpu", vmID)
	}

	lock := m.lockFor(vmID)
	lock.Lock()
	defer lock.Unlock()

	monitor, err := m.Monitor(ctx, vmID)
	if err != nil {
		return CPUResult{}, err
	}

	slots, err := monitor.HotpluggableCPUs(ctx)
	if err != nil {
		return CPUResult{}, fmt.Errorf("vm: reading %s's cpu slots: %w", vmID, err)
	}
	filled, vacant := partitionCPUSlots(slots)

	if target > len(filled)+len(vacant) {
		return CPUResult{}, fmt.Errorf(
			"vm: %s was started with room for %d vcpus; raising the ceiling needs a restart",
			vmID, len(filled)+len(vacant))
	}

	switch {
	case target > len(filled):
		for i := 0; i < target-len(filled) && i < len(vacant); i++ {
			if err := monitor.PlugCPU(ctx, vacant[i], cpuSlotID(vacant[i])); err != nil {
				return CPUResult{}, err
			}
		}
	case target < len(filled):
		// Newest first. The first slots are the boot CPUs, and on most guests
		// vCPU 0 cannot be removed at all.
		for i := len(filled) - 1; i >= target; i-- {
			if err := monitor.DelDevice(ctx, cpuSlotID(filled[i])); err != nil {
				return CPUResult{}, fmt.Errorf("vm: asking %s to release a vcpu: %w", vmID, err)
			}
		}
	}

	actual := m.awaitCPUCount(ctx, monitor, target)
	return CPUResult{
		RequestedVCPUs: target,
		ActualVCPUs:    actual,
		Complied:       actual == target,
	}, nil
}

// awaitCPUCount waits for the guest to settle on a count, and returns whatever
// it settled on.
//
// It never errors: the honest answer to "how many vCPUs does this guest have"
// is the number it reports now, even when that is not the number asked for.
func (m *Manager) awaitCPUCount(ctx context.Context, monitor *qmp.Monitor, target int) int {
	deadline := time.Now().Add(complyWindow)
	actual := 0
	for {
		slots, err := monitor.HotpluggableCPUs(ctx)
		if err == nil {
			filled, _ := partitionCPUSlots(slots)
			actual = len(filled)
			if actual == target {
				return actual
			}
		}
		if time.Now().After(deadline) {
			return actual
		}
		select {
		case <-ctx.Done():
			return actual
		case <-time.After(time.Second):
		}
	}
}

// partitionCPUSlots splits the slot list into filled and vacant, in a stable
// order so that plugging and unplugging walk the topology the same way twice.
func partitionCPUSlots(slots []qmp.HotpluggableCPU) (filled, vacant []qmp.HotpluggableCPU) {
	sort.SliceStable(slots, func(i, j int) bool {
		return cpuSlotID(slots[i]) < cpuSlotID(slots[j])
	})
	for _, slot := range slots {
		if slot.QOMPath != "" {
			filled = append(filled, slot)
			continue
		}
		vacant = append(vacant, slot)
	}
	return filled, vacant
}

// cpuSlotID names a slot by its topology coordinates.
//
// Derived rather than generated so the same slot gets the same id on every
// call: an id that changed between the plug and the unplug would leave a CPU
// nothing can address.
func cpuSlotID(slot qmp.HotpluggableCPU) string {
	return fmt.Sprintf("cpu-%d-%d-%d",
		intProp(slot.Props, "socket-id"),
		intProp(slot.Props, "core-id"),
		intProp(slot.Props, "thread-id"))
}

// intProp reads a numeric slot property. QMP numbers decode as float64 through
// an interface, which is why this is not a plain type assertion to int.
func intProp(props map[string]interface{}, key string) int {
	switch value := props[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case int64:
		return int(value)
	}
	return 0
}

// SetIOThrottle applies a disk's I/O limits to the running guest.
//
// The device id matches what the argv builder emitted, which is why the two
// naming schemes have to stay together: a throttle addressed to an id QEMU
// does not have fails with a message naming neither the guest nor the disk.
func (m *Manager) SetIOThrottle(ctx context.Context, vmID string, disk Disk) error {
	monitor, err := m.Monitor(ctx, vmID)
	if err != nil {
		return err
	}
	return monitor.SetIOThrottle(ctx, fmt.Sprintf("disk%d", disk.Slot),
		disk.ReadIOPS, disk.WriteIOPS, disk.ReadBPS, disk.WriteBPS)
}
