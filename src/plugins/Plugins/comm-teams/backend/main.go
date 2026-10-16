// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the comm-teams plugin. It
// exports a single symbol — `Plugin` — that the host loads via
// `plugin.Open` and dispatches to.
//
// Scope: this plugin owns a single ChannelType ("teams"). The
// comm-teamsClient implements plugin.NotificationAdapterClient
// (Test + Send only) and POSTs an Adaptive Card to a Teams
// Incoming Webhook connector URL.
//
// Why notification (not storage): Teams channels deliver outgoing
// alerts, workflow events, and backup notifications — not files.
// The contract is intentionally narrow: Test to validate the
// webhook URL + Send to deliver a Title/Message payload.
package main

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// teamsChannelType is the ChannelType this plugin owns. Single
// value — the host's communication_channels.channel_type column
// accepts this string.
const teamsChannelType = "teams"

// CommTeamsPlugin is the singleton the host dispatches to. It
// owns one ChannelType's worth of NotificationAdapterSpecs and
// routes host calls into teamsClient.
type CommTeamsPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the parsed plugin.yaml.
func (p *CommTeamsPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes — comm-teams does not register HTTP routes.
// The host's /api/channels CRUD is the single admin surface and
// routes are owned by the host module.
func (p *CommTeamsPlugin) RegisterRoutes(_ plugin.Registry) error {
	return nil
}

// RegisterMenu is intentionally a no-op. Teams channels are
// managed from the host's Settings > Notifications > Channels
// page, which lists every registered ChannelType (built-in +
// plugin). A dedicated sidemenu entry would just duplicate the
// surface.
func (p *CommTeamsPlugin) RegisterMenu(_ plugin.MenuRegistry) error {
	return nil
}

// RegisterWidgets — this plugin has no dashboard widgets.
func (p *CommTeamsPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error {
	return nil
}

// RegisterNodes — this plugin has no workflow nodes.
func (p *CommTeamsPlugin) RegisterNodes(_ plugin.NodeRegistry) error {
	return nil
}

// RegisterPermissions registers the adds_permissions declared in
// the plugin manifest. The host consults this on Enable + on
// every permission check; without it the keys would never be
// visible to operators.
func (p *CommTeamsPlugin) RegisterPermissions(reg plugin.AccessRegistry) error {
	// The single declared permission (channels.use_comm_teams)
	// is registered by the host automatically from the manifest's
	// adds_permissions block, so this is a no-op for now. If we
	// ever add ABAC rules or dynamic scopes, wire them here.
	_ = reg
	return nil
}

// RegisterAdapters registers the Teams client for its single
// ChannelType. The cfg map the host hands to Test/Send carries
// the channel row's columns (webhook_url, optional theme_color)
// keyed by column name.
//
// SupportsRichAttachments is false in v1: the Adaptive Card the
// plugin emits is text-only (Title + Message TextBlocks). Inline
// images, action buttons, and column layouts are a follow-up —
// they require per-message styling that operators rarely want for
// alert/backup notifications.
//
// Slug + Icon + Description are populated from the plugin's own
// manifest so the host's "Add Communication" picker can render
// this plugin as a tile via /api/plugins/loader/icon.
func (p *CommTeamsPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Notification().Register(
		plugin.NotificationAdapterSpec{
			ChannelType: teamsChannelType,
			Slug:        p.manifest.Slug,
			DisplayName: "Microsoft Teams",
			Icon:        "plugin.svg",
			Description: p.manifest.Desc,
			Capabilities: plugin.NotificationAdapterCapabilities{
				EncryptedCredentials:    false,
				SupportsRichAttachments: false,
				// Teams Workflows/webhook cards cannot carry a file; a link is the only option.
				SupportsFileAttachments: false,
			},
			ConfigSchema: teamsConfigSchema(),
		},
		NewTeamsClient(),
	)
}

// teamsConfigSchema returns the form fields the admin UI renders
// when an operator picks this channel type.
//
// webhook_url is the Microsoft Teams Incoming Webhook connector
// URL — the random UUID embedded in the URL path IS the auth
// credential. Anyone holding the URL can post to the channel,
// so treat it as a secret (do not paste into public chats or
// commit to git). The host stores this in the channel row
// alongside the channel_type, matching how Slack and Discord
// webhook URLs are stored.
//
// theme_color is optional. Teams renders the colored stripe on
// the left edge of every Adaptive Card; defaults to Teams brand
// purple (#5059C9) when blank. Operators can override per-channel
// to color-code by environment or severity (e.g. red for prod
// alerts, green for backup-complete).
func teamsConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "webhook_url",
			Type:        "string",
			Label:       "Webhook URL",
			LabelKey:    "fields.webhookUrl",
			Required:    true,
			Help:        "Incoming Webhook connector URL from Microsoft Teams. Channel menu (...) → Connectors → Incoming Webhook → Configure → copy the URL. The UUID in the URL is the auth credential — treat it as a secret.",
			HelpKey:     "help.webhookUrl",
			Placeholder: "https://outlook.office.com/webhook/…@…/IncomingWebhook/…/…",
		},
		{
			Key:         "theme_color",
			Type:        "string",
			Label:       "Theme Color",
			LabelKey:    "fields.themeColor",
			Required:    false,
			Default:     "5059C9",
			Help:        "Optional. Hex color (no #) used for the Adaptive Card's left accent stripe. Defaults to Teams brand purple. Useful for color-coding by environment or severity.",
			HelpKey:     "config.theme_color.help",
			Placeholder: "5059C9",
		},
	}
}

// Init is called after RegisterAdapters. We stash the host
// context so the Teams client can audit on Test + Send.
func (p *CommTeamsPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.comm-teams.init", map[string]any{"slug": p.manifest.Slug})
	}
	return nil
}

// Shutdown is called on graceful stop or uninstall.
func (p *CommTeamsPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.comm-teams.shutdown", nil)
	}
	return nil
}

// Compile-time check that we satisfy the plugin.Plugin interface.
var _ plugin.Plugin = (*CommTeamsPlugin)(nil)

// Plugin is the exported symbol `plugin.Open` looks up. It must
// be a variable (not a const) for `-buildmode=plugin` to find it.
var Plugin plugin.Plugin = &CommTeamsPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "comm-teams",
		Name:    "Microsoft Teams",
		Version: "2026.10.01",
	},
}
