// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lxc

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Init kinds as read from a rootfs rather than as declared. The declared init
// system is a catalog or operator statement; an alpine minirootfs is declared
// openrc and ships no openrc, so only the tree says what PID 1 will be.
const (
	InitKindSystemd  = "systemd"
	InitKindOpenRC   = "openrc"
	InitKindBusybox  = "busybox"
	InitKindSysVInit = "sysvinit"
	InitKindNone     = ""
)

// InitInfo is what DetectInit found. BusyboxInit is separate from Kind because
// an alpine with openrc still runs busybox init as PID 1 and reads a busybox
// inittab; openrc is what that inittab starts, not what PID 1 is.
type InitInfo struct {
	Kind        string
	BusyboxInit bool
}

// rootfsProbe reads a rootfs without following a link out of it; paths are
// relative to the rootfs root.
type rootfsProbe interface {
	Lstat(rel string) (fs.FileInfo, error)
	Readlink(rel string) (string, error)
}

// dirProbe is a rootfsProbe over a directory on the host.
type dirProbe string

// Lstat stats rel inside the rootfs without following a final link.
func (d dirProbe) Lstat(rel string) (fs.FileInfo, error) {
	return os.Lstat(filepath.Join(string(d), filepath.FromSlash(rel)))
}

// Readlink reads the link at rel inside the rootfs.
func (d dirProbe) Readlink(rel string) (string, error) {
	return os.Readlink(filepath.Join(string(d), filepath.FromSlash(rel)))
}

// maxInitLinks bounds how many links resolving /sbin/init may follow, so a
// link cycle in an image cannot spin the agent.
const maxInitLinks = 8

// resolveInRootfs follows rel's links component by component, the way the
// container sees them: an absolute target restarts at the rootfs root and ".."
// stops there, as it does in a chroot. Resolving the whole path in one host
// call instead would follow an image's absolute /sbin -> /usr/sbin link into the
// host's own /usr. It returns the final relative path and whether it exists.
func resolveInRootfs(probe rootfsProbe, rel string) (string, bool) {
	pending := strings.Split(strings.TrimPrefix(rel, "/"), "/")
	resolved := ""
	hops := 0
	for len(pending) > 0 {
		part := pending[0]
		pending = pending[1:]
		switch part {
		case "", ".":
			continue
		case "..":
			if resolved != "" {
				resolved = path.Dir(resolved)
				if resolved == "." {
					resolved = ""
				}
			}
			continue
		}
		next := path.Join(resolved, part)
		info, err := probe.Lstat(next)
		if err != nil {
			return next, false
		}
		if info.Mode()&fs.ModeSymlink == 0 {
			resolved = next
			continue
		}
		hops++
		if hops > maxInitLinks {
			return next, false
		}
		target, err := probe.Readlink(next)
		if err != nil {
			return next, false
		}
		if path.IsAbs(target) {
			resolved = ""
		}
		pending = append(strings.Split(target, "/"), pending...)
	}
	return resolved, resolved != ""
}

// existsInRootfs reports whether any of rels exists, following links.
func existsInRootfs(probe rootfsProbe, rels ...string) bool {
	for _, rel := range rels {
		if _, ok := resolveInRootfs(probe, rel); ok {
			return true
		}
	}
	return false
}

// DetectInit reads what /sbin/init is in a rootfs. The order matters: systemd
// is recognised by where /sbin/init leads, openrc only by its binary being
// present, and busybox by /sbin/init being the busybox multi-call binary.
func DetectInit(probe rootfsProbe) InitInfo {
	target, ok := resolveInRootfs(probe, "sbin/init")
	if !ok {
		return InitInfo{Kind: InitKindNone}
	}
	base := path.Base(target)
	switch {
	case strings.Contains(target, "systemd"):
		return InitInfo{Kind: InitKindSystemd}
	case base == "busybox":
		if existsInRootfs(probe, "sbin/openrc", "usr/sbin/openrc") {
			return InitInfo{Kind: InitKindOpenRC, BusyboxInit: true}
		}
		return InitInfo{Kind: InitKindBusybox, BusyboxInit: true}
	case base == "openrc-init":
		return InitInfo{Kind: InitKindOpenRC}
	default:
		return InitInfo{Kind: InitKindSysVInit}
	}
}

// DetectInitAt is DetectInit over a rootfs directory on the host.
func DetectInitAt(rootfs string) InitInfo {
	return DetectInit(dirProbe(rootfs))
}
