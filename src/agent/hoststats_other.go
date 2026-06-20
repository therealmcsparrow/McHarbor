// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build !linux

package main

// readHostStats is not implemented on non-Linux platforms. Returns ok=false
// so the server reports zeros and the UI shows a "host metrics unavailable"
// state.
func readHostStats() (HostStatsPayload, bool) {
	return HostStatsPayload{}, false
}
