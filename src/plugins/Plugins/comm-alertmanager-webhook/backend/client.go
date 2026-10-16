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
	ip := net.ParseIP(host)
	if ip == nil || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast() || (ip.IsLoopback() && !allowLoopback) {
		return errBlockedAddress
	}
	return nil
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
