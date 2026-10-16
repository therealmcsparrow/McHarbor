// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the comm-slack plugin. It exports
// a communication-channel notification adapter. The host dispatches
// Test and Send from the communications module when channel_type == "slack".
package main

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const slackChannelType = "slack"

// CommSlackPlugin is the singleton the host loads via plugin.Open.
type CommSlackPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the plugin manifest.
func (p *CommSlackPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes is a no-op; channels are managed by the host communications API.
func (p *CommSlackPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

// RegisterMenu is a no-op; this plugin appears as a channel type, not a page.
func (p *CommSlackPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

// RegisterWidgets is a no-op; this plugin contributes no dashboard widgets.
func (p *CommSlackPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

// RegisterNodes is a no-op; this plugin contributes no workflow nodes.
func (p *CommSlackPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

// RegisterPermissions is satisfied by manifest adds_permissions.
func (p *CommSlackPlugin) RegisterPermissions(reg plugin.AccessRegistry) error {
	_ = reg
	return nil
}

// RegisterAdapters registers Slack as a notification channel type.
func (p *CommSlackPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Notification().Register(
		plugin.NotificationAdapterSpec{
			ChannelType: slackChannelType,
			Slug:        p.manifest.Slug,
			DisplayName: "Slack",
			Icon:        "plugin.svg",
			Description: p.manifest.Desc,
			Capabilities: plugin.NotificationAdapterCapabilities{
				EncryptedCredentials:    true,
				SupportsRichAttachments: false,
				// Slack incoming webhooks cannot upload files; that needs a bot token and files.upload, which this channel's config does not carry.
				SupportsFileAttachments: false,
			},
			ConfigSchema: slackConfigSchema(),
		},
		NewSlackClient(),
	)
}

func slackConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{Key: "name", Type: "string", Label: "Channel name", LabelKey: "nameLabel", Required: true, Help: "Display name for this Slack communication channel.", HelpKey: "config.name.help", Placeholder: "Operations Slack"},
		{Key: "webhook_url", Type: "secret", Label: "Webhook URL", LabelKey: "webhookUrlLabel", Required: true, Help: "Slack incoming webhook URL. Stored encrypted at rest.", HelpKey: "config.webhook_url.help", Placeholder: "https://hooks.slack.com/services/..."},
	}
}

// Init stores host context for audit events.
func (p *CommSlackPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.comm-slack.init", map[string]any{"slug": p.manifest.Slug})
	}
	return nil
}

// Shutdown emits a best-effort audit event.
func (p *CommSlackPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.comm-slack.shutdown", nil)
	}
	return nil
}

// Plugin is the symbol the McHarbor plugin runtime loads.
var Plugin plugin.Plugin = &CommSlackPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "comm-slack",
		Name:    "Slack",
		Version: "2026.10.01",
		Author:  "McSparrow <support@mcsparrow.eu>",
		Desc:    "Slack communication channel. Delivers workflow, alert, and backup notifications to a Slack channel through an incoming webhook URL.",
	},
}
