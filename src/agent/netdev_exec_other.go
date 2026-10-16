// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build !linux

package main

import (
	"fmt"
	"os"
)

// isNetdevExecInvocation is always false off Linux: compute, and therefore
// macvtap, is Linux-host-only.
func isNetdevExecInvocation(_ []string) bool { return false }

// runNetdevExec exists so main() compiles everywhere. It is unreachable,
// because the invocation is never recognised off Linux.
func runNetdevExec(_ []string) {
	fmt.Fprintln(os.Stderr, "netdev-exec: guest networking requires a Linux host")
	os.Exit(2)
}
