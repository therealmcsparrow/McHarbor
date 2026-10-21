// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"errors"
	"net"
	"net/netip"
	"strings"
	"syscall"
	"time"
)

// errDestinationRefused marks a dial to an address the operator-set SMB server may never reach:
// cloud metadata, link-local, unspecified, multicast and broadcast.
var errDestinationRefused = errors.New("storage-samba: the SMB server address is not an allowed destination")

// RFC1918, ULA and loopback stay reachable: a LAN NAS or a Samba sidecar sharing this host's
// network is an ordinary setup. Only metadata endpoints and never-real-server addresses are refused.
var refusedDestinations = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("100.100.100.200/32"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("255.255.255.255/32"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
	netip.MustParsePrefix("fd00:ec2::/32"),
}

// Translation prefixes whose addresses carry an IPv4 address a gateway on the path may
// forward to, so 64:ff9b::a9fe:a9fe reaches 169.254.169.254 behind a NAT64.
var (
	nat64WellKnown = netip.MustParsePrefix("64:ff9b::/96")
	nat64Local     = netip.MustParsePrefix("64:ff9b:1::/48")
	sixToFour      = netip.MustParsePrefix("2002::/16")
	teredo         = netip.MustParsePrefix("2001::/32")
	ipv4Compatible = netip.MustParsePrefix("::/96")
)

// rfc6052Offsets are the byte positions of the embedded IPv4 for a /48, /56, /64 and /96
// NAT64 prefix; the u octet (byte 8) is never part of the address. A local-use /48 may be
// carved into any of the four, so every position is checked.
var rfc6052Offsets = [][4]int{
	{6, 7, 9, 10},
	{7, 9, 10, 11},
	{9, 10, 11, 12},
	{12, 13, 14, 15},
}

// checkDestination refuses a resolved "ip:port" the policy forbids. It runs on the address the
// socket is about to connect to, so a name that resolves or rebinds to a refused IP is caught
// the same way as a literal.
func checkDestination(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return errDestinationRefused
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return errDestinationRefused
	}
	// Unmap catches ::ffff:169.254.169.254 and WithZone("") because Prefix.Contains is false for a zoned address.
	ip = ip.Unmap().WithZone("")
	for _, candidate := range append([]netip.Addr{ip}, embeddedIPv4s(ip)...) {
		for _, p := range refusedDestinations {
			if p.Contains(candidate) {
				return errDestinationRefused
			}
		}
	}
	return nil
}

// embeddedIPv4s returns every IPv4 address ip encodes for a translator or tunnel: NAT64,
// 6to4, Teredo (server and de-obfuscated client), ISATAP and the deprecated IPv4-compatible
// form. A false positive costs one unusual address; a miss hands out instance credentials.
func embeddedIPv4s(ip netip.Addr) []netip.Addr {
	if !ip.Is6() {
		return nil
	}
	b := ip.As16()
	at := func(i, j, k, l int) netip.Addr { return netip.AddrFrom4([4]byte{b[i], b[j], b[k], b[l]}) }
	var out []netip.Addr
	switch {
	case nat64WellKnown.Contains(ip):
		out = append(out, at(12, 13, 14, 15))
	case ipv4Compatible.Contains(ip):
		// :: is refused as itself and ::1 is loopback; the rest of ::/96 is the deprecated
		// IPv4-compatible form, which some stacks still route as IPv4.
		if ip != netip.IPv6Unspecified() && ip != netip.IPv6Loopback() {
			out = append(out, at(12, 13, 14, 15))
		}
	case nat64Local.Contains(ip):
		for _, o := range rfc6052Offsets {
			out = append(out, at(o[0], o[1], o[2], o[3]))
		}
	case sixToFour.Contains(ip):
		out = append(out, at(2, 3, 4, 5))
	case teredo.Contains(ip):
		out = append(out, at(4, 5, 6, 7),
			netip.AddrFrom4([4]byte{b[12] ^ 0xff, b[13] ^ 0xff, b[14] ^ 0xff, b[15] ^ 0xff}))
	}
	// ISATAP is an interface identifier, so it can sit under any /64.
	if b[8]&^0x03 == 0 && b[9] == 0 && b[10] == 0x5e && b[11] == 0xfe {
		out = append(out, at(12, 13, 14, 15))
	}
	return out
}

// guardedDialer is the one dialer sambaDial uses; Control sees the address after DNS resolution,
// so a rebinding name is caught, and a unix socket is refused because the server names a host.
func guardedDialer(timeout time.Duration) *net.Dialer {
	return &net.Dialer{
		Timeout: timeout,
		Control: func(network, address string, _ syscall.RawConn) error {
			if !strings.HasPrefix(network, "tcp") && !strings.HasPrefix(network, "udp") {
				return errDestinationRefused
			}
			return checkDestination(address)
		},
	}
}
