// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package httpx resolves the scheme, base URL and client address of a request behind proxies.
package httpx

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// Client-address attribution sources, stored alongside the resolved address
// so an operator reading the history can tell how much to believe it.
const (
	// ClientAddressSourcePeer means no forwarding header was trusted and
	// the client is the socket peer. This is the safe default.
	ClientAddressSourcePeer = "peer"
	// ClientAddressSourceForwarded means an RFC 7239 Forwarded header from
	// a trusted peer supplied the client.
	ClientAddressSourceForwarded = "forwarded"
	// ClientAddressSourceXForwardedFor means X-Forwarded-For from a trusted
	// peer supplied the client.
	ClientAddressSourceXForwardedFor = "x-forwarded-for"
	// ClientAddressSourceXRealIP means X-Real-IP from a trusted peer
	// supplied the client.
	ClientAddressSourceXRealIP = "x-real-ip"
)

// ClientAddress is the outcome of attributing a request to an origin.
// PeerIP is always the address McHarbor's own socket saw. ClientIP is the
// address it is willing to believe is the origin, which equals PeerIP unless
// a trusted proxy said otherwise.
type ClientAddress struct {
	PeerIP   string
	ClientIP string
	Source   string
}

// ClientAddressResolver attributes requests to an origin address, honouring
// forwarding headers only from peers inside configured trusted CIDRs.
//
// This deliberately does not reuse the repository's older ClientIP helper.
// That helper takes the first X-Forwarded-For value from any peer, which is
// fine for a best-effort log line and unusable for security attribution: any
// client can send that header. Here an untrusted peer's headers are ignored
// outright, so a spoofed X-Forwarded-For cannot move ClientIP.
//
// The zero value trusts nothing, which is the correct default for a server
// exposed directly. A resolver is immutable after construction and safe for
// concurrent use.
type ClientAddressResolver struct {
	trusted []netip.Prefix
}

// NewClientAddressResolver parses trusted proxy CIDRs. Each value may be a
// CIDR ("10.0.0.0/8") or a bare address ("192.0.2.7"), which is treated as a
// single-host prefix. Blank entries are skipped so a trailing comma in the
// environment variable is not an error.
//
// A malformed value is a startup error rather than a silent skip: an
// operator who mistypes a CIDR would otherwise get a server that quietly
// ignores its reverse proxy and mis-attributes every request.
func NewClientAddressResolver(values []string) (*ClientAddressResolver, error) {
	resolver := &ClientAddressResolver{}

	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}

		if prefix, err := netip.ParsePrefix(value); err == nil {
			resolver.trusted = append(resolver.trusted, prefix.Masked())
			continue
		}

		addr, err := netip.ParseAddr(value)
		if err != nil {
			return nil, fmt.Errorf("trusted proxy %q is neither a CIDR nor an IP address: %w", value, err)
		}
		addr = normalizeAddr(addr)
		resolver.trusted = append(resolver.trusted, netip.PrefixFrom(addr, addr.BitLen()))
	}

	return resolver, nil
}

// TrustsAnything reports whether any trusted prefix is configured. Useful
// for a startup log line so operators can see which mode they are in.
func (r *ClientAddressResolver) TrustsAnything() bool {
	return r != nil && len(r.trusted) > 0
}

// Resolve attributes req to an origin address.
func (r *ClientAddressResolver) Resolve(req *http.Request) ClientAddress {
	peer, peerOK := peerAddr(req)
	out := ClientAddress{Source: ClientAddressSourcePeer}
	if peerOK {
		out.PeerIP = peer.String()
		out.ClientIP = out.PeerIP
	}

	// An untrusted (or unparseable) peer gets no say in its own identity.
	if r == nil || len(r.trusted) == 0 || !peerOK || !r.isTrusted(peer) {
		return out
	}

	if addr, ok := r.fromForwarded(req.Header.Values("Forwarded")); ok {
		out.ClientIP = addr.String()
		out.Source = ClientAddressSourceForwarded
		return out
	}
	if addr, ok := r.fromChain(req.Header.Values("X-Forwarded-For")); ok {
		out.ClientIP = addr.String()
		out.Source = ClientAddressSourceXForwardedFor
		return out
	}
	if addr, ok := parseCandidate(firstNonEmpty(req.Header.Values("X-Real-IP"))); ok {
		out.ClientIP = addr.String()
		out.Source = ClientAddressSourceXRealIP
		return out
	}

	return out
}

// isTrusted reports whether addr falls inside a configured prefix.
func (r *ClientAddressResolver) isTrusted(addr netip.Addr) bool {
	for _, prefix := range r.trusted {
		// A v4 address is never inside a v6 prefix and vice versa; both
		// sides are already unmapped, so Contains does the right thing.
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// fromChain walks a comma-separated forwarding chain right to left and
// returns the first address that is not itself a trusted proxy — the
// closest hop we have no reason to believe is ours.
//
// Taking the leftmost value instead, as the older helper does, hands the
// answer to whoever sent the header: a client may prepend anything it likes.
// Walking from the right means an attacker can only inject values that sit
// behind hops we already trust.
func (r *ClientAddressResolver) fromChain(headers []string) (netip.Addr, bool) {
	candidates := splitChain(headers)
	if len(candidates) == 0 {
		return netip.Addr{}, false
	}

	var leftmost netip.Addr
	var haveLeftmost bool

	for i := len(candidates) - 1; i >= 0; i-- {
		addr, ok := parseCandidate(candidates[i])
		if !ok {
			// A malformed hop breaks the chain's meaning; anything
			// further left is no longer attributable.
			break
		}
		leftmost, haveLeftmost = addr, true
		if !r.isTrusted(addr) {
			return addr, true
		}
	}

	// Every hop was a trusted proxy. The furthest one is the best
	// available answer; it is at least not attacker-supplied.
	return leftmost, haveLeftmost
}

// fromForwarded parses RFC 7239 Forwarded elements and applies the same
// right-to-left rule to their for= parameters.
func (r *ClientAddressResolver) fromForwarded(headers []string) (netip.Addr, bool) {
	var chain []string
	for _, header := range headers {
		for _, element := range strings.Split(header, ",") {
			for _, param := range strings.Split(element, ";") {
				key, value, found := strings.Cut(param, "=")
				if !found || !strings.EqualFold(strings.TrimSpace(key), "for") {
					continue
				}
				chain = append(chain, strings.TrimSpace(value))
			}
		}
	}
	if len(chain) == 0 {
		return netip.Addr{}, false
	}
	return r.fromChain([]string{strings.Join(chain, ",")})
}

// splitChain flattens repeated headers and their comma-separated values.
func splitChain(headers []string) []string {
	var out []string
	for _, header := range headers {
		for _, part := range strings.Split(header, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

// parseCandidate turns one forwarding-chain entry into an address.
// It rejects the RFC 7239 "unknown" placeholder, obfuscated identifiers
// (which begin with an underscore), and anything that is not an IP.
func parseCandidate(raw string) (netip.Addr, bool) {
	value := strings.TrimSpace(raw)
	value = strings.Trim(value, `"`)
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "unknown") || strings.HasPrefix(value, "_") {
		return netip.Addr{}, false
	}

	// Bracketed IPv6, optionally with a port: [2001:db8::1]:443
	if strings.HasPrefix(value, "[") {
		end := strings.Index(value, "]")
		if end < 0 {
			return netip.Addr{}, false
		}
		value = value[1:end]
	} else if strings.Count(value, ":") == 1 {
		// host:port for IPv4 or a hostname. A bare IPv6 has more colons.
		if host, _, err := net.SplitHostPort(value); err == nil {
			value = host
		}
	}

	addr, err := netip.ParseAddr(value)
	if err != nil {
		return netip.Addr{}, false
	}
	return normalizeAddr(addr), true
}

// peerAddr parses the socket peer out of RemoteAddr.
func peerAddr(req *http.Request) (netip.Addr, bool) {
	if req == nil {
		return netip.Addr{}, false
	}
	remote := strings.TrimSpace(req.RemoteAddr)
	if remote == "" {
		return netip.Addr{}, false
	}

	if host, _, err := net.SplitHostPort(remote); err == nil {
		remote = host
	}
	remote = strings.Trim(remote, "[]")

	addr, err := netip.ParseAddr(remote)
	if err != nil {
		// Unix-socket and test servers can present a non-IP peer. That is
		// not an error; it simply means there is no address to record.
		return netip.Addr{}, false
	}
	return normalizeAddr(addr), true
}

// normalizeAddr unmaps 4-in-6 addresses and drops the zone so stored values
// and prefix comparisons use one canonical form.
func normalizeAddr(addr netip.Addr) netip.Addr {
	return addr.Unmap().WithZone("")
}

func firstNonEmpty(values []string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// TrustsAddress reports whether ip parses and falls inside a trusted prefix.
// A nil resolver, an empty string and a non-IP value all report false.
func (r *ClientAddressResolver) TrustsAddress(ip string) bool {
	if r == nil || len(r.trusted) == 0 {
		return false
	}
	addr, ok := parseCandidate(ip)
	if !ok {
		return false
	}
	return r.isTrusted(addr)
}
