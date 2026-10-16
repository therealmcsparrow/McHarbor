// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"fmt"
)

// validateGuestID refuses anything that is not an xid before it is built into a
// unit name and handed to a shell.
func validateGuestID(id string) error {
	if id == "" {
		return fmt.Errorf("compute: a guest id is required")
	}
	if len(id) > 64 {
		return fmt.Errorf("compute: guest id is too long")
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return fmt.Errorf("compute: guest id contains an invalid character %q", r)
		}
	}
	return nil
}

// volumeNameFor names a guest's disk inside its own directory.
//
// `<guestId>/disk-<slot>` rather than the flat `<guestId>-<slot>`: everything
// belonging to one guest — its disks, their snapshots and their overlays —
// then sits in a directory that can be moved, copied or deleted as one thing,
// and every backing reference inside it is a sibling's name rather than an
// absolute path.
func volumeNameFor(guestID string, slot int) string {
	return fmt.Sprintf("%s/disk-%d", guestID, slot)
}

// cloneCopy is one volume a clone has to copy: the source disk and the name the
// copy takes in the same pool.
type cloneCopy struct {
	Source     VMDiskSpec
	TargetName string
}

// clonedVolume is what the pool made for one copy.
type clonedVolume struct {
	Path      string
	Format    string
	SizeBytes int64
	// Backed is true when the copy reads through the source, a qcow2 overlay on
	// a dir pool. A CoW clone on zfs or btrfs shares blocks without a backing
	// reference and reports false.
	Backed bool
}

// planVMClone decides which disks a clone copies.
//
// A CD-ROM is left out of the rewrite: an installer disc is a file both guests
// can read, not something either owns, and copying a 5 GB ISO into the clone's
// folder would be waste pretending to be isolation.
func planVMClone(source []VMDiskSpec, targetVMID string) ([]cloneCopy, error) {
	if err := validateGuestID(targetVMID); err != nil {
		return nil, err
	}
	seen := make(map[int]bool, len(source))
	var copies []cloneCopy
	for _, disk := range source {
		if seen[disk.Slot] {
			return nil, fmt.Errorf("compute: two disks claim slot %d", disk.Slot)
		}
		seen[disk.Slot] = true
		if disk.IsCDROM {
			continue
		}
		if disk.VolumePath == "" {
			return nil, fmt.Errorf("compute: the disk in slot %d has no volume to clone", disk.Slot)
		}
		copies = append(copies, cloneCopy{Source: disk, TargetName: volumeNameFor(targetVMID, disk.Slot)})
	}
	return copies, nil
}

// clonedDisks is the clone's disk list: every copied volume at its new path, and
// every disc exactly where it was.
func clonedDisks(source []VMDiskSpec, made map[int]clonedVolume) []VMDiskSpec {
	out := make([]VMDiskSpec, 0, len(source))
	for _, disk := range source {
		copied := disk
		if disk.IsCDROM {
			copied.BackingPath = ""
			out = append(out, copied)
			continue
		}
		volume, ok := made[disk.Slot]
		if !ok {
			continue
		}
		copied.VolumePath = volume.Path
		if volume.Format != "" {
			copied.Format = volume.Format
		}
		if volume.SizeBytes > 0 {
			copied.SizeBytes = volume.SizeBytes
		}
		copied.BackingPath = ""
		if volume.Backed {
			copied.BackingPath = disk.VolumePath
		}
		out = append(out, copied)
	}
	return out
}
