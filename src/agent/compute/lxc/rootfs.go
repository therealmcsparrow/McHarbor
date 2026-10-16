// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lxc

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

// ErrUnsafeArchive means a rootfs archive or template tried to write outside
// the tree it was being laid down in, or asked for something that would.
var ErrUnsafeArchive = errors.New("lxc: refusing an unsafe rootfs archive")

// ErrRootfsGuard means a rootfs removal named a path that is not safely inside
// its pool, so nothing was removed.
var ErrRootfsGuard = errors.New("lxc: refusing to remove this rootfs")

// defaultIDCount is the id range assumed for a template whose own range the
// agent cannot see, which is the size every McHarbor allocation uses.
const defaultIDCount = 65536

// IDShift maps the ids a rootfs holds to the host ids that must own them, so
// an unprivileged container's root owns its own files. The zero value is the
// identity mapping a privileged container uses.
type IDShift struct {
	UIDStart int64
	UIDCount int64
	GIDStart int64
	GIDCount int64
}

// ShiftFor returns the ownership shift a container's rootfs needs; a privileged
// container has no user namespace, so its files keep the ids they were built with.
func ShiftFor(spec Spec) IDShift {
	if spec.Privileged {
		return IDShift{}
	}
	return IDShift{
		UIDStart: spec.UIDMapStart, UIDCount: spec.UIDMapCount,
		GIDStart: spec.GIDMapStart, GIDCount: spec.GIDMapCount,
	}
}

// UID maps a container uid to its host owner, refusing one the container's
// range cannot represent rather than handing the file to an unrelated host user.
func (s IDShift) UID(id int64) (int, error) {
	return shiftID("uid", id, s.UIDStart, s.UIDCount)
}

// GID is UID for group ids.
func (s IDShift) GID(id int64) (int, error) {
	return shiftID("gid", id, s.GIDStart, s.GIDCount)
}

func shiftID(kind string, id, start, count int64) (int, error) {
	if id < 0 {
		return 0, fmt.Errorf("%w: negative %s %d", ErrUnsafeArchive, kind, id)
	}
	if count == 0 {
		return int(id), nil
	}
	if id >= count {
		return 0, fmt.Errorf("%w: %s %d is outside the container's %d-id range", ErrUnsafeArchive, kind, id, count)
	}
	return int(start + id), nil
}

// EntryPath validates one archive member name and returns it cleaned and
// relative, with "" meaning the root itself. Absolute names and any ".."
// component are refused outright instead of being cleaned into something
// harmless-looking, because an archive carrying one was built to escape.
func EntryPath(name string) (string, error) {
	if name == "" || strings.ContainsRune(name, 0) {
		return "", fmt.Errorf("%w: empty or malformed member name", ErrUnsafeArchive)
	}
	if strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("%w: absolute member %q", ErrUnsafeArchive, name)
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return "", fmt.Errorf("%w: member %q climbs out of the rootfs", ErrUnsafeArchive, name)
		}
	}
	clean := path.Clean(name)
	if clean == "." {
		return "", nil
	}
	return clean, nil
}

// SymlinkEscapes reports whether a relative symlink target climbs above the
// rootfs. An absolute target is not an escape: inside the container it resolves
// against the container's own root, which is how every distribution ships them.
func SymlinkEscapes(rel, target string) bool {
	if target == "" {
		return true
	}
	if path.IsAbs(target) {
		return false
	}
	resolved := path.Clean(path.Join(path.Dir(rel), target))
	return resolved == ".." || strings.HasPrefix(resolved, "../")
}

// CheckRootfsRemovable refuses a removal that is not strictly inside an
// absolute pool target, so neither "/" nor the pool root itself can ever be
// handed to a recursive delete.
func CheckRootfsRemovable(rootfs, poolTarget string) error {
	if strings.TrimSpace(rootfs) == "" || strings.TrimSpace(poolTarget) == "" {
		return fmt.Errorf("%w: the request named no rootfs path or no pool target", ErrRootfsGuard)
	}
	if !path.IsAbs(rootfs) || !path.IsAbs(poolTarget) {
		return fmt.Errorf("%w: %q in pool %q is not an absolute path", ErrRootfsGuard, rootfs, poolTarget)
	}
	root := path.Clean(poolTarget)
	target := path.Clean(rootfs)
	if root == "/" {
		return fmt.Errorf("%w: a pool rooted at / cannot hold a removable rootfs", ErrRootfsGuard)
	}
	if target == "/" || target == root {
		return fmt.Errorf("%w: %q is the pool root", ErrRootfsGuard, rootfs)
	}
	if !strings.HasPrefix(target, root+"/") {
		return fmt.Errorf("%w: %q is outside pool %q", ErrRootfsGuard, rootfs, poolTarget)
	}
	return nil
}

// CheckTemplatePath accepts only another container's rootfs as a template
// source, so a create can never copy an arbitrary host directory into a guest.
func CheckTemplatePath(template string) error {
	if !path.IsAbs(template) {
		return fmt.Errorf("%w: template %q is not an absolute path", ErrUnsafeArchive, template)
	}
	clean := path.Clean(template)
	if clean != template && clean+"/" != template {
		return fmt.Errorf("%w: template %q is not a clean path", ErrUnsafeArchive, template)
	}
	if path.Base(clean) != "rootfs" || path.Base(path.Dir(path.Dir(clean))) != "containers" {
		return fmt.Errorf("%w: template %q is not a container rootfs", ErrUnsafeArchive, template)
	}
	return nil
}

// EmptyDirsAfterRootfsRemoval lists, innermost first, the directories a removed
// rootfs leaves for rmdir: <pool>/containers/<id>, then <pool>/containers. Any
// other shape lists nothing, and the pool root itself is never listed.
func EmptyDirsAfterRootfsRemoval(ctID, rootfs, poolTarget string) []string {
	if ctID == "" || !path.IsAbs(rootfs) || !path.IsAbs(poolTarget) {
		return nil
	}
	root := path.Clean(poolTarget)
	if root == "/" {
		return nil
	}
	containerDir := path.Dir(path.Clean(rootfs))
	containersDir := path.Dir(containerDir)
	if path.Base(containerDir) != ctID || path.Base(containersDir) != "containers" ||
		path.Dir(containersDir) != root {
		return nil
	}
	return []string{containerDir, containersDir}
}
