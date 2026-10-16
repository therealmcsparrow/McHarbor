// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package lxc

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// rootfsUsageMaxEntries bounds one walk. A tree past it reports what was
// counted so far, which undercounts rather than stalling a stats call.
const rootfsUsageMaxEntries = 2_000_000

// errWalkBudget ends a walk that ran out of entries or time.
var errWalkBudget = errors.New("lxc: rootfs usage walk budget reached")

// measureRootfs sums the blocks a rootfs occupies, as du does: allocated space
// rather than apparent size, each hard-linked inode once, and never across a
// mount point, so a volume mounted into the tree is not charged to it.
func measureRootfs(ctx context.Context, root string) (uint64, error) {
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return 0, err
	}
	rootStat, ok := rootInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, errors.New("lxc: the rootfs cannot be measured on this platform")
	}

	type inode struct{ dev, ino uint64 }
	seen := make(map[inode]struct{})
	var total uint64
	entries := 0
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // unreadable entries are skipped so a partial walk still yields a usage estimate
		}
		entries++
		if entries > rootfsUsageMaxEntries || ctx.Err() != nil {
			return errWalkBudget
		}
		info, err := entry.Info()
		if err != nil {
			return nil //nolint:nilerr // unreadable entries are skipped so a partial walk still yields a usage estimate
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return nil
		}
		if entry.IsDir() && path != root && uint64(stat.Dev) != uint64(rootStat.Dev) {
			return filepath.SkipDir
		}
		if stat.Nlink > 1 && !entry.IsDir() {
			key := inode{uint64(stat.Dev), uint64(stat.Ino)}
			if _, dup := seen[key]; dup {
				return nil
			}
			seen[key] = struct{}{}
		}
		if stat.Blocks > 0 {
			total += uint64(stat.Blocks) * 512
		}
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, errWalkBudget) {
		return total, walkErr
	}
	return total, nil
}
