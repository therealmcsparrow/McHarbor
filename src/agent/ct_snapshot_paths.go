// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"fmt"
	"path"

	"github.com/therealmcsparrow/mcharbor-agent/compute/lxc"
)

// Container snapshots are copies of the rootfs tree, one per snapshot, kept in
// the pool beside the container's own directory:
//
//	<pool>/containers/<ctID>/rootfs                    the container
//	<pool>/containers/<ctID>.snap-<snapshotID>/rootfs  one snapshot
//
// A copy rather than a pool snapshot because a container's rootfs is a plain
// directory inside the pool, not a dataset or subvolume of its own: a zfs or
// btrfs snapshot would capture every container on the pool, and reverting it
// would roll all of them back. The copy goes through the same id-preserving
// tree copy a template clone uses, so ownership and special files survive.
//
// The layout is shaped like a container rootfs on purpose: the copy routine
// accepts only a path of that shape as its source, which is the guard that
// stops a revert from copying an arbitrary host directory into a guest.

// ctSnapshotMarker separates a container id from a snapshot id in the
// snapshot's directory name.
const ctSnapshotMarker = ".snap-"

// ctSnapshotRootfs is where one snapshot of a container lives, checked to sit
// strictly inside the container's pool.
func ctSnapshotRootfs(rootfs, poolTarget, ctID, snapshotID string) (string, error) {
	if err := validateGuestID(ctID); err != nil {
		return "", err
	}
	if err := validateGuestID(snapshotID); err != nil {
		return "", fmt.Errorf("compute: invalid snapshot id: %w", err)
	}
	if err := lxc.CheckRootfsRemovable(rootfs, poolTarget); err != nil {
		return "", err
	}
	clean := path.Clean(rootfs)
	containerDir := path.Dir(clean)
	containers := path.Dir(containerDir)
	if path.Base(clean) != "rootfs" || path.Base(containerDir) != ctID || path.Base(containers) != "containers" {
		return "", fmt.Errorf("compute: %s is not container %s's rootfs", rootfs, ctID)
	}
	snapshot := path.Join(containers, ctID+ctSnapshotMarker+snapshotID, "rootfs")
	if err := lxc.CheckRootfsRemovable(snapshot, poolTarget); err != nil {
		return "", err
	}
	return snapshot, nil
}

// ctSnapshotGlob matches every snapshot directory of one container, for the
// cleanup a container delete owes its snapshots.
func ctSnapshotGlob(rootfs, ctID string) string {
	return path.Join(path.Dir(path.Dir(path.Clean(rootfs))), ctID+ctSnapshotMarker+"*")
}
