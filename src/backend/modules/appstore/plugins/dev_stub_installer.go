// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"context"
	"errors"
)

// ErrDevInstallerStub is returned by the dev installer when an operator
// hits Install / Uninstall / Configure against a host that hasn't
// loaded the real plugin runtime. The catalog + installed-listing GETs
// still work — only the mutating endpoints are stubbed.
var ErrDevInstallerStub = errors.New("plugins: dev installer stub — runtime not wired in this build")

// devInstaller is the minimal Installer used when main.go is built
// without the runtime's contrib hooks (read-only dev use case). The
// catalog can still be browsed and the installed-list row queries still
// return data, but Install/Uninstall/Configure return
// ErrDevInstallerStub instead of crashing the request. The production
// build uses DefaultInstaller wired in main.go.
type devInstaller struct{}

// Install satisfies the Installer interface — returns the dev-stub error.
func (devInstaller) Install(ctx context.Context, src Source, progress ProgressFunc) (*Installed, error) {
	return nil, ErrDevInstallerStub
}

// Uninstall satisfies the Installer interface.
func (devInstaller) Uninstall(ctx context.Context, slug string, _ UninstallOptions, progress ProgressFunc) (*Uninstalled, error) {
	return nil, ErrDevInstallerStub
}

// NewDevInstaller returns the no-runtime read-only installer used in
// the dev-only /mnt/plugins flow. main.go conditionally swaps in the
// real DefaultInstaller if and only if a runtime is loaded.
func NewDevInstaller() Installer { return devInstaller{} }
