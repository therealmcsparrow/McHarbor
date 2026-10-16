// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// transferPortFromListen extracts the port from a listen address such as
// "0.0.0.0:8788". It returns 0 when the address has no usable port, which
// disables reflexive candidate construction on the server.
func transferPortFromListen(listen string) int {
	listen = strings.TrimSpace(listen)
	if listen == "" {
		return 0
	}
	_, portStr, err := net.SplitHostPort(listen)
	if err != nil {
		return 0
	}
	port, err := strconv.Atoi(strings.TrimSpace(portStr))
	if err != nil || port <= 0 || port > 65535 {
		return 0
	}
	return port
}

// deriveLocalCandidates builds one base URL per usable local interface address,
// so a peer agent on the same network can reach this receiver without anyone
// configuring an advertise URL by hand.
//
// Loopback, link-local, multicast and unspecified addresses are skipped: none of
// them are reachable from another host, and advertising them only costs the
// source agent a failed probe. IPv6 addresses are bracketed so the resulting
// string is a valid URL authority.
//
// The listen address is honoured when it names a concrete interface. Binding to
// 10.0.0.5 and then advertising 192.168.1.5 would produce a candidate that can
// never accept a connection.
// primaryOutboundIP reports the address this host would send from.
//
// The UDP dial sends nothing — connecting a datagram socket only asks the
// kernel to pick a route and bind a source address, which is exactly the
// question being asked. It answers correctly on a host-networked agent, where
// enumerating interfaces would offer a Docker bridge with equal confidence.
//
// Empty when the host has no route out, which is a real answer: an agent on an
// isolated segment has no address worth showing an operator.
func primaryOutboundIP(ctx context.Context) string {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "udp", "192.0.2.1:9")
	if err != nil {
		return ""
	}
	defer func() { errlog.Close(conn) }()

	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr.IP == nil || addr.IP.IsUnspecified() || addr.IP.IsLoopback() {
		return ""
	}
	return addr.IP.String()
}

func deriveLocalCandidates(listen string) []string {
	port := transferPortFromListen(listen)
	if port == 0 {
		return nil
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(listen))
	if err != nil {
		return nil
	}
	host = strings.TrimSpace(host)

	// A concrete bind address advertises only itself.
	if host != "" && host != "0.0.0.0" && host != "::" && host != "[::]" {
		if addr := net.ParseIP(host); addr != nil {
			if candidate := candidateForIP(addr, port); candidate != "" {
				return []string{candidate}
			}
			return nil
		}
		// A hostname was given; trust the operator and advertise it verbatim.
		return []string{fmt.Sprintf("http://%s:%d", host, port)}
	}

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}

	seen := make(map[string]struct{}, len(addrs))
	candidates := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		candidate := candidateForIP(ipNet.IP, port)
		if candidate == "" {
			continue
		}
		if _, exists := seen[candidate]; exists {
			continue
		}
		seen[candidate] = struct{}{}
		candidates = append(candidates, candidate)
	}
	return candidates
}

// candidateForIP renders one base URL for an address, or "" when the address
// cannot be reached from another host.
func candidateForIP(ip net.IP, port int) string {
	if ip == nil || ip.IsLoopback() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return ""
	}
	if ip4 := ip.To4(); ip4 != nil {
		return fmt.Sprintf("http://%s:%d", ip4.String(), port)
	}
	return fmt.Sprintf("http://[%s]:%d", ip.String(), port)
}
