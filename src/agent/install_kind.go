// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"os"
	"strings"
)

// Install kinds reported to the server at authentication.
const (
	// InstallKindNative is the agent running directly on the host, normally as
	// a systemd unit. Only this kind can host guests.
	InstallKindNative = "native"
	// InstallKindContainer is the agent running inside a container. Everything
	// except guest hosting works.
	InstallKindContainer = "container"
)

// detectInstallKind reports whether this agent runs in a container.
//
// It exists because the difference is invisible in the UI and decides whether
// the host can run virtual machines at all — an operator who cannot see it
// spends the diagnosis on a capability panel instead of on one field.
//
// Three signals, because no single one is reliable across runtimes: Docker's
// marker file, Podman's, and the cgroup path, which names the runtime on
// cgroup v1 hosts and on cgroup v2 hosts that still nest the container under
// a docker/containerd scope.
func detectInstallKind() string {
	for _, marker := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(marker); err == nil {
			return InstallKindContainer
		}
	}

	if raw, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		body := string(raw)
		for _, needle := range []string{"/docker/", "/docker-", "containerd", "/lxc/", "kubepods"} {
			if strings.Contains(body, needle) {
				return InstallKindContainer
			}
		}
	}

	return InstallKindNative
}
