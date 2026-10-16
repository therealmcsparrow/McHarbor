// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build windows

package main

import (
	"fmt"
	"os"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// replaceExecutable swaps the binary on Windows, where a running
// executable cannot be overwritten but can be renamed. The old file is
// parked as `<exe>.old` and removed on the next successful update.
func replaceExecutable(newPath, exePath string) error {
	old := exePath + ".old"
	errlog.Log(os.Remove(old))
	if err := os.Rename(exePath, old); err != nil {
		return fmt.Errorf("parking current executable: %w", err)
	}
	if err := os.Rename(newPath, exePath); err != nil {
		// Put the original back so the agent keeps working.
		errlog.Log(os.Rename(old, exePath))
		return err
	}
	return nil
}

// reexecSelf has no exec(2) equivalent on Windows; exiting lets the
// service manager restart the agent from the replaced binary.
func reexecSelf(_ string) error {
	os.Exit(0)
	return nil
}
