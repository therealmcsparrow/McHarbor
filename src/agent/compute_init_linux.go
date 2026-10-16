// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package main

import (
	"os"

	"github.com/therealmcsparrow/mcharbor-agent/compute/lxc"
)

// The container-init re-exec branch, kept behind a build tag so the Windows and
// macOS agent builds carry none of libcontainer, none of its CGO requirement,
// and none of this code path.
//
// See lxc.RunInit for why main() must call this before anything else.

// isContainerInitInvocation reports whether this process was re-executed by
// libcontainer as a container's init helper rather than started as the agent.
func isContainerInitInvocation(args []string) bool {
	return lxc.IsInitInvocation(args)
}

// runContainerInit hands control to libcontainer. It never returns.
func runContainerInit() {
	lxc.RunInit()
	// Belt and braces: RunInit exits on every path, and continuing into agent
	// startup from inside a half-built container would be far worse than
	// stopping here.
	os.Exit(1)
}
