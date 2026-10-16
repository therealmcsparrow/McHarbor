// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build !linux

package capability

import (
	"context"
	"runtime"
)

// Probe answers that Compute is Linux-host-only: KVM is a Linux kernel module and LXC is Linux
// namespaces and cgroups. Neither has a Windows or macOS equivalent McHarbor
// could drive.
//
// The probe still runs and still answers. Returning a populated report that
// says "unsupported, and here is why" is what lets the UI explain the absence,
// which was the roadmap's own recommendation over letting the capability simply
// not appear.
func Probe(_ context.Context, _ bool, _ string) Report {
	return unsupportedReport(
		"Compute requires a Linux host. KVM is a Linux kernel module and system " +
			"containers are Linux namespaces and cgroups, so this agent's host cannot " +
			"run virtual machines or system containers. Install the agent on a Linux " +
			"host to enable compute there.")
}

// unsupportedReport is what a non-Linux host answers.
//
// It is deliberately a populated report saying "no, and here is why" rather
// than an error or an absent capability. The roadmap left this as an open
// question and recommended exactly this: the UI can then explain why a Windows
// host shows no compute surfaces, instead of staying silent about a section the
// operator went looking for.
func unsupportedReport(why string) Report {
	return Report{
		OS:               runtime.GOOS,
		Arch:             runtime.GOARCH,
		Supported:        false,
		UnsupportedWhy:   why,
		CanRunVMs:        false,
		CanRunContainers: false,
	}
}
