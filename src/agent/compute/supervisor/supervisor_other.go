// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build !linux

package supervisor

import "context"

// Compute is Linux-host-only. The Windows and macOS agent builds still compile
// this package — the protocol types and the guest lifecycle code reference it
// unconditionally — but every entry point refuses.
//
// The refusal is deliberate rather than the package simply being absent: the
// capability probe reports compute as unsupported with a reason, so the UI can
// explain why a host shows no compute surfaces instead of staying silent about
// a section the operator expected to see.

// New refuses on a non-Linux host.
func New(_ context.Context) (Supervisor, error) {
	return nil, ErrUnavailable
}

// NewSystemdRun refuses on a non-Linux host.
func NewSystemdRun() (Supervisor, error) {
	return nil, ErrUnavailable
}
