// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lxc

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"syscall"

	"github.com/opencontainers/runc/libcontainer/configs"
)

// RootfsOwnerFor reads a rootfs's owner off its root directory, which is the
// container's root in every McHarbor layout; configJSON only sizes the range.
//
// The agent keeps no container table, and the rootfs root is also what the
// restore guard infers a legacy backup's range from, so both read one source.
func RootfsOwnerFor(rootfs, configJSON string) (RootfsOwner, error) {
	info, err := os.Lstat(rootfs)
	if err != nil {
		return RootfsOwner{}, fmt.Errorf("lxc: read the rootfs owner: %w", err)
	}
	if !info.IsDir() {
		return RootfsOwner{}, fmt.Errorf("lxc: rootfs %s is not a directory", rootfs)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return RootfsOwner{}, errors.New("lxc: the rootfs owner is not readable on this platform")
	}
	uid, gid := int64(stat.Uid), int64(stat.Gid)
	if uid == 0 && gid == 0 {
		return RootfsOwner{}, nil
	}
	uidCount, gidCount := rangeSizes(configJSON, uid, gid)
	return RootfsOwner{
		UID: int(uid),
		GID: int(gid),
		Shift: IDShift{
			UIDStart: uid, UIDCount: uidCount,
			GIDStart: gid, GIDCount: gidCount,
		},
	}, nil
}

// rangeSizes takes the map sizes from the container's config.json when its
// maps start where the rootfs root says; a missing or disagreeing file falls
// back to the size every McHarbor allocation uses.
func rangeSizes(configJSON string, uid, gid int64) (int64, int64) {
	uidCount, gidCount := int64(defaultIDCount), int64(defaultIDCount)
	raw, err := os.ReadFile(configJSON)
	if err != nil {
		return uidCount, gidCount
	}
	var config struct {
		UIDMappings []configs.IDMap `json:"uid_mappings"`
		GIDMappings []configs.IDMap `json:"gid_mappings"`
	}
	if json.Unmarshal(raw, &config) != nil {
		return uidCount, gidCount
	}
	if size := rootMapSize(config.UIDMappings, uid); size > 0 {
		uidCount = size
	}
	if size := rootMapSize(config.GIDMappings, gid); size > 0 {
		gidCount = size
	}
	return uidCount, gidCount
}

func rootMapSize(maps []configs.IDMap, host int64) int64 {
	for _, m := range maps {
		if m.ContainerID == 0 && m.HostID == host && m.Size > 0 {
			return m.Size
		}
	}
	return 0
}
