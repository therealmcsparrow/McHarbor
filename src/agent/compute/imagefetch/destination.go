// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package imagefetch

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"syscall"
	"time"
)

// ErrDestinationRefused marks an image, checksum or signature URL whose resolved address the
// agent may never fetch from: this host, link-local, multicast or a cloud-metadata endpoint.
var ErrDestinationRefused = errors.New("imagefetch: the image source address is not an allowed destination")

// RFC1918 and ULA stay reachable on purpose, because an operator's local mirror is an ordinary
// setup. Loopback is refused: the agent runs as root beside the Docker API, a hypervisor and
// whatever else listens on 127.0.0.1, and a mirror on the same machine is reached by its LAN address.
var refusedDestinations = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("100.100.100.200/32"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("255.255.255.255/32"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
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
// NAT64 prefix; a local-use /48 may be carved into any of them, so every position is checked.
var rfc6052Offsets = [][4]int{
	{6, 7, 9, 10},
	{7, 9, 10, 11},
	{9, 10, 11, 12},
	{12, 13, 14, 15},
}

// checkDestination refuses a resolved "ip:port" the policy forbids. It runs on the address the
// socket is about to connect to, so a name that resolves or rebinds to a refused IP, and a
// redirect to one, are caught like a literal. allowLoopback exists for tests only.
func checkDestination(address string, allowLoopback bool) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%w: unparseable address", ErrDestinationRefused)
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return fmt.Errorf("%w: not an IP literal", ErrDestinationRefused)
	}
	// Unmap catches ::ffff:169.254.169.254 and WithZone("") because Prefix.Contains is false for a zoned address.
	ip = ip.Unmap().WithZone("")
	if allowLoopback && ip.IsLoopback() {
		return nil
	}
	for _, candidate := range append([]netip.Addr{ip}, embeddedIPv4s(ip)...) {
		for _, p := range refusedDestinations {
			if p.Contains(candidate) {
				return ErrDestinationRefused
			}
		}
	}
	return nil
}

// embeddedIPv4s returns every IPv4 address ip encodes for a translator or tunnel: NAT64,
// 6to4, Teredo (server and de-obfuscated client), ISATAP and the IPv4-compatible form.
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

// maxMirrorRedirects bounds a redirect chain; a mirror redirector needs one or two.
const maxMirrorRedirects = 10

// newMirrorClient returns the client every mirror fetch uses. The dialled address is checked,
// the environment proxy is never consulted (through a proxy the check would see only the proxy),
// and a redirect must stay https, since validateSourceURL only ever sees the first URL.
func newMirrorClient(allowLoopback bool) *http.Client {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			if strings.HasPrefix(network, "unix") {
				return ErrDestinationRefused
			}
			return checkDestination(address, allowLoopback)
		},
	}
	return &http.Client{
		// No overall client timeout: that would cap a legitimate multi-gigabyte
		// download. The per-request context carries the real budget, and the
		// transport timeouts below catch a dead peer.
		Transport: &http.Transport{
			Proxy:                 nil,
			DialContext:           dialer.DialContext,
			TLSHandshakeTimeout:   30 * time.Second,
			ResponseHeaderTimeout: 60 * time.Second,
			IdleConnTimeout:       90 * time.Second,
			ForceAttemptHTTP2:     true,
			// Without this Go asks for gzip and unpacks it out of sight. A CDN
			// will gzip a disk image on the fly when asked (AlmaLinux's does),
			// and a bad trailer from one edge then fails the whole download
			// with "gzip: invalid checksum"; it also hides the real size from
			// the progress bar. The bytes hashed are the identity bytes anyway.
			DisableCompression: true,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxMirrorRedirects {
				return fmt.Errorf("imagefetch: stopped after %d redirects", maxMirrorRedirects)
			}
			if req.URL.Scheme != "https" {
				return fmt.Errorf("%w: redirect leaves https", ErrDestinationRefused)
			}
			return nil
		},
	}
}
