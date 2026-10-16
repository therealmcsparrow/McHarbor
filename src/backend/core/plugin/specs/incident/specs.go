// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package incident is the optional two-way contract an ITSM comm plugin implements beside its
// NotificationAdapterClient; it is a new package so no already-built plugin changes its ABI.
package incident

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// Severity is McHarbor's own three-level scale; each plugin maps it to its vendor's
// impact/urgency/priority in one table so the mapping is reviewable in one place.
type Severity string

// SeverityCritical, SeverityWarning and SeverityInfo mirror alerts.Severity values verbatim.
const (
	SeverityCritical Severity = "critical"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
)

// State is the McHarbor-side lifecycle transition that produced the Event.
type State string

// StateFiring opens or re-notifies, StateResolved closes, StateUpdate appends a note only.
const (
	StateFiring   State = "firing"
	StateResolved State = "resolved"
	StateUpdate   State = "update"
)

// RemoteState is the vendor ticket status normalised to five values the host can act on.
type RemoteState string

// RemoteOpen through RemoteCancelled are the only values a plugin may report back.
const (
	RemoteOpen         RemoteState = "open"
	RemoteAcknowledged RemoteState = "acknowledged"
	RemoteResolved     RemoteState = "resolved"
	RemoteClosed       RemoteState = "closed"
	RemoteCancelled    RemoteState = "cancelled"
)

// Terminal reports whether the remote ticket no longer accepts work, which is when the host
// stops polling it and stops sending updates to it.
func (s RemoteState) Terminal() bool {
	return s == RemoteResolved || s == RemoteClosed || s == RemoteCancelled
}

// Event is one lifecycle transition. DedupKey is stable for the life of an alert condition
// (alertID:environmentID:subjectID) and is what vendors with native dedup receive verbatim.
type Event struct {
	DedupKey        string            `json:"dedupKey"`
	State           State             `json:"state"`
	Severity        Severity          `json:"severity"`
	Title           string            `json:"title"`
	Message         string            `json:"message"`
	Source          string            `json:"source"`
	AlertID         string            `json:"alertId"`
	AlertName       string            `json:"alertName"`
	EnvironmentID   string            `json:"environmentId"`
	EnvironmentName string            `json:"environmentName"`
	SubjectID       string            `json:"subjectId"`
	SubjectName     string            `json:"subjectName"`
	Labels          map[string]string `json:"labels"`
	Link            string            `json:"link"`
	OccurredAt      time.Time         `json:"occurredAt"`
}

// ErrInvalidEvent is returned by Validate so callers can tell a host bug from a vendor failure.
var ErrInvalidEvent = errors.New("incident: invalid event")

// Validate rejects events a vendor would store badly; 255 matches the smallest correlation
// field among supported vendors (ServiceNow correlation_id is 100, so plugins hash longer keys).
func (e Event) Validate() error {
	if e.DedupKey == "" || len(e.DedupKey) > 255 || e.Title == "" {
		return ErrInvalidEvent
	}
	switch e.State {
	case StateFiring, StateResolved, StateUpdate:
	default:
		return ErrInvalidEvent
	}
	switch e.Severity {
	case SeverityCritical, SeverityWarning, SeverityInfo:
	default:
		return ErrInvalidEvent
	}
	return nil
}

// Ref identifies the remote ticket; ExternalID is the vendor's immutable key (sys_id, UUID),
// Number is the human-facing one (INC0012345), URL deep-links into the vendor UI.
type Ref struct {
	ExternalID string `json:"externalId"`
	Number     string `json:"number"`
	URL        string `json:"url"`
}

// RemoteStatus is what a webhook or a poll learned about one ticket.
type RemoteStatus struct {
	Ref       Ref         `json:"ref"`
	DedupKey  string      `json:"dedupKey"`
	State     RemoteState `json:"state"`
	Assignee  string      `json:"assignee"`
	Note      string      `json:"note"`
	UpdatedAt time.Time   `json:"updatedAt"`
}

// ErrNotFound tells the host the remote ticket was deleted, so the link is orphaned rather
// than retried forever.
var ErrNotFound = errors.New("incident: remote ticket not found")

// ErrRetryable wraps transient vendor failures (429, 5xx, timeouts); anything else is
// treated as permanent and parked with last_error for the operator.
var ErrRetryable = errors.New("incident: retryable")

// ErrOperatorMessage marks an error whose text is safe to show an operator (a vendor's
// validation message with no credentials in it); unmarked errors show a generic string.
var ErrOperatorMessage = errors.New("incident: operator message")

// RetryAfterError carries a vendor's Retry-After so the outbox waits exactly that long
// instead of its own backoff; it always also wraps ErrRetryable.
type RetryAfterError struct {
	After time.Duration
}

// Error implements error.
func (e *RetryAfterError) Error() string { return "incident: retry after " + e.After.String() }

// Unwrap makes errors.Is(err, ErrRetryable) true for every RetryAfterError.
func (e *RetryAfterError) Unwrap() error { return ErrRetryable }

// LifecycleClient is asserted on the plugin's NotificationAdapterClient. Open must be
// idempotent on DedupKey: a retried Open after a lost response must not create a second ticket.
type LifecycleClient interface {
	Open(ctx context.Context, config map[string]string, ev Event) (Ref, error)
	Update(ctx context.Context, config map[string]string, ref Ref, ev Event) error
	Resolve(ctx context.Context, config map[string]string, ref Ref, ev Event) error
}

// WebhookParser is optional; plugins that implement it get the host's inbound route and must
// authenticate the request themselves because the route is public by necessity.
type WebhookParser interface {
	VerifyWebhook(config map[string]string, header http.Header, body []byte) error
	ParseWebhook(ctx context.Context, config map[string]string, header http.Header, body []byte) ([]RemoteStatus, error)
}

// Poller is optional; the host calls it for open links on channels whose admins cannot
// configure outbound webhooks in their ITSM.
type Poller interface {
	Poll(ctx context.Context, config map[string]string, refs []Ref) ([]RemoteStatus, error)
}
