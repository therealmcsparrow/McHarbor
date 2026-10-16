// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build windows

package cluster

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func diskFree(path string) (int64, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, fmt.Errorf("encoding path: %w", err)
	}
	var freeAvail, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &freeAvail, &total, &totalFree); err != nil {
		return 0, fmt.Errorf("GetDiskFreeSpaceEx: %w", err)
	}
	return int64(freeAvail), nil
}
