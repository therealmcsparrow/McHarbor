// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// vsockFirstCID is the lowest context id a guest may take.
//
// The kernel reserves 0 (hypervisor), 1 (local) and 2 (host), and a guest
// given one of them simply never receives anything.
const vsockFirstCID = 3

// vsockMaxCID bounds the search. The field is 32-bit, but a host with ten
// thousand guests on it has larger problems, and an unbounded loop against a
// unique index is a way to hang a request.
const vsockMaxCID = 10000

// allocateVSockCID picks the lowest free context id on a host.
//
// Lowest-free rather than random so the ids stay readable, and derived from
// what is stored rather than from a counter so a deleted guest returns its id.
// The unique index on (environment_id, vsock_cid) is what actually prevents a
// collision; this only avoids provoking one.
func (s *Service) allocateVSockCID(ctx context.Context, envID string) (int, error) {
	rows, err := s.db.Query(ctx, `
		SELECT vsock_cid FROM plugin_addon_kvm_vms
		 WHERE environment_id = ? AND vsock_cid IS NOT NULL
		 ORDER BY vsock_cid
		 LIMIT 1000`, envID)
	if err != nil {
		return 0, fmt.Errorf("reading allocated context ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	taken := map[int]bool{}
	for rows.Next() {
		var cid int
		if err := rows.Scan(&cid); err != nil {
			return 0, fmt.Errorf("reading allocated context ids: %w", err)
		}
		taken[cid] = true
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("reading allocated context ids: %w", err)
	}

	for cid := vsockFirstCID; cid < vsockMaxCID; cid++ {
		if !taken[cid] {
			return cid, nil
		}
	}
	return 0, fmt.Errorf("this host has no free guest context ids left")
}

// validateFeatureSet refuses combinations QEMU will not run.
//
// Checked on the merged row rather than on the patch: virtio-mem without a
// ceiling, or a NUMA policy with no nodes, are only visible once the change is
// applied to what is already there.
func validateFeatureSet(vm VM) error {
	if vm.VirtioMem && vm.MemoryMaxMB <= vm.MemoryMB {
		return invalid(codeInvalidResize,
			"resizable memory needs a memory ceiling above the boot size")
	}
	if vm.VCPUsMax > 0 && vm.VCPUsMax < vm.VCPUs {
		return invalid(codeInvalidResize,
			"the processor ceiling cannot be below the processor count")
	}
	if !numaPolicies[vm.NUMAPolicy] {
		return invalid(codeInvalidResize, "that is not a NUMA policy this host can apply")
	}
	if vm.NUMAPolicy == "mirror" && len(vm.NUMAHostNodes) < 2 {
		return invalid(codeInvalidResize,
			"mirroring the host's layout needs at least two host nodes")
	}
	return nil
}

// UpdateDiskFlags changes a disk's sharing and its place in the boot order.
//
// Both are read while building the command line, so both need a stopped guest:
// changing them under a running QEMU would leave the UI describing a machine
// other than the one executing.
func (s *Service) UpdateDiskFlags(ctx context.Context, vmID, diskID string, shareRW *bool, bootIndex *int) (Disk, error) {
	vm, err := s.Get(ctx, vmID)
	if err != nil {
		return Disk{}, err
	}
	if vm.State != StateStopped {
		return Disk{}, ErrRunning
	}
	disk, found := findDisk(vm.Disks, diskID)
	if !found {
		return Disk{}, ErrNotFound
	}

	if shareRW != nil {
		if *shareRW && disk.IsCDROM {
			return Disk{}, invalid(codeBadRequest, "a disc cannot be shared read-write")
		}
		disk.ShareRW = *shareRW
	}
	if bootIndex != nil {
		if *bootIndex < 0 || *bootIndex > 99 {
			return Disk{}, invalid(codeBadRequest, "a boot position must be between 0 and 99")
		}
		disk.BootIndex = *bootIndex
	}

	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_kvm_disks
		   SET share_rw = ?, boot_index = ?, updated_at = ?
		 WHERE id = ? AND vm_id = ?`,
		boolToInt(disk.ShareRW), nullIfZero(disk.BootIndex),
		time.Now().UTC().Format(time.RFC3339), diskID, vmID); err != nil {
		return Disk{}, fmt.Errorf("updating the disk: %w", err)
	}
	return disk, nil
}

// encodeNodes stores a node list as JSON, or NULL when there is none.
func encodeNodes(nodes []int) any {
	if len(nodes) == 0 {
		return nil
	}
	encoded, err := json.Marshal(nodes)
	if err != nil {
		// Cannot happen for a slice of ints; storing NULL rather than a
		// half-written value keeps the column readable either way.
		return nil
	}
	return string(encoded)
}
