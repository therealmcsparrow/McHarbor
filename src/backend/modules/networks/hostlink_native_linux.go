// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package networks

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// newNativeHostLinker returns the executor for a McHarbor that runs on its Docker host, or nil
// when ip(8) is not installed, which the caller reports as unavailable.
func newNativeHostLinker() *nativeHostLinker {
	if _, err := exec.LookPath("ip"); err != nil {
		return nil
	}
	return &nativeHostLinker{ip: runIP, readAlias: readSysfsAlias, writeSysctl: writeProcSys}
}

// runIP executes ip with a fixed argument vector; every name in args passed validateNativeLink.
func runIP(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "ip", args...).CombinedOutput() //nolint:gosec // G204: fixed command, arguments validated by validateNativeLink
	return string(out), err
}

// readSysfsAlias reports whether the link exists and the ifalias it carries.
func readSysfsAlias(name string) (string, bool, error) {
	dir := filepath.Join("/sys/class/net", name)
	if _, err := os.Stat(dir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", false, nil
		}
		return "", false, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, "ifalias"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", true, nil
		}
		return "", true, err
	}
	return strings.TrimSpace(string(raw)), true, nil
}

// writeProcSys writes one sysctl, mapping EPERM and EACCES to the permission error the agent reports.
func writeProcSys(path, value string) error {
	if err := os.WriteFile(path, []byte(value), 0o644); err != nil { //nolint:gosec // G306: /proc/sys files are managed by the kernel
		if errors.Is(err, fs.ErrPermission) {
			return fmt.Errorf("%w: %w", errNativePermission, err)
		}
		return err
	}
	return nil
}
