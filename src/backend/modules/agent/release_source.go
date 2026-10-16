// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"context"
	"sync"
)

// ReleaseTarget is the slice of the agent release store the binary and image
// routes serve from; main wires the agentreleases store in through
// SetReleaseSource so this module never imports it.
type ReleaseTarget interface {
	TargetBinaryPath(ctx context.Context, osName, arch string) (path, version, build string)
	TargetImage(ctx context.Context, osName, arch string) (path, sha, ref, version string)
	TargetImageRef(ctx context.Context) string
}

var (
	releaseSourceMu sync.RWMutex
	releaseSource   func() ReleaseTarget
)

// SetReleaseSource installs fn as the lookup for the operator-selected release
// store; fn is called on every request because the store can be swapped at
// runtime, and returns nil when there is none.
func SetReleaseSource(fn func() ReleaseTarget) {
	releaseSourceMu.Lock()
	defer releaseSourceMu.Unlock()
	releaseSource = fn
}

// releaseTarget returns the current release store, or nil when none is wired
// or selected, in which case only built-in binaries are served.
func releaseTarget() ReleaseTarget {
	releaseSourceMu.RLock()
	fn := releaseSource
	releaseSourceMu.RUnlock()
	if fn == nil {
		return nil
	}
	return fn()
}
