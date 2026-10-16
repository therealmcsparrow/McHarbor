// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package networks

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
)

// localLinks lists this host's links the way an agent's NetDetect does, for a McHarbor that
// runs directly on the host it manages. Only the standard library is used: netlink would be a
// new backend dependency, and every plugin links the backend module.
func localLinks() ([]coreagent.ComputeHostLink, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("listing interfaces: %w", err)
	}
	routes, err := os.ReadFile("/proc/net/route")
	if err != nil {
		routes = nil
	}
	defaultIface := defaultRouteInterface(string(routes))

	out := make([]coreagent.ComputeHostLink, 0, len(ifaces))
	for _, iface := range ifaces {
		sys := filepath.Join("/sys/class/net", iface.Name)
		uevent, err := os.ReadFile(filepath.Join(sys, "uevent"))
		if err != nil {
			uevent = nil
		}
		_, deviceErr := os.Stat(filepath.Join(sys, "device"))
		link := coreagent.ComputeHostLink{
			Name:            iface.Name,
			Kind:            classifyLink(iface.Name, ueventDevtype(string(uevent)), deviceErr == nil),
			MTU:             iface.MTU,
			Up:              iface.Flags&net.FlagUp != 0,
			Managed:         strings.HasPrefix(iface.Name, "mcharbor-"),
			HasDefaultRoute: iface.Name == defaultIface,
		}
		if len(iface.HardwareAddr) > 0 {
			link.MAC = iface.HardwareAddr.String()
		}
		if addrs, err := iface.Addrs(); err == nil {
			for _, a := range addrs {
				link.Addresses = append(link.Addresses, a.String())
			}
		}
		out = append(out, link)
	}
	return out, nil
}
