// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build windows

package support

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// localDiskUsage returns total and free bytes of the volume that
// holds path via GetDiskFreeSpaceEx. Only reached when the backend
// runs natively on Windows during development; the shipped container
// takes the unix path.
func localDiskUsage(path string) (total, free int64, err error) {
	if path == "" {
		path = "."
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, fmt.Errorf("encoding path: %w", err)
	}
	var freeAvail, totalBytes, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &freeAvail, &totalBytes, &totalFree); err != nil {
		return 0, 0, fmt.Errorf("GetDiskFreeSpaceEx: %w", err)
	}
	return int64(totalBytes), int64(freeAvail), nil
}
