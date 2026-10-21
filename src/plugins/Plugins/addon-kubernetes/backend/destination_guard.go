// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"

	"k8s.io/client-go/rest"
)

// errDestinationRefused marks a dial to an address a cluster connection may never reach:
// link-local space and the cloud-metadata endpoints.
var errDestinationRefused = errors.New("kubernetes: the cluster address is not an allowed destination")

// Loopback and RFC1918/ULA stay reachable on purpose, because local kind and k3s clusters
// live there; only addresses that hand out instance credentials or are never a cluster are refused.
var refusedDestinations = []netip.Prefix{
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("fd00:ec2::/32"),
	netip.MustParsePrefix("100.100.100.200/32"),
}

// checkDestination refuses a resolved "ip:port" inside refusedDestinations. It runs on the
// address the socket is about to connect to, so a hostname that resolves or rebinds to a
// refused IP, and a redirect to one, are caught the same way as a literal.
func checkDestination(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%w: unparseable address", errDestinationRefused)
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return fmt.Errorf("%w: not an IP literal", errDestinationRefused)
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
	case nat64WellKnown.Contains(ip), ipv4Compatible.Contains(ip):
		out = append(out, at(12, 13, 14, 15))
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

// guardedDialer checks every connect attempt after name resolution, in the socket's Control hook.
var guardedDialer = &net.Dialer{
	Timeout:   30 * time.Second,
	KeepAlive: 30 * time.Second,
	Control: func(network, address string, _ syscall.RawConn) error {
		if strings.HasPrefix(network, "unix") {
			return errDestinationRefused
		}
		return checkDestination(address)
	},
}

// guardedDial is the rest.Config Dial for every cluster connection.
func guardedDial(ctx context.Context, network, address string) (net.Conn, error) {
	return guardedDialer.DialContext(ctx, network, address)
}

// bodyStrippingTransport drops the body of every non-2xx reply. client-go copies that body
// into its error text, which the handlers return as `detail`, so a cluster address that points
// at some other internal service would otherwise read that service's reply back; the status
// code survives and client-go builds its generic error for the class from it. Once trust says
// the address answered a real /version probe, a capped `kind: Status` body is kept, because
// that message (RBAC, admission, validation) is the operator's real diagnosis.
type bodyStrippingTransport struct {
	next  http.RoundTripper
	trust *apiTrust
}

func (t bodyStrippingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.next.RoundTrip(req)
	if err != nil || resp == nil {
		return resp, err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil
	}
	if resp.Body != nil && t.trust.confirmed() && keepStatusBody(resp) {
		return resp, nil
	}
	if resp.Body != nil {
		// The body is being discarded, so a failed close has nothing left to report.
		_ = resp.Body.Close()
	}
	resp.Body = http.NoBody
	resp.ContentLength = 0
	resp.Header.Del("Content-Type")
	resp.Header.Del("Content-Length")
	return resp, nil
}

// keepStatusBody buffers a small reply and keeps it only when it is a Kubernetes `Status`
// object; anything else (an ingress error page, an oversized body) is left for the caller to drop.
func keepStatusBody(resp *http.Response) bool {
	// An oversized reply is cut at the cap, which leaves invalid JSON, so it is refused below.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxStatusBody))
	// The body is replaced or discarded below either way.
	_ = resp.Body.Close()
	if err != nil {
		return false
	}
	var head struct {
		Kind string `json:"kind"`
	}
	if json.Unmarshal(raw, &head) != nil || head.Kind != "Status" {
		return false
	}
	resp.Body = io.NopCloser(bytes.NewReader(raw))
	resp.ContentLength = int64(len(raw))
	return true
}

// noProxy is the rest.Config Proxy for every cluster connection. client-go falls back to
// HTTP(S)_PROXY when Proxy is nil, and through a proxy the dial guard would only ever see the
// proxy's address, so the cluster is always dialled directly and checked itself.
func noProxy(*http.Request) (*url.URL, error) { return nil, nil }

// applyDestinationPolicy installs the dial guard, the direct-dial proxy policy and the body
// stripper on cfg. A nil trust keeps the stripper unconditional.
func applyDestinationPolicy(cfg *rest.Config, trust *apiTrust) {
	cfg.Dial = guardedDial
	cfg.Proxy = noProxy
	prev := cfg.WrapTransport
	cfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		if prev != nil {
			rt = prev(rt)
		}
		return bodyStrippingTransport{next: rt, trust: trust}
	}
}
