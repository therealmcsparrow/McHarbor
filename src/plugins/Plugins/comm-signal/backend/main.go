// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the comm-signal plugin. It
// exports a single symbol — `Plugin` — that the host loads via
// `plugin.Open` and dispatches to.
//
// Scope: this plugin owns a single ChannelType ("signal"). The
// signalClient implements plugin.NotificationAdapterClient
// (Test + Send only). The Send surface branches on a per-channel
// `method` field so one channel can route through four different
// Signal transports:
//
//  1. rest    — Official signal-cli-rest-api server
//  2. bot     — Token-based Signal bot server
//  3. signald — signald daemon JSON-RPC API
//  4. simple  — No-auth community relay / public test endpoint
//
// Why notification (not storage): Signal channels deliver outgoing
// alerts, workflow events, and backup notifications — not files.
// The contract is intentionally narrow: Test to validate
// credentials + Send to deliver a Title/Message payload.
package main

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// signalChannelType is the ChannelType this plugin owns.
// Single value — the host's communication_channels.channel_type
// column accepts this string.
const signalChannelType = "signal"

// Method identifiers written to cfg["method"]. The host's admin
// form renders one tile per method via the UMD ConfigForm and the
// backend branches on these strings at Test/Send time. Lowercase
// + stable — DO NOT rename without a data migration.
const (
	methodRest    = "rest"    // signal-cli-rest-api
	methodBot     = "bot"     // token-based Signal bot server
	methodSignald = "signald" // signald daemon JSON-RPC API
	methodSimple  = "simple"  // no-auth community relay
)

// CommSignalPlugin is the singleton the host dispatches to.
// It owns one ChannelType's worth of NotificationAdapterSpecs and
// routes host calls into signalClient.
type CommSignalPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the parsed plugin.yaml.
func (p *CommSignalPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes — comm-signal does not register HTTP routes.
// The host's /api/channels CRUD is the single admin surface and
// routes are owned by the host module.
func (p *CommSignalPlugin) RegisterRoutes(_ plugin.Registry) error {
	return nil
}

// RegisterMenu is intentionally a no-op. Signal channels are
// managed from the host's Settings > Notifications > Channels
// page, which lists every registered ChannelType (built-in +
// plugin). A dedicated sidemenu entry would just duplicate the
// surface.
func (p *CommSignalPlugin) RegisterMenu(_ plugin.MenuRegistry) error {
	return nil
}

// RegisterWidgets — this plugin has no dashboard widgets.
func (p *CommSignalPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error {
	return nil
}

// RegisterNodes — this plugin has no workflow nodes.
func (p *CommSignalPlugin) RegisterNodes(_ plugin.NodeRegistry) error {
	return nil
}

// RegisterPermissions registers the adds_permissions declared in
// the plugin manifest. The host consults this on Enable + on
// every permission check; without it the keys would never be
// visible to operators.
func (p *CommSignalPlugin) RegisterPermissions(reg plugin.AccessRegistry) error {
	// The single declared permission (channels.use_comm_signal)
	// is registered by the host automatically from the manifest's
	// adds_permissions block, so this is a no-op for now. If we
	// ever add ABAC rules or dynamic scopes, wire them here.
	_ = reg
	return nil
}

// RegisterAdapters registers the Signal client for its single
// ChannelType. The cfg map the host hands to Test/Send carries
// the channel row's columns keyed by ConfigFieldDecl.Key.
//
// SupportsRichAttachments is false in v1: Signal gateways accept
// text-only attachments via separate upload endpoints; the host
// currently delivers Title + Message text only.
//
// Slug + Icon + Description are populated from the plugin's
// own manifest so the host's "Add Communication" picker can
// render this plugin as a tile via /api/plugins/loader/icon.
func (p *CommSignalPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Notification().Register(
		plugin.NotificationAdapterSpec{
			ChannelType: signalChannelType,
			Slug:        p.manifest.Slug,
			DisplayName: "Signal",
			Icon:        "plugin.svg",
			Description: p.manifest.Desc,
			Capabilities: plugin.NotificationAdapterCapabilities{
				EncryptedCredentials:    true,
				SupportsRichAttachments: false,
				SupportsFileAttachments: true,

				// The sender account stays the channel's; only the numbers it is
				// delivered to move.
				RecipientOverride: plugin.NotificationRecipientOverride{
					Key:           "recipients",
					Kind:          "phone",
					LabelEN:       "Recipients",
					PlaceholderEN: "+31612345678, +31687654321",
					Multiple:      true,
				},
			},
			ConfigSchema: signalConfigSchema(),
		},
		NewSignalClient(),
	)
}

// signalConfigSchema returns the form fields the admin UI
// renders when an operator picks this channel type.
//
// The schema is the UNION of fields across all four methods:
// the host's form renderer groups them under one "Method" tab
// selector and shows the relevant subset once the operator
// picks a method. Required + secret flags still apply — the
// form must validate per-method before Save.
//
// Fields summary (see signal_client.go for endpoint shapes):
//
//	method          required  string   rest|bot|signald|simple
//	server_url      required  string   base URL of the Signal gateway
//	sender_number   required  string   registered Signal account, E.164
//	recipients      required  string   comma-separated E.164 recipients
//
//	rest (signal-cli-rest-api):
//	  auth_username  optional  string   HTTP Basic auth user
//	  auth_password  optional  secret   HTTP Basic auth password
//
//	bot (token-based Signal bot server):
//	  bot_token      required  secret   Bearer token
//
//	signald (signald daemon JSON-RPC):
//	  account_id     required  string   signald account (UUID or E.164)
func signalConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		// --- discriminator + common to every method ---
		{
			Key:      "method",
			Type:     "string",
			Label:    "Connection Method",
			LabelKey: "config.method.label",
			Required: true,
			Default:  methodRest,
			Help:     "Which Signal transport to use. REST = official signal-cli-rest-api. Bot = token-based bot server. signald = signald daemon JSON-RPC. Simple = no-auth community relay.",
			HelpKey:  "config.method.help",
		},
		{
			Key:         "server_url",
			Type:        "string",
			Label:       "Server URL",
			LabelKey:    "config.server_url.label",
			Required:    true,
			Help:        "Base URL of the Signal gateway / daemon. Example: https://signal.example.com or http://localhost:8080.",
			HelpKey:     "config.server_url.help",
			Placeholder: "https://signal.example.com",
		},
		{
			Key:         "sender_number",
			Type:        "string",
			Label:       "Sender Number",
			LabelKey:    "config.sender_number.label",
			Required:    true,
			Help:        "Registered Signal account number in E.164 format. The bot's outbound identity. Used by every method.",
			HelpKey:     "config.sender_number.help",
			Placeholder: "+14155551234",
		},
		{
			Key:         "recipients",
			Type:        "string",
			Label:       "Recipients",
			LabelKey:    "fields.recipients",
			Required:    true,
			Help:        "Comma-separated list of recipient numbers in E.164 format. The plugin splits on commas and sends one Signal message per recipient.",
			HelpKey:     "help.recipients",
			Placeholder: "+14155551234, +14155555678",
		},

		// --- method: rest (signal-cli-rest-api) ---
		{
			Key:      "auth_username",
			Type:     "string",
			Label:    "Username",
			LabelKey: "config.auth_username.label",
			Required: false,
			Help:     "Optional. HTTP Basic auth username for signal-cli-rest-api. Leave blank if the daemon is unauthenticated.",
			HelpKey:  "config.auth_username.help",
		},
		{
			Key:      "auth_password",
			Type:     "secret",
			Label:    "Password",
			LabelKey: "config.auth_password.label",
			Required: false,
			Help:     "Optional. HTTP Basic auth password for signal-cli-rest-api. Stored encrypted at rest.",
			HelpKey:  "config.auth_password.help",
		},

		// --- method: bot (token-based bot server) ---
		{
			Key:      "bot_token",
			Type:     "secret",
			Label:    "Bot Token",
			LabelKey: "config.bot_token.label",
			Required: false,
			Help:     "Token-based Bot only. Bearer token for the Signal bot server. Issued when you register the bot with the platform. Stored encrypted at rest.",
			HelpKey:  "config.bot_token.help",
		},

		// --- method: signald (signald daemon) ---
		{
			Key:         "account_id",
			Type:        "string",
			Label:       "Account ID",
			LabelKey:    "config.account_id.label",
			Required:    false,
			Help:        "signald method only. signald account identifier — either the E.164 phone string or the UUID assigned at `signaldctl account register`. The plugin uses it in the JSON-RPC account path.",
			HelpKey:     "config.account_id.help",
			Placeholder: "+14155551234 or uuid",
		},
	}
}

// Init is called after RegisterAdapters. We stash the host
// context so the Signal client can audit on Test + Send.
func (p *CommSignalPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.comm-signal.init", map[string]any{"slug": p.manifest.Slug})
	}
	return nil
}

// Shutdown is called on graceful stop or uninstall.
func (p *CommSignalPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.comm-signal.shutdown", nil)
	}
	return nil
}

// Compile-time check that we satisfy the plugin.Plugin interface.
var _ plugin.Plugin = (*CommSignalPlugin)(nil)

// Plugin is the exported symbol `plugin.Open` looks up. It must be
// a variable (not a const) for `-buildmode=plugin` to find it.
var Plugin plugin.Plugin = &CommSignalPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "comm-signal",
		Name:    "Signal",
		Version: "2026.10.01",
	},
}
