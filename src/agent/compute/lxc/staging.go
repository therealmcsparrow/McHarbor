// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lxc

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// DefaultStageBase is the directory an unprivileged container's rootfs is
// bind-mounted under before runc sees it. It is under /run so a reboot, which
// also ends every container, clears whatever a killed agent left there.
const DefaultStageBase = "/run/mcharbor-agent"

// stageLayout is where one container's rootfs is staged.
type stageLayout struct {
	// Owned are the directories McHarbor creates and chmods to stageDirMode,
	// outermost first. Nothing above the first one is ever modified.
	Owned []string
	// Dir is the per-container directory, the last entry of Owned.
	Dir string
	// Target is the bind mount point handed to runc as the container's rootfs.
	Target string
}

// stageLayoutFor builds the staging paths for ctID under base, refusing an id
// that is not a single path element so a release can never rmdir outside base.
func stageLayoutFor(base, ctID string) (stageLayout, error) {
	if ctID == "" || ctID == "." || ctID == ".." || strings.ContainsAny(ctID, `/\`) {
		return stageLayout{}, fmt.Errorf("lxc: %q cannot name a staging directory", ctID)
	}
	root := filepath.Join(base, "ct")
	dir := filepath.Join(root, ctID)
	return stageLayout{
		Owned:  []string{base, root, dir},
		Dir:    dir,
		Target: filepath.Join(dir, "rootfs"),
	}, nil
}

// needsStaging reports whether a container's rootfs has to be staged. Only a
// user-namespaced init resolves the rootfs path as an unprivileged host uid; a
// privileged one is host root and can traverse the pool's 0700 directories.
func needsStaging(spec Spec) bool {
	return !spec.Privileged
}

// untraversableAncestor returns the first directory above base that others
// cannot search, or "" when the whole chain is searchable. Those directories
// are not McHarbor's to chmod, so the start is refused with the path named.
func untraversableAncestor(base string, modeOf func(string) (fs.FileMode, error)) (string, error) {
	for dir := filepath.Dir(base); ; dir = filepath.Dir(dir) {
		mode, err := modeOf(dir)
		if err != nil {
			return "", fmt.Errorf("lxc: inspect %s: %w", dir, err)
		}
		if mode.Perm()&0o001 == 0 {
			return dir, nil
		}
		if filepath.Dir(dir) == dir {
			return "", nil
		}
	}
}

// keepStageOnSweep decides whether a startup sweep leaves a staged rootfs
// mounted. A live container's staging path is runc's working directory for
// every exec into it, so anything that may still be running keeps its mount.
func keepStageOnSweep(p presence, state string) bool {
	if p != presenceLoaded {
		return false
	}
	return state != StateStopped
}
