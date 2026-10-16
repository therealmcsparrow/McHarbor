// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build !windows

package main

import (
	"os"
	"syscall"
)

// replaceExecutable atomically moves the downloaded binary over the running
// one. On Unix the running process keeps its old inode, so the rename is
// safe while the agent is still executing.
func replaceExecutable(newPath, exePath string) error {
	return os.Rename(newPath, exePath)
}

// reexecSelf replaces the current process image with the new binary,
// keeping the PID, arguments and environment. Works under systemd as well
// as a plain nohup start because no supervisor round-trip is needed.
func reexecSelf(exePath string) error {
	return syscall.Exec(exePath, os.Args, os.Environ())
}
