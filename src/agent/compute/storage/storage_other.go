// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build !linux

package storage

// Compute is Linux-host-only, so the drivers are compiled only there. The
// non-Linux agent still links this package because the protocol types name it;
// it simply has no drivers to offer, and the capability probe already told the
// UI why.

// NewDirDriver is unavailable off Linux.
func NewDirDriver() Driver { return nil }

// NewZFSDriver is unavailable off Linux.
func NewZFSDriver() Driver { return nil }

// NewBtrfsDriver is unavailable off Linux.
func NewBtrfsDriver() Driver { return nil }

// NewLVMDriver is unavailable off Linux.
func NewLVMDriver() Driver { return nil }
