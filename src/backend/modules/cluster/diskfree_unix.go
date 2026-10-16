// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build !windows

package cluster

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func diskFree(path string) (int64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, fmt.Errorf("statfs: %w", err)
	}
	return int64(st.Bavail) * int64(st.Bsize), nil
}
