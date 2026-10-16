// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package selfhost is the contract for running a script on the machine McHarbor
// is installed on, so a module can drive the system module's runner without importing it.
package selfhost

import (
	"context"
	"time"
)

// Result is the combined output and exit code of one script run on the
// machine McHarbor is installed on.
type Result struct {
	Output   string
	ExitCode int64
}

// Runner runs `chroot /host …` scripts on McHarbor's own machine; main wires the
// system module's Service in, which is what host updates already use.
type Runner interface {
	RunSelfHost(ctx context.Context, script string, readWrite bool, timeout time.Duration) (*Result, error)
	// SelfHostUnreachableReason returns the operator-facing sentence err carries
	// when the host could not be reached at all.
	SelfHostUnreachableReason(err error) (string, bool)
}
