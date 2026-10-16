// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package networks

import (
	"strings"
	"sync/atomic"
)

// nativeHost is set when McHarbor runs directly on its host. Only then do its own network
// interfaces belong to the host behind a local Docker socket; inside a container they are the
// container's namespace.
var nativeHost atomic.Bool

// SetNativeHost tells the networks module whether McHarbor runs directly on its host.
func SetNativeHost(native bool) {
	nativeHost.Store(native)
}

// classifyLink maps sysfs facts onto the kinds the agent reports, so a local listing and an
// agent listing filter alike. devtype is the DEVTYPE from the link's uevent; hasDevice is
// whether /sys/class/net/<name>/device exists, which only hardware-backed links have.
func classifyLink(name, devtype string, hasDevice bool) string {
	if name == "lo" {
		return "loopback"
	}
	switch devtype {
	case "bridge", "bond", "vlan":
		return devtype
	case "wlan":
		return "physical"
	case "":
		if hasDevice {
			return "physical"
		}
	}
	return "unknown"
}

// ueventDevtype reads DEVTYPE out of a sysfs uevent file's content.
func ueventDevtype(uevent string) string {
	for _, line := range strings.Split(uevent, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "DEVTYPE="); ok {
			return v
		}
	}
	return ""
}

// defaultRouteInterface names the interface the IPv4 default route leaves by, from the
// content of /proc/net/route, or "" when there is none.
func defaultRouteInterface(routes string) string {
	for i, line := range strings.Split(routes, "\n") {
		fields := strings.Fields(line)
		if i == 0 || len(fields) < 8 {
			continue
		}
		if fields[1] == "00000000" && fields[7] == "00000000" {
			return fields[0]
		}
	}
	return ""
}
