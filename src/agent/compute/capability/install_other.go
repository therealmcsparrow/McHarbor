// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build !linux

package capability

import (
	"context"
	"errors"
)

// ErrNotInstallable is returned everywhere off Linux, where compute does not run at all.
var ErrNotInstallable = errors.New("this item cannot be installed by the agent on this host")

// ErrInstallBusy mirrors the Linux build so callers compile on every platform.
var ErrInstallBusy = errors.New("another package install is already running on this host")

// Install refuses off Linux: there is no compute here to install packages for.
func Install(_ context.Context, _ string) ([]string, string, error) {
	return nil, "", ErrNotInstallable
}
