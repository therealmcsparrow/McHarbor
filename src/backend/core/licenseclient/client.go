// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package licenseclient talks to the McHarbor licence server at
// https://www.mcharbor.eu.
//
// Three calls, no API key: the licence key the operator pasted is the
// credential, and it travels in the body. `activate` claims a seat,
// `check` both validates the key and beats the heartbeat that keeps the
// seat alive, `deactivate` gives it back.
//
// Two properties of the protocol shape everything here:
//
//   - A lapsed licence is a *successful* answer. Expired, suspended,
//     revoked, pending approval and not-yet-started all come back 200
//     with `valid: false` and a `reason`. Only a malformed request, an
//     unknown key or a refused activation carries a 4xx, so callers read
//     Response.Valid, never the status code.
//   - An unreachable server says nothing about the licence. Every
//     transport failure and 5xx collapses onto ErrUnavailable, and the
//     caller is expected to keep running on its cached entitlement. There
//     is no signed offline token yet; failing closed here would take an
//     operator's infrastructure down over our own bad afternoon.
package licenseclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Errors the caller distinguishes. Anything else is wrapped and logged
// server-side; none of them ever reaches an end user verbatim.
var (
	// ErrNotConfigured means there is no licence-server URL. Not a
	// failure — it is the state an install sits in when an operator has
	// deliberately pointed licensing at nothing.
	ErrNotConfigured = errors.New("licence server not configured")
	// ErrNoKey is a call attempted with no licence key. The state a fresh
	// install sits in until somebody pastes one.
	ErrNoKey = errors.New("no licence key")
	// ErrUnknownKey is a 404: the licence server has never heard of this
	// key. Treat it as a typo, not as a revocation.
	ErrUnknownKey = errors.New("licence key unknown to the licence server")
	// ErrInstanceNotFound is a 404 on deactivate for an instance that was
	// never registered. Nothing to undo.
	ErrInstanceNotFound = errors.New("instance not registered")
	// ErrSeatLimit is a 409: every seat on the licence is in use.
	ErrSeatLimit = errors.New("licence seat limit reached")
	// ErrRefused is a 403: the licence is real but cannot be activated
	// right now. The reason on the APIError says why.
	ErrRefused = errors.New("activation refused")
	// ErrInvalidRequest is a 422. A client bug — log it, do not put it in
	// front of the operator.
	ErrInvalidRequest = errors.New("licence request malformed")
	// ErrRateLimited is a 429. A daily check-in should never see one.
	ErrRateLimited = errors.New("licence server rate limit reached")
	// ErrUnavailable is any transport failure or 5xx. It says nothing
	// about the licence.
	ErrUnavailable = errors.New("licence server unavailable")
)

// requestTimeout matches the self-update checker: long enough for a slow
// link, short enough that a hung licence server never holds a goroutine.
const requestTimeout = 10 * time.Second

// Instance types. Both draw on the same seat pool — an agent server is a
// second machine running McHarbor code, and the licence counts it.
const (
	InstanceServer = "server"
	InstanceAgent  = "agent"
)

// Reasons the licence server returns for a licence that is not valid.
// Every one of them is a 200 on `check` and a 403 on `activate`.
const (
	ReasonPendingApproval = "pending_approval"
	ReasonExpired         = "expired"
	ReasonSuspended       = "suspended"
	ReasonRevoked         = "revoked"
	ReasonNotStarted      = "not_started"
	ReasonSeatLimit       = "seat_limit_reached"
	ReasonInstanceMissing = "instance_not_found"
)

// ActivateRequest claims a seat on a licence for this installation.
type ActivateRequest struct {
	Key          string         `json:"key"`
	InstanceID   string         `json:"instance_id"`
	InstanceType string         `json:"instance_type"`
	Hostname     string         `json:"hostname,omitempty"`
	Version      string         `json:"version,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	// MachineID collapses a server and an agent on one host onto a single
	// seat. Optional and additive: a licence server that does not know the
	// field ignores it, and a host that cannot fingerprint itself omits it
	// and is counted exactly as it is today. See machine.go.
	MachineID string `json:"machine_id,omitempty"`
}

// CheckRequest validates the key and, when InstanceID is set, doubles as
// the heartbeat that keeps this seat alive. Sending it without an
// instance id asks for a bare verdict on the key, which is what a
// "validate as the operator types" path wants.
type CheckRequest struct {
	Key        string `json:"key"`
	InstanceID string `json:"instance_id,omitempty"`
	Version    string `json:"version,omitempty"`
	// MachineID collapses a server and an agent on one host onto a single
	// seat. Optional and additive: a licence server that does not know the
	// field ignores it, and a host that cannot fingerprint itself omits it
	// and is counted exactly as it is today. See machine.go.
	MachineID string `json:"machine_id,omitempty"`
}

// DeactivateRequest hands a seat back.
type DeactivateRequest struct {
	Key          string `json:"key"`
	InstanceID   string `json:"instance_id"`
	InstanceType string `json:"instance_type"`
	// MachineID collapses a server and an agent on one host onto a single
	// seat. Optional and additive: a licence server that does not know the
	// field ignores it, and a host that cannot fingerprint itself omits it
	// and is counted exactly as it is today. See machine.go.
	MachineID string `json:"machine_id,omitempty"`
}

// Seats is the seat allowance on a licence. Unlimited outranks Total —
// the free and non-commercial licences are unmetered, and their counts
// are informational.
type Seats struct {
	Unlimited bool `json:"unlimited"`
	Total     int  `json:"total"`
	Used      int  `json:"used"`
}

// Term is the licence's validity window. Perpetual outranks ExpiresAt.
type Term struct {
	Perpetual     bool   `json:"perpetual"`
	StartsAt      string `json:"starts_at"`
	ExpiresAt     string `json:"expires_at"`
	DaysRemaining int    `json:"days_remaining"`
	AutoRenew     bool   `json:"auto_renew"`
}

// LicenseInfo is the entitlement as the licence server describes it. Key
// arrives masked; the unmasked one never leaves the operator's hands.
type LicenseInfo struct {
	ID     string `json:"id"`
	Key    string `json:"key"`
	Type   string `json:"type"`
	Status string `json:"status"`
	Seats  Seats  `json:"seats"`
	Term   Term   `json:"term"`
}

// InstanceInfo is this seat, as the licence server sees it.
type InstanceInfo struct {
	InstanceID    string `json:"instance_id"`
	InstanceType  string `json:"instance_type"`
	Hostname      string `json:"hostname"`
	HoldsSeat     bool   `json:"holds_seat"`
	LastSeenAt    string `json:"last_seen_at"`
	DeactivatedAt string `json:"deactivated_at"`
}

// Response is what activate and check return.
//
// Valid is the field that decides, not the HTTP status: a licence that
// lapsed is a well-formed answer to a well-formed question.
//
// InstanceRegistered false alongside Valid true is the signal to
// re-activate — an operator released the seat, or this host went quiet
// for 30 days and lost it.
type Response struct {
	Valid              bool          `json:"valid"`
	Reason             string        `json:"reason"`
	Message            string        `json:"message"`
	License            *LicenseInfo  `json:"license"`
	Instance           *InstanceInfo `json:"instance"`
	InstanceRegistered bool          `json:"instance_registered"`
	ActivatedAt        string        `json:"activated_at"`
	CheckedAt          string        `json:"checked_at"`
	Deactivated        bool          `json:"deactivated"`
}

// APIError is a 4xx the licence server explained. It wraps one of the
// package sentinels so callers can either switch on the sentinel or read
// the reason for a more precise message.
type APIError struct {
	Status   int
	Reason   string
	Message  string
	Seats    *Seats
	sentinel error
}

func (e *APIError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("licence server: %s (status %d)", e.Reason, e.Status)
	}
	return fmt.Sprintf("licence server: status %d", e.Status)
}

// Unwrap exposes the sentinel so errors.Is works on the shorthand.
func (e *APIError) Unwrap() error { return e.sentinel }

// errorBody is the shape a 4xx carries. `context` is open-ended; only the
// seat block is read here, because it is the one a client repeats back to
// the operator ("this licence covers 5 servers and all 5 are in use").
type errorBody struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
	Error   string `json:"error"`
	Context struct {
		Seats *Seats `json:"seats"`
	} `json:"context"`
}

// Client is a licence-server client. Construct one per call site with
// New; it holds no state beyond its configuration.
type Client struct {
	baseURL string
	http    *http.Client
}

// New builds a client. An empty base URL yields a client whose every call
// returns ErrNotConfigured.
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		http:    &http.Client{Timeout: requestTimeout},
	}
}

// Configured reports whether this client has somewhere to talk to. The
// licence key is not part of this: a client with a URL and no key is a
// fresh install waiting for one, which is a different state entirely.
func (c *Client) Configured() bool {
	return c.baseURL != ""
}

// Activate claims a seat. Repeating it for the same instance id refreshes
// the details and consumes no second seat.
func (c *Client) Activate(ctx context.Context, req ActivateRequest) (*Response, error) {
	if strings.TrimSpace(req.Key) == "" {
		return nil, ErrNoKey
	}
	var out Response
	if err := c.do(ctx, "/api/v1/license/activate", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Check validates the key and beats the heartbeat for the seat.
func (c *Client) Check(ctx context.Context, req CheckRequest) (*Response, error) {
	if strings.TrimSpace(req.Key) == "" {
		return nil, ErrNoKey
	}
	var out Response
	if err := c.do(ctx, "/api/v1/license/check", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Deactivate gives the seat back. Courteous rather than mandatory — an
// unreachable host frees its own seat after 30 quiet days.
func (c *Client) Deactivate(ctx context.Context, req DeactivateRequest) error {
	if strings.TrimSpace(req.Key) == "" {
		return ErrNoKey
	}
	return c.do(ctx, "/api/v1/license/deactivate", req, nil)
}

// do posts a JSON body and decodes a JSON reply. `out` may be nil for a
// call whose body the caller does not need.
func (c *Client) do(ctx context.Context, path string, body, out any) error {
	if !c.Configured() {
		return ErrNotConfigured
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encoding licence request: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("building licence request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Debug("licenseclient: closing response body failed", "error", err)
		}
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return statusError(resp)
	}

	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding licence response: %w", err)
	}
	return nil
}

// statusError turns a non-2xx into an APIError wrapping the matching
// sentinel, reading the reason out of the body where the server sent one.
//
// The body is read with a cap: a licence server having a bad day can
// answer a JSON request with an unbounded HTML error page, and none of it
// is worth buffering.
func statusError(resp *http.Response) error {
	if resp.StatusCode >= 500 {
		return fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	}

	var parsed errorBody
	if raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10)); err == nil {
		if json.Unmarshal(raw, &parsed) != nil {
			parsed = errorBody{}
		}
	}

	apiErr := &APIError{
		Status:  resp.StatusCode,
		Reason:  parsed.Reason,
		Message: parsed.Message,
		Seats:   parsed.Context.Seats,
	}
	if apiErr.Message == "" {
		apiErr.Message = parsed.Error
	}

	switch {
	case resp.StatusCode == http.StatusNotFound && parsed.Reason == ReasonInstanceMissing:
		apiErr.sentinel = ErrInstanceNotFound
	case resp.StatusCode == http.StatusNotFound:
		apiErr.sentinel = ErrUnknownKey
	case resp.StatusCode == http.StatusConflict:
		apiErr.sentinel = ErrSeatLimit
	case resp.StatusCode == http.StatusForbidden:
		apiErr.sentinel = ErrRefused
	case resp.StatusCode == http.StatusUnprocessableEntity:
		apiErr.sentinel = ErrInvalidRequest
	case resp.StatusCode == http.StatusTooManyRequests:
		apiErr.sentinel = ErrRateLimited
	default:
		apiErr.sentinel = ErrInvalidRequest
	}
	return apiErr
}

// Reason digs the licence server's machine-readable reason out of an
// error, returning "" when there is not one. It saves every call site
// writing the same type assertion.
func Reason(err error) string {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Reason
	}
	return ""
}
