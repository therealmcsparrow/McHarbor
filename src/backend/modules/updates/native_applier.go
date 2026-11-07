// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package updates

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/nativeupdate"
)

// nativeInstaller is the part of nativeupdate.Updater the automatic installer needs.
type nativeInstaller interface {
	Apply(ctx context.Context, version string) (nativeupdate.Result, error)
}

// nativeApplier passes the target AutoInstall chose on as the version to install, because
// Apply with no version reads latest, which is the prior stable on a prerelease host.
func nativeApplier(n nativeInstaller) SelfUpdateApplier {
	return func(ctx context.Context, version string) error {
		_, err := n.Apply(ctx, version)
		return err
	}
}
