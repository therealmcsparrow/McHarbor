// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package egress guards host-side HTTP fetches of operator- or catalog-supplied URLs, so a
// catalog row or a catalog entry cannot point McHarbor at the cloud-metadata service or itself.
package egress

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// ErrDestinationRefused marks a fetch whose resolved address the policy forbids. Callers map it
// to their own i18n code; the wrapped text never names the resolved IP.
var ErrDestinationRefused = errors.New("egress: destination is not an allowed address")

// RFC1918 and ULA stay reachable on purpose, because a self-hosted catalog on the LAN is an
// ordinary setup; only addresses that are this host or hand out instance credentials are refused.
var (
	alwaysRefused = []netip.Prefix{
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
	loopback = []netip.Prefix{
		netip.MustParsePrefix("127.0.0.0/8"),
		netip.MustParsePrefix("::1/128"),
	}
)

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

// Policy is what a caller may reach beyond the default. AllowLoopback is for the few fetchers
// that deliberately serve from this host (the plugin installer's loopback http mirror, a local
// registry); metadata, link-local and multicast stay refused whatever it says.
type Policy struct {
	AllowLoopback bool
}

// CheckAddress refuses a resolved "ip:port" the policy forbids. A hostname is refused because
// this runs on the address the socket is about to connect to, after resolution.
func CheckAddress(address string, p Policy) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%w: unparseable address", ErrDestinationRefused)
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return fmt.Errorf("%w: not an IP literal", ErrDestinationRefused)
	}
	return CheckIP(ip, p)
}

// CheckIP refuses ip, and every IPv4 address it embeds for a translator or tunnel, when the
// policy forbids it. Unmap and an empty zone come first because Prefix.Contains is false for both.
func CheckIP(ip netip.Addr, p Policy) error {
	ip = ip.Unmap().WithZone("")
	for _, candidate := range append([]netip.Addr{ip}, embeddedIPv4s(ip)...) {
		for _, prefix := range alwaysRefused {
			if prefix.Contains(candidate) {
				return ErrDestinationRefused
			}
		}
		if p.AllowLoopback {
			continue
		}
		for _, prefix := range loopback {
			if prefix.Contains(candidate) {
				return ErrDestinationRefused
			}
		}
	}
	return nil
}

// embeddedIPv4s returns every IPv4 address ip encodes: NAT64, 6to4, Teredo (server and
// de-obfuscated client), ISATAP and the IPv4-compatible form. A false positive costs one
// unusual address; a miss hands out instance credentials.
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

// Options configures NewClient.
type Options struct {
	// Timeout bounds the whole request, as http.Client.Timeout does; zero means none.
	Timeout time.Duration
	Policy  Policy
}

// maxRedirects bounds a redirect chain; a CDN alias needs one or two.
const maxRedirects = 5

// proxyResolveTimeout bounds the lookup done before a request is handed to a proxy.
const proxyResolveTimeout = 10 * time.Second

// NewClient returns a client whose dialer checks every address it connects to, whose redirects
// may not downgrade https to http, and which keeps honouring HTTP(S)_PROXY for operators behind
// one, after resolving the target and checking it first, since the dial then only sees the proxy.
func NewClient(o Options) *http.Client {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			if strings.HasPrefix(network, "unix") {
				return ErrDestinationRefused
			}
			return CheckAddress(address, o.Policy)
		},
	}
	transport := &http.Transport{
		Proxy:                 checkedProxy(o.Policy, http.ProxyFromEnvironment, net.DefaultResolver),
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	return &http.Client{
		Transport:     transport,
		Timeout:       o.Timeout,
		CheckRedirect: CheckRedirect,
	}
}

// CheckRedirect is NewClient's redirect policy, exported so a caller with a stricter scheme
// rule can still reuse the hop limit. Each hop is dialled again, so its address is re-checked.
func CheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("stopped after %d redirects", maxRedirects)
	}
	if req.URL.Scheme != "https" && req.URL.Scheme != "http" {
		return fmt.Errorf("%w: redirect to scheme %q", ErrDestinationRefused, req.URL.Scheme)
	}
	if len(via) > 0 && via[len(via)-1].URL.Scheme == "https" && req.URL.Scheme != "https" {
		return fmt.Errorf("%w: redirect leaves https", ErrDestinationRefused)
	}
	return nil
}

// lookuper is the part of net.Resolver checkedProxy needs, so a test can stub resolution.
type lookuper interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// checkedProxy wraps a proxy selector so a proxied request's target is resolved and checked
// before the proxy sees it. The proxy resolves again, so a rebinding name can still slip past;
// that residual is the price of keeping proxied installs able to reach a catalog at all.
func checkedProxy(p Policy, next func(*http.Request) (*url.URL, error), resolver lookuper) func(*http.Request) (*url.URL, error) {
	return func(req *http.Request) (*url.URL, error) {
		proxyURL, err := next(req)
		if err != nil || proxyURL == nil {
			return proxyURL, err
		}
		host := req.URL.Hostname()
		if ip, parseErr := netip.ParseAddr(host); parseErr == nil {
			return proxyURL, CheckIP(ip, p)
		}
		ctx, cancel := context.WithTimeout(req.Context(), proxyResolveTimeout)
		defer cancel()
		addrs, err := resolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, fmt.Errorf("resolving %s before proxying: %w", host, err)
		}
		for _, addr := range addrs {
			if err := CheckIP(addr, p); err != nil {
				return nil, err
			}
		}
		return proxyURL, nil
	}
}
