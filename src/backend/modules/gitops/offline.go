// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package gitops

import (
	"errors"

	coredemo "github.com/therealmcsparrow/mcharbor/core/demo"
)

// ErrForgeOffline is returned instead of any clone, ref listing or forge API call
// while the instance runs in demo mode, where every repository names a forge that
// does not exist and reaching for it would leak traffic to a real host.
var ErrForgeOffline = errors.New("gitops: demo mode simulates every Git forge, so nothing was fetched from or sent to it")

// forgeReachable is the single gate every outbound GitOps path passes: the ref
// listing and the compose fetch through remoteFor, the probe and the pull-request
// comment through forgeCredential.
func forgeReachable() error {
	if coredemo.Active() {
		return ErrForgeOffline
	}
	return nil
}
