// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build !linux

package hardware

import "context"

// readInventory reports nothing on platforms without sysfs.
//
// Mirrors `metrics/host_stats_other.go`: the backend builds and runs on
// macOS and Windows for development, and the honest answer there is "this
// platform exposes no inventory", which the UI states plainly. It is not
// an error — the shipped image is Linux, and a developer on another OS
// should not see a red panel because of it.
func readInventory(_ context.Context) Inventory {
	return emptyInventory()
}
