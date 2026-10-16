// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build !linux

package main

import (
	"context"
	"errors"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// Compute is Linux-host-only. The dispatch cases in ReadLoop are unconditional
// by design — nobody should be able to drop them from the Linux path by editing
// a build-tagged file — so these stubs answer on the platforms that cannot host
// guests.
//
// The answer is a populated refusal rather than silence: a server waiting on a
// reply it never receives hangs until its own timeout, and the operator learns
// nothing. The capability probe has already told the UI why compute is
// unavailable here.

func (a *Agent) handleComputeMessage(_ context.Context, msg WSMessage) {
	// A populated refusal, shaped to the family the request belongs to, so the
	// server's waiter finds a payload where it expects one. Silence would hang
	// the caller until its own timeout and teach the operator nothing.
	a.replyComputeError(msg, errComputeNotLinux)
}

// errComputeNotLinux is the reason every compute call fails on this platform.
var errComputeNotLinux = errors.New(
	"compute requires a Linux host: KVM is a Linux kernel module and system " +
		"containers are Linux namespaces and cgroups, so this agent's host cannot " +
		"run them")

func (a *Agent) handleConsoleStart(_ context.Context, id string, payload ConsolePayload) {
	errlog.Log(a.sendCompute(WSMessage{
		Type: MsgConsoleEnd,
		ID:   id,
		Console: &ConsolePayload{
			SessionID: payload.SessionID,
			Error: "Guest consoles require a Linux host. This agent runs on a platform " +
				"that cannot host virtual machines or system containers.",
		},
	}))
}

func (a *Agent) handleConsoleFrame(_ context.Context, _ string, _ string, _ ConsolePayload) {}
