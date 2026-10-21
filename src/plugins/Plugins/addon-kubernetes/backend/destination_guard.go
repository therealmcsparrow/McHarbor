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
	for _, p := range refusedDestinations {
		if p.Contains(ip) {
			return errDestinationRefused
		}
	}
	return nil
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

// applyDestinationPolicy installs the dial guard and the body stripper on cfg. A nil trust
// keeps the stripper unconditional.
func applyDestinationPolicy(cfg *rest.Config, trust *apiTrust) {
	cfg.Dial = guardedDial
	prev := cfg.WrapTransport
	cfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		if prev != nil {
			rt = prev(rt)
		}
		return bodyStrippingTransport{next: rt, trust: trust}
	}
}
