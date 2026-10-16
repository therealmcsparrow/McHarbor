// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build !windows

package support

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// localDiskUsage returns total and free bytes of the filesystem that
// holds path, as the unprivileged caller sees them (Bavail, not
// Bfree — reserved blocks are not free space the backup can use).
func localDiskUsage(path string) (total, free int64, err error) {
	if path == "" {
		path = "."
	}
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, 0, fmt.Errorf("statfs: %w", err)
	}
	bsize := int64(st.Bsize)
	if bsize <= 0 {
		return 0, 0, fmt.Errorf("statfs: invalid block size %d", bsize)
	}
	return int64(st.Blocks) * bsize, int64(st.Bavail) * bsize, nil
}
