// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package logging defines the logging-family adapter spec types
// (sub-registry, Spec struct, Client interface, capabilities, and the
// delivery shape exchanged with the host).
//
// Why split: see the package doc in `core/plugin/contract`. Each
// family lives in its own sub-package so the host's reach set of
// `specs/logging` matches the plugin's reach set exactly.
//
// Scope: a logging adapter forwards *audit events* off-box to an
// external log sink — an HTTP SIEM collector (Splunk HEC, Microsoft
// Sentinel, Elastic) or an RFC5424 syslog daemon (rsyslog, syslog-ng,
// QRadar, ArcSight). This is deliberately narrower than the
// notification family: notification adapters deliver human-readable
// Title/Message pairs a person reads, logging adapters deliver
// machine-readable structured records a log pipeline ingests. The two
// never share a client — a Slack channel is not a log sink, and a
// syslog daemon is not a place to send "backup finished".
//
// # Why the adapter polls instead of the host pushing
//
// The obvious contract for this family would be
// `Forward(ctx, events)` — the host reads pending audit rows and
// hands them over. It does not work, because the retry accounting
// that decides *which* rows are pending lives in the adapter's own
// delivery-history table: a row is pending when it has no successful
// delivery AND fewer than max_retries attempts, both of which are
// facts only the adapter's table knows. A host-side "pending" query
// would have to reach into per-plugin tables it must not know about.
//
// So the adapter owns the cycle: it queries the host's `audit_logs`
// (read-only, via host.DB) left-joined against its own
// `plugin_<slug>_delivery_history`, delivers what it finds, and
// records the attempts. The host contributes the two things a plugin
// genuinely cannot do for itself — a ticker, and HA leader election
// so a three-node cluster forwards each event once rather than three
// times. See ForwardPending.
//
// Imported by:
//   - host modules that drive the forwarding loop (audit)
//   - logging plugins (logging-siem, logging-syslog)
package logging

import (
	"context"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/types"
)

// LogForwarderAdapterRegistry is the sub-registry for outbound audit
// log forwarders. A plugin registers one entry per ForwarderType it
// handles. Registering the same ForwarderType twice from the same
// plugin (or from two different plugins) is an error — the host uses
// the ForwarderType as the lookup key.
//
// Unlike the notification family, there is no built-in fallback: a
// host with no logging plugin installed simply does not forward audit
// events off-box. Audit events are still written to `audit_logs` and
// remain exportable from the Compliance page, so removing every
// logging plugin degrades forwarding, never recording.
type LogForwarderAdapterRegistry interface {
	// Register adds an adapter. The host's forwarding worker drives
	// every registered adapter on its ticker. The client must be safe
	// for concurrent use — Test can be invoked from the admin UI
	// while ForwardPending is mid-cycle.
	Register(spec LogForwarderAdapterSpec, client LogForwarderAdapterClient) error
}

// LogForwarderAdapterSpec describes one log sink type a plugin
// handles. The host uses the spec to render the Settings >
// Integrations > Logging tab (tile, validation, capability hints) and
// to route Test sends at runtime.
type LogForwarderAdapterSpec struct {
	// ForwarderType is the unique key the host uses to dispatch and
	// to name the adapter's config row. Plugins should use
	// lowercase, kebab-friendly identifiers ("siem", "syslog").
	ForwarderType string

	// Slug is the owning plugin's slug (e.g. "logging-syslog"). The
	// host surfaces it alongside the spec so the frontend can resolve
	// icon assets via /api/plugins/loader/icon without a second
	// round-trip. Plugins populate this in RegisterAdapters from
	// their own manifest.Slug.
	Slug string

	// DisplayName is the user-facing label shown on the adapter tile
	// in the Logging tab.
	DisplayName string

	// Icon is the asset filename inside the plugin's icons/ directory
	// (resolved by the host's /api/plugins/loader/icon route at
	// render time). Empty string is valid — renderers fall back to
	// IconPuzzle.
	Icon string

	// Description is the operator-facing short description shown
	// under the tile label. Empty string is valid.
	Description string

	// Capabilities describes what the adapter supports. The host uses
	// these to hide unsupported actions in the admin UI and to
	// short-circuit unsupported calls.
	Capabilities LogForwarderAdapterCapabilities

	// ConfigSchema is the list of form fields the Logging tab renders
	// when an operator configures this forwarder. Unlike the
	// notification family — which maps onto fixed
	// communication_channels columns — logging adapters own their
	// storage entirely: the plugin's own migration creates
	// `plugin_<slug>_config`, and the host never interprets these
	// keys. Fields typed "secret" are encrypted at rest by the host's
	// EncryptionHandle and redacted in API responses.
	ConfigSchema []types.ConfigFieldDecl
}

// LogForwarderAdapterCapabilities declares what the adapter supports.
// The host consults these before invoking the corresponding method on
// LogForwarderAdapterClient.
type LogForwarderAdapterCapabilities struct {
	// EncryptedCredentials indicates the adapter accepts credential
	// fields the host should encrypt at rest via the EncryptionHandle.
	// Every "secret"-typed ConfigFieldDecl is stored encrypted
	// regardless; this flag tells the UI to show the "stored
	// encrypted" affordance on the form.
	EncryptedCredentials bool

	// SupportsDeliveryHistory indicates the adapter records
	// per-attempt delivery rows the host can surface in the Logging
	// tab's history panel. Adapters that declare false render no
	// history section — the host does not synthesise one, because a
	// fabricated "success" row on a fire-and-forget UDP send would be
	// a compliance lie.
	SupportsDeliveryHistory bool

	// RequiresLeaderElection indicates the adapter must run on
	// exactly one node in an HA cluster. True for every sink that
	// would otherwise receive N copies of each event from an N-node
	// cluster — which is all of them today. Declared rather than
	// assumed so a future adapter with a genuinely idempotent,
	// dedup-on-ingest sink can opt out and forward from every node.
	RequiresLeaderElection bool
}

// ForwardResult reports what one ForwardPending cycle did. The host's
// worker logs it and the Logging tab surfaces the counts, so an
// operator can tell "nothing to send" apart from "everything failed"
// — two states that otherwise look identical from the outside.
type ForwardResult struct {
	// Delivered is the number of audit events accepted by the sink
	// this cycle.
	Delivered int

	// Failed is the number of delivery attempts that errored this
	// cycle. Events that exhausted max_retries are counted once, on
	// the attempt that exhausted them, and then never retried.
	Failed int

	// Skipped is the number of candidate events the adapter passed
	// over — already delivered, or past their retry ceiling.
	Skipped int
}

// LogDelivery is one recorded forwarding attempt, returned by
// DeliveryHistory. The host renders these in the Logging tab so an
// operator can see whether the sink is actually receiving events —
// the single most common support question for this family.
type LogDelivery struct {
	// ID is the delivery row's own id.
	ID string

	// EventID is the `audit_logs.id` this attempt was for. Empty for
	// a Test send, which has no originating audit row.
	EventID string

	// Status is "success" or "failed". Adapters must not invent a
	// third value — the host's badge rendering switches on exactly
	// these two.
	Status string

	// Attempt is the 1-based retry counter.
	Attempt int

	// StatusCode is the sink's protocol-level response code when one
	// exists (HTTP status for collectors). Zero for protocols that
	// have none, such as UDP syslog.
	StatusCode int

	// Protocol is the transport actually used ("https", "tls", "udp",
	// "tcp"). Shown in the history row so an operator can see at a
	// glance that a "success" went out over plaintext UDP.
	Protocol string

	// Error is the failure reason, already safe to show an operator:
	// adapters MUST NOT put raw credentials, endpoint URLs with
	// embedded tokens, or unwrapped transport errors here. Empty on
	// success.
	Error string

	// CreatedAt is when the attempt was made.
	CreatedAt time.Time
}

// LogForwarderAdapterClient is the runtime contract for
// plugin-provided audit log forwarders. The host calls these when:
//   - the Logging tab's "Send test" button is clicked (Test),
//   - the forwarding worker ticks (ForwardPending), and
//   - the Logging tab's history panel is rendered (DeliveryHistory).
//
// All methods receive a config map carrying the adapter's stored
// config keyed by ConfigSchema key, with secret fields already
// decrypted by the host. Plugins decide which keys they need; the
// host does not pre-validate the map.
//
// Implementations MUST be safe for concurrent use.
type LogForwarderAdapterClient interface {
	// Test delivers a synthetic event to the configured sink and
	// verifies it was accepted. The host calls this from the Logging
	// tab's "Send test" button.
	//
	// Test must actually hit the wire. A Test that only validates the
	// config shape tells the operator nothing about whether their
	// firewall, TLS chain, or collector token works — which is the
	// entire reason the button exists.
	Test(ctx context.Context, config map[string]string) error

	// ForwardPending runs one forwarding cycle: find audit events this
	// adapter has not yet delivered, deliver them, record the
	// attempts. The host's worker calls this on a ticker, and — for
	// adapters declaring RequiresLeaderElection — only on the node
	// currently holding the forwarder lease.
	//
	// The adapter is responsible for its own watermark, dedup, and
	// retry ceiling; the host passes no cursor because it has none to
	// pass. See the package doc for why.
	//
	// ForwardPending MUST honour ctx, which the host gives a deadline:
	// a cycle that outruns the ticker interval blocks the next one.
	// Returning an error does not disable the adapter — the host logs
	// it and ticks again.
	ForwardPending(ctx context.Context, config map[string]string) (ForwardResult, error)

	// DeliveryHistory returns the most recent attempts, newest first,
	// capped at limit. Adapters that declared
	// SupportsDeliveryHistory=false may return (nil, nil) — the host
	// never calls this on them.
	DeliveryHistory(ctx context.Context, config map[string]string, limit int) ([]LogDelivery, error)
}
