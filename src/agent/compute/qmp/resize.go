// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package qmp

import (
	"context"
	"fmt"
)

// virtioMemID is the qdev id the argv builder gives every guest's virtio-mem
// device. Resizing addresses it by path, so the two have to agree.
const virtioMemID = "virtiomem0"

// QOMSet writes one QOM property.
func (m *Monitor) QOMSet(ctx context.Context, path, property string, value interface{}) error {
	_, err := m.Run(ctx, "qom-set", map[string]interface{}{
		"path": path, "property": property, "value": value,
	})
	return err
}

// QOMGetInt reads one integer QOM property.
func (m *Monitor) QOMGetInt(ctx context.Context, path, property string) (int64, error) {
	var out int64
	err := m.RunInto(ctx, "qom-get", map[string]interface{}{
		"path": path, "property": property,
	}, &out)
	return out, err
}

// MemoryState is what a virtio-mem device was asked for and what it has.
//
// The two differ until the guest complies, and on a shrink they may differ
// forever: unplug depends entirely on the guest being able to free the blocks.
// Reporting only the request is how a resize looks like it worked when it did
// not.
type MemoryState struct {
	RequestedBytes int64
	PluggedBytes   int64
	BlockSizeBytes int64
}

// ResizeMemory asks a guest's virtio-mem device for a new size.
//
// The size is what the device adds on top of the boot memory, not the guest's
// total. Requests are rounded up to the host's block size — 2 MiB normally,
// larger with hugepages — so the caller reports the rounded figure back rather
// than the number the operator typed.
func (m *Monitor) ResizeMemory(ctx context.Context, addedBytes int64) (MemoryState, error) {
	path := "/machine/peripheral/" + virtioMemID
	if err := m.QOMSet(ctx, path, "requested-size", addedBytes); err != nil {
		return MemoryState{}, fmt.Errorf("qmp: requesting %s of memory: %w",
			humanBytes(addedBytes), err)
	}
	return m.MemoryState(ctx)
}

// MemoryState reads a guest's virtio-mem device.
func (m *Monitor) MemoryState(ctx context.Context) (MemoryState, error) {
	path := "/machine/peripheral/" + virtioMemID

	requested, err := m.QOMGetInt(ctx, path, "requested-size")
	if err != nil {
		return MemoryState{}, fmt.Errorf("qmp: reading the requested memory size: %w", err)
	}
	plugged, err := m.QOMGetInt(ctx, path, "size")
	if err != nil {
		return MemoryState{}, fmt.Errorf("qmp: reading the plugged memory size: %w", err)
	}
	// Block size is informational and older builds do not expose it, so a
	// failure here leaves it zero rather than failing the whole read.
	blockSize, err := m.QOMGetInt(ctx, path, "block-size")
	if err != nil {
		blockSize = 0
	}

	return MemoryState{
		RequestedBytes: requested,
		PluggedBytes:   plugged,
		BlockSizeBytes: blockSize,
	}, nil
}

// HotpluggableCPU is one slot from query-hotpluggable-cpus.
//
// QOMPath is the field that says whether the slot is filled: an empty path is
// a vacant socket/core/thread, a populated one names the CPU object already
// there. Props is passed back verbatim to device_add, because the exact key
// set differs per machine type and inventing it is how a plug silently lands
// in the wrong slot.
type HotpluggableCPU struct {
	Type      string                 `json:"type"`
	VCPUCount int                    `json:"vcpus-count"`
	QOMPath   string                 `json:"qom-path,omitempty"`
	Props     map[string]interface{} `json:"props"`
}

// HotpluggableCPUs lists the guest's CPU slots, filled and vacant.
func (m *Monitor) HotpluggableCPUs(ctx context.Context) ([]HotpluggableCPU, error) {
	var out []HotpluggableCPU
	err := m.RunInto(ctx, "query-hotpluggable-cpus", nil, &out)
	return out, err
}

// PlugCPU fills one vacant slot.
//
// The id is derived from the slot's own properties so an unplug can address it
// later: a CPU added without a stable id can be seen but never removed.
func (m *Monitor) PlugCPU(ctx context.Context, slot HotpluggableCPU, id string) error {
	args := map[string]interface{}{"driver": slot.Type, "id": id}
	for key, value := range slot.Props {
		args[key] = value
	}
	if err := m.AddDevice(ctx, args); err != nil {
		return fmt.Errorf("qmp: plugging a vcpu into %s: %w", id, err)
	}
	return nil
}

// SetIOThrottle applies I/O limits to a running guest's disk.
//
// Every field is sent on every call, including the zeroes: QMP replaces the
// whole limit set rather than merging, so omitting a field that was previously
// set would leave it in place and make a "no limit" edit do nothing.
func (m *Monitor) SetIOThrottle(ctx context.Context, deviceID string, readIOPS, writeIOPS, readBPS, writeBPS int64) error {
	_, err := m.Run(ctx, "block_set_io_throttle", map[string]interface{}{
		"id":        deviceID,
		"iops":      0,
		"iops_rd":   readIOPS,
		"iops_wr":   writeIOPS,
		"bps":       0,
		"bps_rd":    readBPS,
		"bps_wr":    writeBPS,
		"iops_size": 0,
	})
	if err != nil {
		return fmt.Errorf("qmp: setting I/O limits on %s: %w", deviceID, err)
	}
	return nil
}

// humanBytes renders a byte count in binary units, as `df -h` does.
func humanBytes(value int64) string {
	if value <= 0 {
		return "0 B"
	}
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	size := float64(value)
	unit := 0
	for size >= 1024 && unit < len(units)-1 {
		size /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d B", value)
	}
	return fmt.Sprintf("%.1f %s", size, units[unit])
}
