// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import "context"

// ReadOnlyInstaller exposes the catalog without supporting install /
// uninstall / enable / configure. Used when the dev plugin-catalog
// is mounted but the full plugin runtime (DefaultInstaller) isn't
// yet wired into the host — the admin can browse the catalog and
// see entries, but the action buttons return a clean "not enabled"
// error instead of crashing.
type ReadOnlyInstaller struct{}

// Install refuses with ErrPlatformUnsupported because no plugin runtime is wired in.
func (ReadOnlyInstaller) Install(context.Context, Source, ProgressFunc) (*Installed, error) {
	return nil, ErrPlatformUnsupported
}

// Uninstall refuses with ErrPlatformUnsupported because no plugin runtime is wired in.
func (ReadOnlyInstaller) Uninstall(context.Context, string, UninstallOptions, ProgressFunc) (*Uninstalled, error) {
	return nil, ErrPlatformUnsupported
}

// Enable refuses with ErrPlatformUnsupported because no plugin runtime is wired in.
func (ReadOnlyInstaller) Enable(context.Context, string) error { return ErrPlatformUnsupported }

// Disable refuses with ErrPlatformUnsupported because no plugin runtime is wired in.
func (ReadOnlyInstaller) Disable(context.Context, string) error {
	return ErrPlatformUnsupported
}

var _ Installer = (*ReadOnlyInstaller)(nil)
