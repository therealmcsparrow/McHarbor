// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lxc

import (
	"path"
	"strings"
)

// RootfsOwner is who a new entry in one container's rootfs must belong to: the
// container's root, and the range an existing owner has to fall in to be kept.
// The zero value is a privileged container, whose host ids are its own.
type RootfsOwner struct {
	UID   int
	GID   int
	Shift IDShift
}

// Owns reports whether a host owner is one the container can see as a real user;
// anything else shows as the overflow id inside it and fails the restore guard.
func (o RootfsOwner) Owns(uid, gid int64) bool {
	return inRange(uid, o.Shift.UIDStart, o.Shift.UIDCount) && inRange(gid, o.Shift.GIDStart, o.Shift.GIDCount)
}

func inRange(id, start, count int64) bool {
	if count == 0 {
		return id >= 0
	}
	return id >= start && id < start+count
}

// RootfsOf splits a pool-relative slash path that lies strictly inside a
// container rootfs (containers/<id>/rootfs/...) into the container id and that
// rootfs's pool-relative directory. The rootfs directory itself is not inside.
func RootfsOf(rel string) (ctID, rootfsRel string, ok bool) {
	if rel == "" || strings.HasPrefix(rel, "/") {
		return "", "", false
	}
	parts := strings.Split(path.Clean(rel), "/")
	if len(parts) < 4 || parts[0] != "containers" || parts[2] != "rootfs" {
		return "", "", false
	}
	id := parts[1]
	if id == "" || id == "." || id == ".." {
		return "", "", false
	}
	return id, path.Join(parts[0], id, parts[2]), true
}
