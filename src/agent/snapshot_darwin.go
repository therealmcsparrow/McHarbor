// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build darwin

package main

import "context"

// macOSSnapshotUnsupported is the one explanation every operation on
// this platform returns. Docker Desktop runs containers in a LinuxKit
// VM whose filesystem the agent cannot reach from the host side, so
// there is nothing to freeze here.
const macOSSnapshotUnsupported = "macOS host snapshots are not supported: Docker Desktop runs containers in a Linux VM the agent cannot snapshot. Use a Linux host for zfs, btrfs or lvm, or a NAS snapshot plugin."

// snapshotDetect reports no usable driver, in the same shape the Linux
// and Windows agents use, so the settings UI never branches on the
// agent's platform.
func snapshotDetect(_ context.Context, _ SnapshotPayload) (*SnapshotPayload, error) {
	return &SnapshotPayload{
		Operation: "detect",
		Success:   true,
		Detected: []SnapshotDriverOut{{
			Driver:    "none",
			Available: false,
			Error:     macOSSnapshotUnsupported,
		}},
	}, nil
}

// snapshotCapability reports no snapshot-capable driver on macOS.
func snapshotCapability(_ context.Context, payload SnapshotPayload) (*SnapshotPayload, error) {
	return &SnapshotPayload{
		Driver:    payload.Driver,
		Available: false,
		Error:     macOSSnapshotUnsupported,
	}, nil
}

// snapshotCreate always fails on macOS.
func snapshotCreate(_ context.Context, payload SnapshotPayload) (*SnapshotPayload, error) {
	return &SnapshotPayload{
		Driver:  payload.Driver,
		Success: false,
		Error:   macOSSnapshotUnsupported,
	}, nil
}

// snapshotRemove always succeeds on macOS: nothing was ever created,
// so the host's deferred cleanup has nothing left to fail on.
func snapshotRemove(_ context.Context, payload SnapshotPayload) (*SnapshotPayload, error) {
	return &SnapshotPayload{Driver: payload.Driver, Success: true}, nil
}
