// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package lxc

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// Extended attributes that carry an id and so move with the owners.
const (
	xattrACLAccess  = "system.posix_acl_access"
	xattrACLDefault = "system.posix_acl_default"
	xattrFileCaps   = "security.capability"
)

// inodeKey identifies an inode, so a hard-linked file is re-owned once: its
// second name already carries the target's owner, which the source range
// does not contain.
type inodeKey struct {
	dev uint64
	ino uint64
}

// SummarizeOwners reads the inode owners of a tree without following any
// symlink, for a legacy backup whose source range has to be inferred.
func SummarizeOwners(ctx context.Context, root string) (OwnerSummary, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return OwnerSummary{}, fmt.Errorf("lxc: reading the restored rootfs: %w", err)
	}
	rootStat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return OwnerSummary{}, fmt.Errorf("lxc: the restored rootfs carries no ownership")
	}
	sum := OwnerSummary{
		RootUID: int64(rootStat.Uid), RootGID: int64(rootStat.Gid),
		MinUID: int64(rootStat.Uid), MaxUID: int64(rootStat.Uid),
		MinGID: int64(rootStat.Gid), MaxGID: int64(rootStat.Gid),
	}
	err = filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		entryInfo, err := entry.Info()
		if err != nil {
			return err
		}
		stat, ok := entryInfo.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("%s carries no ownership", current)
		}
		uid, gid := int64(stat.Uid), int64(stat.Gid)
		sum.MinUID, sum.MaxUID = min(sum.MinUID, uid), max(sum.MaxUID, uid)
		sum.MinGID, sum.MaxGID = min(sum.MinGID, gid), max(sum.MaxGID, gid)
		return nil
	})
	if err != nil {
		return OwnerSummary{}, fmt.Errorf("lxc: reading the owners of the restored rootfs: %w", err)
	}
	return sum, nil
}

// ReownTree moves every entry under root from one container's id range into
// another's.
//
// lchown never follows a symlink. chown clears setuid, setgid and file
// capabilities, so the mode is re-applied and the capability re-written after
// it; POSIX ACL entries and a v3 capability's root id carry ids of their own
// and are remapped too. The whole tree is checked before the first change, so
// an entry outside the source range refuses the call with nothing moved.
func ReownTree(ctx context.Context, root string, remap IDRemap) (int, error) {
	if remap.Identity() {
		return 0, nil
	}
	var problems idProblems
	err := walkInodes(ctx, root, func(current string, info fs.FileInfo, stat *syscall.Stat_t) error {
		if _, err := planEntry(current, info, stat, remap); err != nil {
			problems.add(relName(root, current), err)
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("lxc: checking the restored rootfs's owners: %w", err)
	}
	if err := problems.err(remap); err != nil {
		return 0, err
	}

	moved := 0
	err = walkInodes(ctx, root, func(current string, info fs.FileInfo, stat *syscall.Stat_t) error {
		plan, err := planEntry(current, info, stat, remap)
		if err != nil {
			return fmt.Errorf("%s changed while it was being re-owned: %w", relName(root, current), err)
		}
		if err := plan.apply(current); err != nil {
			return fmt.Errorf("re-owning %s: %w", relName(root, current), err)
		}
		moved++
		return nil
	})
	if err != nil {
		return moved, fmt.Errorf("lxc: moving the restored rootfs into the container's id range: %w", err)
	}
	return moved, nil
}

// walkInodes visits every entry under root once per inode, never following a
// symlink, so a hard-linked file is seen by its first name only.
func walkInodes(ctx context.Context, root string, visit func(string, fs.FileInfo, *syscall.Stat_t) error) error {
	seen := map[inodeKey]bool{}
	return filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("%s carries no ownership", current)
		}
		if stat.Nlink > 1 && !info.IsDir() {
			key := inodeKey{dev: uint64(stat.Dev), ino: stat.Ino}
			if seen[key] {
				return nil
			}
			seen[key] = true
		}
		return visit(current, info, stat)
	})
}

// entryPlan is everything one entry needs written, computed before anything is
// changed so a refusal is found before the first chown.
type entryPlan struct {
	uid, gid int
	mode     uint32
	symlink  bool
	xattrs   map[string][]byte
}

func planEntry(current string, info fs.FileInfo, stat *syscall.Stat_t, remap IDRemap) (entryPlan, error) {
	uid, err := remap.UID(int64(stat.Uid))
	if err != nil {
		return entryPlan{}, err
	}
	gid, err := remap.GID(int64(stat.Gid))
	if err != nil {
		return entryPlan{}, err
	}
	plan := entryPlan{
		uid: int(uid), gid: int(gid),
		mode:    stat.Mode & 0o7777,
		symlink: info.Mode()&fs.ModeSymlink != 0,
	}
	if plan.symlink {
		return plan, nil
	}
	for _, name := range []string{xattrACLAccess, xattrACLDefault, xattrFileCaps} {
		value, present, err := lgetxattr(current, name)
		if err != nil {
			return entryPlan{}, err
		}
		if !present {
			continue
		}
		var mapped []byte
		if name == xattrFileCaps {
			mapped, err = remapFileCaps(value, remap)
		} else {
			mapped, err = remapACL(value, remap)
		}
		if err != nil {
			return entryPlan{}, err
		}
		if plan.xattrs == nil {
			plan.xattrs = map[string][]byte{}
		}
		plan.xattrs[name] = mapped
	}
	return plan, nil
}

// apply writes one plan in the only order that keeps everything: the owner,
// then the ACLs, then the mode (chown dropped setuid/setgid), then the file
// capability last because chown removed it.
func (p entryPlan) apply(current string) error {
	if err := os.Lchown(current, p.uid, p.gid); err != nil {
		return err
	}
	if p.symlink {
		return nil
	}
	for _, name := range []string{xattrACLAccess, xattrACLDefault} {
		if value, ok := p.xattrs[name]; ok {
			if err := unix.Lsetxattr(current, name, value, 0); err != nil {
				return fmt.Errorf("writing %s: %w", name, err)
			}
		}
	}
	// Not a symlink, so chmod cannot be redirected; the tree is private staging
	// that nothing else writes to while this runs.
	if err := unix.Fchmodat(unix.AT_FDCWD, current, p.mode, 0); err != nil {
		return fmt.Errorf("restoring the mode: %w", err)
	}
	if value, ok := p.xattrs[xattrFileCaps]; ok {
		if err := unix.Lsetxattr(current, xattrFileCaps, value, 0); err != nil {
			return fmt.Errorf("writing %s: %w", xattrFileCaps, err)
		}
	}
	return nil
}

// lgetxattr reads one attribute of the entry itself, reporting absence
// (including a filesystem that holds no xattrs) as not present.
func lgetxattr(current, name string) ([]byte, bool, error) {
	size, err := unix.Lgetxattr(current, name, nil)
	if errors.Is(err, unix.ENODATA) || errors.Is(err, unix.ENOTSUP) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("reading %s: %w", name, err)
	}
	value := make([]byte, size)
	if size > 0 {
		if size, err = unix.Lgetxattr(current, name, value); err != nil {
			return nil, false, fmt.Errorf("reading %s: %w", name, err)
		}
	}
	return value[:size], true, nil
}

// relName is the path a message names, relative to the rootfs root.
func relName(root, current string) string {
	rel, err := filepath.Rel(root, current)
	if err != nil || rel == "." {
		return "/"
	}
	return "/" + strings.TrimPrefix(filepath.ToSlash(rel), "./")
}

// ResolveRestoreRemap settles the shift a restore needs: the recorded source
// range when McHarbor has one, else the range the tree itself shows, refusing
// when the tree does not show one unambiguously.
func ResolveRestoreRemap(ctx context.Context, staging string, source *IDMap, target IDMap) (IDRemap, error) {
	var from IDMap
	if source != nil {
		from = *source
	} else {
		sum, err := SummarizeOwners(ctx, staging)
		if err != nil {
			return IDRemap{}, err
		}
		inferred, err := InferIDMap(sum)
		if err != nil {
			return IDRemap{}, fmt.Errorf("%w; McHarbor recorded no source range for this backup, so it cannot "+
				"be restored into a container with a different id range", err)
		}
		from = inferred
	}
	return NewIDRemap(from, target)
}
