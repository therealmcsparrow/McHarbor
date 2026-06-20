// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build !linux

package metrics

// readHostStats returns ok=false on non-Linux platforms so the service
// reports zeros and the UI can show a "host metrics unavailable" state.
func readHostStats() (cpuPercent, memUsed float64, load1, load5, load15 float64, ok bool) {
	return 0, 0, 0, 0, 0, false
}

// readRootFSUsage is not implemented on non-Linux platforms.
func readRootFSUsage() (int64, int64, bool) {
	return 0, 0, false
}

// readUptime is not implemented on non-Linux platforms.
func readUptime() int64 {
	return 0
}
