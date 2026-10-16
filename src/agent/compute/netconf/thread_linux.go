// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package netconf

import "runtime"

// Thin aliases so the netns helper reads as one idea rather than interleaving
// runtime calls with netlink ones. See inNetns for why the lock is mandatory.
func runtime_LockOSThread()   { runtime.LockOSThread() }
func runtime_UnlockOSThread() { runtime.UnlockOSThread() }
