// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

const (
	callTimeout  = 30 * time.Second
	dialTimeout  = 10 * time.Second
	defaultRetry = 30 * time.Second
	userAgent    = "McHarbor-Alertmanager-Webhook/2026.10.01"
)

var errBlockedAddress = errors.New("comm-alertmanager-webhook: blocked address")

// Client posts Alertmanager v4 payloads; its dialer refuses loopback, link-local and metadata
// addresses because the receiver URL is operator-supplied and could otherwise probe the host.
type Client struct {
	hc     *http.Client
	dialer *net.Dialer
	now    func() time.Time
}

// NewClient returns the production client, which never permits loopback targets.
func NewClient() *Client { return newClient(false) }

// newClient exists so tests can reach an httptest server on loopback; production never sets it.
func newClient(allowLoopback bool) *Client {
	dialer := &net.Dialer{Timeout: dialTimeout, Control: func(_, address string, _ syscall.RawConn) error {
		return checkDialAddress(address, allowLoopback)
	}}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = dialer.DialContext
	tr.Proxy = nil
	return &Client{
		now:    time.Now,
		dialer: dialer,
		hc: &http.Client{
			Timeout:       callTimeout + 5*time.Second,
			Transport:     tr,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

// checkDialAddress runs after DNS resolution, so a hostname that resolves to the metadata
// service is refused exactly like the literal address. RFC 1918 stays allowed on purpose.
func checkDialAddress(address string, allowLoopback bool) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return errBlockedAddress
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return errBlockedAddress
	}
	// Unmap catches ::ffff:169.254.169.254 and WithZone("") because Prefix.Contains is false for a zoned address.
	ip = ip.Unmap().WithZone("")
	if allowLoopback && ip.IsLoopback() {
		return nil
	}
	for _, candidate := range append([]netip.Addr{ip}, embeddedIPv4s(ip)...) {
		for _, p := range blockedPrefixes {
			if p.Contains(candidate) {
				return errBlockedAddress
			}
		}
	}
	return nil
}

// blockedPrefixes are this host, link-local, multicast and the cloud-metadata endpoints; a
// receiver on any of them could only be a probe of the host, never an Alertmanager gateway.
var blockedPrefixes = []netip.Prefix{
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

func blockedErr(origin string) error {
	return fmt.Errorf("%w: %w: receiver %s resolves to a blocked address", errBlockedAddress, incident.ErrOperatorMessage, origin)
}

// post is the one HTTP path every call takes. It never echoes the receiver's body or the URL
// path, because both can carry the integration token.
func (c *Client) post(ctx context.Context, cfg amConfig, body []byte) error {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("comm-alertmanager-webhook: build request for %s failed", cfg.Origin)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	switch cfg.AuthMode {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+cfg.Bearer)
	case "basic":
		req.SetBasicAuth(cfg.BasicUser, cfg.BasicPass)
	}
	if cfg.HMACSecret != "" {
		req.Header.Set("X-McHarbor-Signature", signature(cfg.HMACSecret, c.now(), body))
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		if errors.Is(err, errBlockedAddress) {
			return blockedErr(cfg.Origin)
		}
		return fmt.Errorf("%w: comm-alertmanager-webhook: transport to %s failed", incident.ErrRetryable, cfg.Origin)
	}
	defer resp.Body.Close()
	if _, err := io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10)); err != nil {
		return fmt.Errorf("%w: comm-alertmanager-webhook: read response from %s failed", incident.ErrRetryable, cfg.Origin)
	}
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode == http.StatusTooManyRequests:
		return &incident.RetryAfterError{After: retryAfter(resp.Header.Get("Retry-After"), c.now())}
	case resp.StatusCode >= 500:
		return fmt.Errorf("%w: comm-alertmanager-webhook: %s answered HTTP %d", incident.ErrRetryable, cfg.Origin, resp.StatusCode)
	default:
		return operatorErr("receiver %s answered HTTP %d", cfg.Origin, resp.StatusCode)
	}
}

// signature is t=<unix>,v1=<hex HMAC-SHA256 of "t.body">; the timestamp is inside the MAC so a
// gateway can reject replays by age without trusting an unsigned header.
func signature(secret string, now time.Time, body []byte) string {
	ts := strconv.FormatInt(now.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	return "t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

func retryAfter(v string, now time.Time) time.Duration {
	if v == "" {
		return defaultRetry
	}
	if s, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && s >= 0 {
		return time.Duration(s) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil && t.After(now) {
		return t.Sub(now)
	}
	return defaultRetry
}
