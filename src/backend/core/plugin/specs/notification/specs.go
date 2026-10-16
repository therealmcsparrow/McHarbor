// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package notification defines the notification-family adapter spec
// types (sub-registry, Spec struct, Client interface, capabilities,
// and the message / attachment shapes exchanged with the host).
//
// Why split: see the package doc in `core/plugin/contract`. Each
// family lives in its own sub-package so the host's reach set of
// `specs/notification` matches the plugin's reach set exactly.
//
// Imported by:
//   - host modules that dispatch notifications
//     (communications, alert_engine)
//   - notification plugins (comm-ntfy, comm-telegram, comm-signal,
//     comm-teams, comm-whatsapp)
package notification

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin/types"
)

// NotificationAdapterRegistry is the sub-registry for outgoing
// communication channel handlers (Slack, Discord, Teams, Gotify,
// ntfy, Telegram, Signal, WhatsApp, etc.). A plugin registers one
// entry per channel type it handles. Registering the same
// ChannelType twice from the same plugin (or from two different
// plugins) is an error — the host uses the ChannelType as the
// lookup key.
//
// "internal" (in-app / in-app broadcast) is intentionally NOT
// exposed via this registry: the host's notify.Dispatcher owns the
// built-in path so operators without any plugin installed still
// get broadcast-style notifications out of the box.
type NotificationAdapterRegistry interface {
	// Register adds an adapter. The host dispatches channels with
	// channel_type == spec.ChannelType to client. The client must be
	// safe for concurrent use — the host invokes it from multiple
	// goroutines when a workflow fans a single event out to several
	// channels.
	Register(spec NotificationAdapterSpec, client NotificationAdapterClient) error
}

// NotificationAdapterSpec describes one outbound channel type a
// plugin handles. The host uses the spec to render the admin UI
// (validation, capability hints) and to route Test sends and live
// notifications at runtime.
type NotificationAdapterSpec struct {
	// ChannelType is the unique key the host uses to dispatch
	// channels. Must match a value the host's communication_channels
	// table accepts (and match the values documented in the host's
	// i18n keys). Plugins should use lowercase, kebab-friendly
	// identifiers.
	ChannelType string

	// DisplayName is the user-facing label shown in the channel
	// form's channel_type dropdown and in capabilities listings.
	DisplayName string

	// Slug is the owning plugin's slug (e.g. "comm-telegram").
	// The host surfaces it alongside the spec so the frontend can
	// resolve icon assets via /api/plugins/loader/icon without a
	// second round-trip. Plugins populate this in RegisterAdapters
	// from their own manifest.Slug.
	Slug string

	// Icon is the asset filename inside the plugin's icons/
	// directory (resolved by the host's /api/plugins/loader/icon
	// route at render time). Empty string is valid — renderers
	// fall back to IconPuzzle. The host trusts the value because
	// it is declared in the plugin's own plugin.yaml — an attacker
	// who can edit a plugin's manifest already controls the .so
	// bundle.
	Icon string

	// Description is the operator-facing short description shown
	// under the tile label in the channel picker. Empty string is
	// valid — the tile renders without a subtitle.
	Description string

	// Capabilities describes what the adapter supports. The host
	// uses these to hide unsupported actions in the admin UI and to
	// short-circuit unsupported calls with ErrUnsupported.
	Capabilities NotificationAdapterCapabilities

	// ConfigSchema is the optional list of custom form fields the
	// admin UI renders when an operator picks this channel type. The
	// host already knows about the per-type columns on
	// communication_channels (webhook_url, token, server_url, …);
	// plugins extend the schema by adding custom keys that the host
	// stores in the same row's columns when they line up, or in a
	// plugin-owned column for anything truly plugin-specific. This is
	// intentionally simple for v1 — a richer JSON-typed config column
	// is a follow-up.
	ConfigSchema []types.ConfigFieldDecl
}

// NotificationAdapterCapabilities declares what the adapter supports.
// The host consults these before invoking the corresponding method
// on NotificationAdapterClient.
type NotificationAdapterCapabilities struct {
	// EncryptedCredentials indicates the adapter accepts credential
	// fields that the host should encrypt at rest via the
	// EncryptionHandle. Adapters that need only an opaque payload
	// (e.g., a single webhook URL) leave this true as well — the
	// host stores every "secret"-typed ConfigFieldDecl encrypted.
	EncryptedCredentials bool

	// SupportsRichAttachments indicates the adapter honors the
	// Title / Message / Attachment structure of a NotificationMessage
	// (Slack supports Block Kit, Teams supports Adaptive Cards,
	// Telegram supports inline keyboards, …). When false, the host
	// falls back to plain Title + Message text-only delivery.
	SupportsRichAttachments bool

	// SupportsFileAttachments indicates the provider behind this
	// adapter can transmit a file — a PDF report, an export, a log
	// bundle. Declare it only when the provider genuinely can: the
	// host stops substituting a download link once an adapter claims
	// the capability, so a wrong true means the recipient gets
	// nothing at all, while a false costs only the convenience of an
	// inline file.
	//
	// Adapters that declare false are never handed
	// NotificationMessage.Files — the host strips it before Send —
	// so a false adapter needs no file-handling code whatsoever.
	SupportsFileAttachments bool

	// RecipientOverride declares the one config key a caller may
	// retarget for a single send, when the provider behind this
	// adapter has a destination that is a plain field rather than a
	// credential.
	//
	// An SMTP channel addressed to the ops list can carry one report
	// to a board member; a Telegram bot can post one message to a
	// different chat. A Slack or Discord webhook cannot: the URL *is*
	// the destination and *is* the secret, so those adapters leave
	// this zero and every caller correctly offers no field.
	//
	// Key must name a field the adapter already reads out of its
	// config map (`to`, `chat_id`, `topic`). The host writes the
	// caller's value into exactly that key and nothing else, which is
	// why callers are never handed the config map itself - a plugin
	// that could set arbitrary keys could set `password`.
	RecipientOverride NotificationRecipientOverride
}

// NotificationRecipientOverride describes an adapter's overridable
// destination field, both to permit the override and to let a UI ask
// for it in the provider's own vocabulary.
//
// A zero value (empty Key) means the adapter has no such field. That
// is the safe default: an adapter says nothing and no caller can
// retarget it.
type NotificationRecipientOverride struct {
	// Key is the config field the override replaces. Empty disables
	// the whole mechanism for this adapter.
	Key string

	// Kind tells a UI what it is asking for, so it can validate and
	// label the input without a per-channel-type table of its own.
	// One of "email", "phone", "chat_id", "topic". An unrecognised
	// value is treated as free text.
	Kind string

	// LabelEN and PlaceholderEN are English source strings for a UI
	// that has no bundle of its own for this adapter - the same
	// convention ConfigFieldDecl.Label already follows. A host or
	// plugin with its own i18n keys should prefer those.
	LabelEN       string
	PlaceholderEN string

	// Multiple reports whether the field accepts a comma-separated
	// list. True for a mail `to`, false for a Telegram chat id.
	Multiple bool
}

// NotificationMessage is the payload the host hands to a plugin when
// a workflow, alert, or backup-completion event fans out. Plugins
// implement NotificationAdapterClient.Send.
type NotificationMessage struct {
	// Title is a one-line subject. Plugins typically render this
	// bolded above the message body. Required; never empty when the
	// host invokes Send.
	Title string

	// Message is the body of the notification. May contain
	// line breaks; the host does not strip them.
	Message string

	// Level is one of "info", "warning", "error". Plugins MAY use
	// this to color the message; the host does not require it.
	Level string

	// Attachments are optional structured payloads the plugin MAY
	// render. Plugins that declared SupportsRichAttachments=false
	// must ignore this field. The host does NOT pre-format these —
	// the plugin owns its provider's data shape.
	Attachments []NotificationAttachment

	// Files are real files to transmit — a generated report, an
	// export, a log bundle. Nil for the text-only sends that make up
	// most traffic, so existing callers are unaffected.
	//
	// The host only populates this for adapters that declared
	// SupportsFileAttachments; it is stripped before Send for every
	// other adapter. A plugin therefore never has to check the
	// capability it itself declared — if Files is non-empty, this
	// adapter said it could carry them.
	Files []NotificationFile
}

// NotificationFile is one file to transmit through a channel.
//
// Data is held in memory rather than streamed: a notification
// attachment is a report or an export, not a disk image, and every
// provider API behind these adapters wants either a multipart body or
// a base64 blob it can size up front. Callers are expected to keep
// files to a few megabytes.
type NotificationFile struct {
	// Filename is what the recipient sees, including extension —
	// "capacity-2026-08.pdf". Required; adapters may sanitise it for
	// their provider but must not drop the extension.
	Filename string

	// ContentType is the MIME type, e.g. "application/pdf". Adapters
	// that must declare a type to their provider use this; the rest
	// may ignore it.
	ContentType string

	// Data is the file's bytes.
	Data []byte
}

// NotificationAttachment is a single structured attachment a plugin
// can render alongside the message body. The host does not interpret
// the Payload — plugins own their provider's data shape.
type NotificationAttachment struct {
	// Type is a free-form, provider-specific label ("block",
	// "card", "inline_keyboard", …). Plugins decide what to render
	// based on the type.
	Type string

	// Payload is the provider-specific structured payload encoded
	// as a JSON-compatible map (a Go-side this is map[string]any; the
	// SDK codifies the shape as opaque to the host).
	Payload map[string]any
}

// NotificationAdapterClient is the runtime contract for
// plugin-provided outbound notification handlers. The host calls
// these methods when:
//   - the admin UI's "Send test" button is clicked (Test), and
//   - a workflow, alert, or backup event fans out (Send).
//
// All methods receive a config map carrying the channel's row
// columns (channel_type, webhook_url, token, server_url, topic, …)
// keyed by column name. Plugins decide which keys they need; the
// host does not pre-validate the map.
//
// Implementations MUST be safe for concurrent use. The host invokes
// Test and Send from parallel goroutines during fan-out.
type NotificationAdapterClient interface {
	// Test verifies the channel is reachable and the credentials
	// work. The host calls this from the admin UI's "Test connection"
	// button. Implementations should validate every config field
	// they will use at runtime and return a descriptive error on
	// failure.
	Test(ctx context.Context, config map[string]string) error

	// Send delivers msg to the configured channel. The host calls
	// Send from workflows, alerts, and any other fan-out point.
	// Implementations should treat Send as fire-and-forget from the
	// host's perspective: a non-nil error is logged and surfaced to
	// the user, but the host does not retry.
	Send(ctx context.Context, config map[string]string, msg NotificationMessage) error
}
