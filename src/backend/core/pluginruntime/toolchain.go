// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pluginruntime

import (
	"debug/buildinfo"
	"errors"
	"fmt"
)

// ErrToolchainMismatch marks a .so built by a different Go toolchain than the host.
// plugin.Open cannot be trusted to reject one: since Go 1.27 it can throw a fatal,
// unrecoverable "no plugin module data" that takes the whole host down.
var ErrToolchainMismatch = errors.New("plugin built with a different Go toolchain")

// ErrNotGoPlugin marks a .so that carries no Go build info, which plugin.Open
// would otherwise have to discover the hard way.
var ErrNotGoPlugin = errors.New("plugin has no Go build info")

// checkToolchain refuses soPath unless it was built by the want toolchain, so one
// stale plugin fails its own load instead of crash-looping the host.
func checkToolchain(soPath, want string) error {
	info, err := buildinfo.ReadFile(soPath)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrNotGoPlugin, soPath, err)
	}
	if info.GoVersion != want {
		return fmt.Errorf("%w: %s was built with %s, host runs %s; rebuild it from the catalog",
			ErrToolchainMismatch, soPath, info.GoVersion, want)
	}
	return nil
}
