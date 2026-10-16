// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the comm-ntfy plugin. It exports
// a single symbol — `Plugin` — that the host loads via
// `plugin.Open` and dispatches to.
//
// Scope: this plugin owns a single ChannelType ("ntfy"). The
// comm-ntfyClient implements plugin.NotificationAdapterClient
// (Test + Send only) and POSTs messages to <server_url>/<topic>
// with optional Bearer or Basic auth.
//
// Why notification (not storage): ntfy channels deliver outgoing
// alerts, workflow events, and backup notifications — not files.
// The contract is intentionally narrow: Test to validate
// credentials + Send to deliver a Title/Message payload.
package main

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// ntfyChannelType is the ChannelType this plugin owns. Single
// value — the host's communication_channels.channel_type column
// accepts this string.
const ntfyChannelType = "ntfy"

// CommNtfyPlugin is the singleton the host dispatches to. It owns
// one ChannelType's worth of NotificationAdapterSpecs and routes
// host calls into ntfyClient.
type CommNtfyPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the parsed plugin.yaml.
func (p *CommNtfyPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes — comm-ntfy does not register HTTP routes.
// The host's /api/channels CRUD is the single admin surface and
// routes are owned by the host module.
func (p *CommNtfyPlugin) RegisterRoutes(_ plugin.Registry) error {
	return nil
}

// RegisterMenu is intentionally a no-op. ntfy channels are managed
// from the host's Settings > Notifications > Channels page, which
// lists every registered ChannelType (built-in + plugin). A
// dedicated sidemenu entry would just duplicate the surface.
func (p *CommNtfyPlugin) RegisterMenu(_ plugin.MenuRegistry) error {
	return nil
}

// RegisterWidgets — this plugin has no dashboard widgets.
func (p *CommNtfyPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error {
	return nil
}

// RegisterNodes — this plugin has no workflow nodes.
func (p *CommNtfyPlugin) RegisterNodes(_ plugin.NodeRegistry) error {
	return nil
}

// RegisterPermissions registers the adds_permissions declared in
// the plugin manifest. The host consults this on Enable + on
// every permission check; without it the keys would never be
// visible to operators.
func (p *CommNtfyPlugin) RegisterPermissions(reg plugin.AccessRegistry) error {
	// The single declared permission (channels.use_comm_ntfy)
	// is registered by the host automatically from the manifest's
	// adds_permissions block, so this is a no-op for now. If we
	// ever add ABAC rules or dynamic scopes, wire them here.
	_ = reg
	return nil
}

// RegisterAdapters registers the ntfy client for its single
// ChannelType. The cfg map the host hands to Test/Send carries
// the channel row's columns (server_url, topic, access_token,
// username, password, priority, tags) keyed by column name.
//
// SupportsRichAttachments is false in v1: ntfy only consumes the
// message body as plain text (or markdown via Click action), no
// inline image attachments via the publish API. The host delivers
// Title + Message text only and ntfy renders them as the body
// with the Title as the ntfy header.
//
// Slug + Icon + Description are populated from the plugin's own
// manifest so the host's "Add Communication" picker can render
// this plugin as a tile via /api/plugins/loader/icon.
func (p *CommNtfyPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Notification().Register(
		plugin.NotificationAdapterSpec{
			ChannelType: ntfyChannelType,
			Slug:        p.manifest.Slug,
			DisplayName: "ntfy",
			Icon:        "plugin.svg",
			Description: p.manifest.Desc,
			Capabilities: plugin.NotificationAdapterCapabilities{
				EncryptedCredentials:    true,
				SupportsRichAttachments: false,
				// ntfy accepts a file as the PUT body with a Filename header.
				SupportsFileAttachments: true,

				// A topic is a path on the same server, not a credential, so one
				// message can be published to a different topic.
				RecipientOverride: plugin.NotificationRecipientOverride{
					Key:           "topic",
					Kind:          "topic",
					LabelEN:       "Topic",
					PlaceholderEN: "reports-board",
					Multiple:      false,
				},
			},
			ConfigSchema: ntfyConfigSchema(),
		},
		NewNtfyClient(),
	)
}

// ntfyConfigSchema returns the form fields the admin UI renders
// when an operator picks this channel type.
//
// server_url is the ntfy base URL. Default to ntfy.sh when
// blank — covers the common cloud case without forcing an
// extra field. Operators on a self-hosted ntfy set this to
// https://ntfy.example.com.
//
// topic is the channel name operators subscribe to on the ntfy
// app (Android/iOS/web). Treated as `string` so operators can use
// any name — ntfy.sh allows up to 64 chars of [a-zA-Z0-9_-].
//
// access_token is the topic-scoped token printed by ntfy when an
// operator clicks "Get token" on the topic page. Treated as
// `secret` so the host encrypts it at rest via AES-256-GCM.
//
// username + password are HTTP Basic auth credentials. Used
// instead of access_token on self-hosted ntfy servers that gate
// the publish endpoint behind per-user auth. Both fields are
// `secret`. Operators set access_token OR (username + password),
// never both.
//
// priority is one of ntfy's named levels: min, low, default,
// high, urgent. The plugin maps these to integer priorities
// 1/2/3/4/5 when setting the Priority header. Empty string
// keeps ntfy's server default (3 / "default").
//
// tags is a comma-separated list of emoji shortcodes (e.g.
// "warning,rocket") that ntfy renders as inline icons in the
// mobile app. Optional; ignored when blank.
func ntfyConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "server_url",
			Type:        "string",
			Label:       "Server URL",
			LabelKey:    "fields.serverUrl",
			Required:    true,
			Default:     "https://ntfy.sh",
			Help:        "Base URL of your ntfy server. Defaults to https://ntfy.sh — change to your self-hosted instance (e.g. https://ntfy.example.com) for on-prem setups.",
			HelpKey:     "config.server_url.help",
			Placeholder: "https://ntfy.sh",
		},
		{
			Key:         "topic",
			Type:        "string",
			Label:       "Topic",
			LabelKey:    "fields.topic",
			Required:    true,
			Help:        "Topic name subscribers receive on the ntfy app. Use a unique name (e.g. mcharbor-alerts-prod). ntfy.sh allows up to 64 chars of [a-zA-Z0-9_-].",
			HelpKey:     "config.topic.help",
			Placeholder: "mcharbor-alerts-prod",
		},
		{
			Key:         "access_token",
			Type:        "secret",
			Label:       "Access Token",
			LabelKey:    "fields.accessToken",
			Required:    false,
			Help:        "Topic-scoped token from ntfy. Click 'Get token' on the topic page in the ntfy web UI. Takes precedence over username/password when set. Encrypted at rest.",
			HelpKey:     "config.access_token.help",
			Placeholder: "tk_xxxxxxxxxxxxxxxxxxxx",
		},
		{
			Key:         "username",
			Type:        "string",
			Label:       "Username",
			LabelKey:    "fields.username",
			Required:    false,
			Help:        "HTTP Basic auth username for self-hosted ntfy. Used only when access_token is blank.",
			HelpKey:     "help.username",
			Placeholder: "ntfy-user",
		},
		{
			Key:         "password",
			Type:        "secret",
			Label:       "Password",
			LabelKey:    "fields.password",
			Required:    false,
			Help:        "HTTP Basic auth password for self-hosted ntfy. Used only when access_token is blank. Encrypted at rest.",
			HelpKey:     "help.password",
			Placeholder: "••••••••",
		},
		{
			Key:      "priority",
			Type:     "string",
			Label:    "Priority",
			LabelKey: "fields.priority",
			Required: false,
			Default:  "",
			Help:     "Optional. One of min, low, default, high, urgent. Maps to ntfy's integer priority 1/2/3/4/5. Leave blank for server default.",
			HelpKey:  "config.priority.help",
		},
		{
			Key:      "tags",
			Type:     "string",
			Label:    "Tags",
			LabelKey: "fields.tags",
			Required: false,
			Default:  "",
			Help:     "Optional. Comma-separated emoji shortcodes rendered as inline icons in the ntfy mobile app (e.g. 'warning,rocket'). See https://docs.ntfy.sh/emojis/ for the catalog.",
			HelpKey:  "config.tags.help",
		},
	}
}

// Init is called after RegisterAdapters. We stash the host
// context so the ntfy client can audit on Test + Send.
func (p *CommNtfyPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.comm-ntfy.init", map[string]any{"slug": p.manifest.Slug})
	}
	return nil
}

// Shutdown is called on graceful stop or uninstall.
func (p *CommNtfyPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.comm-ntfy.shutdown", nil)
	}
	return nil
}

// Compile-time check that we satisfy the plugin.Plugin interface.
var _ plugin.Plugin = (*CommNtfyPlugin)(nil)

// Plugin is the exported symbol `plugin.Open` looks up. It must
// be a variable (not a const) for `-buildmode=plugin` to find it.
var Plugin plugin.Plugin = &CommNtfyPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "comm-ntfy",
		Name:    "ntfy",
		Version: "2026.10.01",
	},
}
