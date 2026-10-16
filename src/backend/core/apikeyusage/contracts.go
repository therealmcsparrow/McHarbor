// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package apikeyusage carries the hot-path contracts for per-API-key request
// telemetry: what an event is, where events go, and how the few free-text
// fields are made safe before they leave the request goroutine.
//
// The package imports nothing outside the standard library on purpose.
// core/rbac needs Sink so the Bearer middleware can record attributable
// rejections, and the recording middleware needs the principal that same
// package produces. Keeping the contracts dependency-neutral and injecting
// the two resolvers as function values breaks what would otherwise be an
// import cycle, and it keeps the SQL, batching and pruning in
// modules/api_keys where the repository's layering rules put them.
//
// This is operational telemetry for diagnosing access, not a billing ledger.
// Nothing here may carry a credential: no token, no token hash, no
// Authorization header, no cookies, no request or response body, no
// arbitrary headers, and no query-string values.
package apikeyusage

import (
	"context"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

// Outcome explains how the API-key authentication attempt itself resolved.
// It is deliberately separate from the HTTP status: an accepted key that
// then hits a permission denial is Accepted with status 403, which is the
// distinction an operator needs when reading the history.
type Outcome string

const (
	// OutcomeAccepted means key authentication succeeded. StatusCode
	// describes the downstream result, including 403 and 5xx.
	OutcomeAccepted Outcome = "accepted"
	// OutcomeRevoked means a known but revoked key was presented.
	OutcomeRevoked Outcome = "revoked"
	// OutcomeExpired means a known but expired key was presented.
	OutcomeExpired Outcome = "expired"
	// OutcomeOwnerInvalid means the key resolved but its owner is missing
	// or disabled.
	OutcomeOwnerInvalid Outcome = "owner_invalid"
)

// Valid reports whether o is one of the four recorded outcomes.
func (o Outcome) Valid() bool {
	switch o {
	case OutcomeAccepted, OutcomeRevoked, OutcomeExpired, OutcomeOwnerInvalid:
		return true
	default:
		return false
	}
}

const (
	// RouteUnresolved is stored when a request is rejected before chi has
	// matched a pattern. Storing the raw URL instead would put unbounded
	// cardinality — and query-string values — into the table.
	RouteUnresolved = "unresolved"

	// MaxUserAgentBytes caps the stored User-Agent. Real agents are far
	// shorter; the cap exists so a hostile client cannot grow the table a
	// megabyte at a time.
	MaxUserAgentBytes = 512

	// SourcePeer is the client-IP attribution used when no forwarding
	// header may be trusted. It is the safe default.
	SourcePeer = "peer"
)

// Event is one attributable API-key request. It holds only values already
// derived from the request; no *http.Request is retained past the enqueue
// call, so an event may safely outlive the request goroutine.
type Event struct {
	APIKeyID       string
	OccurredAt     time.Time
	Method         string
	RoutePattern   string
	StatusCode     int
	Duration       time.Duration
	Outcome        Outcome
	EnvironmentID  string
	PeerIP         string
	ClientIP       string
	ClientIPSource string
	UserAgent      string
}

// Sink accepts events. Implementations must not block the caller: the
// request path calls Record inline, so a full queue drops and counts rather
// than waiting. Record must be safe for concurrent use.
type Sink interface {
	Record(Event)
}

// NopSink discards every event. It is the zero-configuration default so
// call sites never need a nil check.
type NopSink struct{}

// Record implements Sink by doing nothing.
func (NopSink) Record(Event) {}

// KeyIDResolver returns the API-key id behind a request context, or "" when
// the request authenticated with a session cookie. Wiring supplies an
// adapter over rbac.APIKeyFromContext.
type KeyIDResolver func(ctx context.Context) string

// AddressResolverFunc returns the direct socket peer, the resolved client,
// and which signal produced the client value. Wiring supplies an adapter
// over httpx.ClientAddressResolver.
type AddressResolverFunc func(r *http.Request) (peerIP, clientIP, source string)

// SanitizeUserAgent strips control characters and caps the result at
// MaxUserAgentBytes on a rune boundary. A User-Agent is attacker-controlled
// free text; it reaches a table and later a UI table cell, so it is
// normalized once, here, rather than at each read.
func SanitizeUserAgent(raw string) string {
	if raw == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range raw {
		// Drop C0/C1 controls and DEL. Everything printable survives,
		// including non-ASCII, which real agents do contain.
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			continue
		}
		if r == utf8.RuneError {
			continue
		}
		b.WriteRune(r)
	}

	out := strings.TrimSpace(b.String())
	if len(out) <= MaxUserAgentBytes {
		return out
	}

	// Truncate on a rune boundary so the stored value stays valid UTF-8.
	cut := out[:MaxUserAgentBytes]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return strings.TrimSpace(cut)
}

// NormalizeRoutePattern returns pattern, or RouteUnresolved when chi did not
// match one. It never falls back to the raw path.
func NormalizeRoutePattern(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || pattern == "/*" {
		return RouteUnresolved
	}
	return pattern
}
