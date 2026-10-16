// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the comm-discord plugin. It exports
// a communication-channel notification adapter. The host dispatches
// Test and Send from the communications module when channel_type == "discord".
package main

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const discordChannelType = "discord"

// CommDiscordPlugin is the singleton the host loads via plugin.Open.
type CommDiscordPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the plugin manifest.
func (p *CommDiscordPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes is a no-op; channels are managed by the host communications API.
func (p *CommDiscordPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

// RegisterMenu is a no-op; this plugin appears as a channel type, not a page.
func (p *CommDiscordPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

// RegisterWidgets is a no-op; this plugin contributes no dashboard widgets.
func (p *CommDiscordPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

// RegisterNodes is a no-op; this plugin contributes no workflow nodes.
func (p *CommDiscordPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

// RegisterPermissions is satisfied by manifest adds_permissions.
func (p *CommDiscordPlugin) RegisterPermissions(reg plugin.AccessRegistry) error {
	_ = reg
	return nil
}

// RegisterAdapters registers Discord as a notification channel type.
func (p *CommDiscordPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Notification().Register(
		plugin.NotificationAdapterSpec{
			ChannelType: discordChannelType,
			Slug:        p.manifest.Slug,
			DisplayName: "Discord",
			Icon:        "plugin.svg",
			Description: p.manifest.Desc,
			Capabilities: plugin.NotificationAdapterCapabilities{
				EncryptedCredentials:    true,
				SupportsRichAttachments: false,
				// Discord webhooks accept a multipart file upload alongside the message.
				SupportsFileAttachments: true,
			},
			ConfigSchema: discordConfigSchema(),
		},
		NewDiscordClient(),
	)
}

func discordConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{Key: "name", Type: "string", Label: "Channel name", LabelKey: "nameLabel", Required: true, Help: "Display name for this Discord communication channel.", HelpKey: "config.name.help", Placeholder: "Operations Discord"},
		{Key: "webhook_url", Type: "secret", Label: "Webhook URL", LabelKey: "webhookUrlLabel", Required: true, Help: "Discord channel webhook URL. Stored encrypted at rest.", HelpKey: "config.webhook_url.help", Placeholder: "https://discord.com/api/webhooks/..."},
	}
}

// Init stores host context for audit events.
func (p *CommDiscordPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.comm-discord.init", map[string]any{"slug": p.manifest.Slug})
	}
	return nil
}

// Shutdown emits a best-effort audit event.
func (p *CommDiscordPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.comm-discord.shutdown", nil)
	}
	return nil
}

// Plugin is the symbol the McHarbor plugin runtime loads.
var Plugin plugin.Plugin = &CommDiscordPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "comm-discord",
		Name:    "Discord",
		Version: "2026.10.01",
		Author:  "McSparrow <support@mcsparrow.eu>",
		Desc:    "Discord communication channel. Delivers workflow, alert, and backup notifications to a Discord channel through a webhook URL.",
	},
}
