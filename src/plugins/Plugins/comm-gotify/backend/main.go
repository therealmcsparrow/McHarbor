// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the comm-gotify plugin. It exports
// a communication-channel notification adapter. The host dispatches
// Test and Send from the communications module when channel_type == "gotify".
package main

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const gotifyChannelType = "gotify"

// CommGotifyPlugin is the singleton the host loads via plugin.Open.
type CommGotifyPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the plugin manifest.
func (p *CommGotifyPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes is a no-op; channels are managed by the host communications API.
func (p *CommGotifyPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

// RegisterMenu is a no-op; this plugin appears as a channel type, not a page.
func (p *CommGotifyPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

// RegisterWidgets is a no-op; this plugin contributes no dashboard widgets.
func (p *CommGotifyPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

// RegisterNodes is a no-op; this plugin contributes no workflow nodes.
func (p *CommGotifyPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

// RegisterPermissions is satisfied by manifest adds_permissions.
func (p *CommGotifyPlugin) RegisterPermissions(reg plugin.AccessRegistry) error {
	_ = reg
	return nil
}

// RegisterAdapters registers Gotify as a notification channel type.
func (p *CommGotifyPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Notification().Register(
		plugin.NotificationAdapterSpec{
			ChannelType: gotifyChannelType,
			Slug:        p.manifest.Slug,
			DisplayName: "Gotify",
			Icon:        "plugin.svg",
			Description: p.manifest.Desc,
			Capabilities: plugin.NotificationAdapterCapabilities{
				EncryptedCredentials:    true,
				SupportsRichAttachments: false,
				// Gotify has no attachment API — messages carry text and extras only.
				SupportsFileAttachments: false,
			},
			ConfigSchema: gotifyConfigSchema(),
		},
		NewGotifyClient(),
	)
}

func gotifyConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{Key: "name", Type: "string", Label: "Channel name", LabelKey: "nameLabel", Required: true, Help: "Display name for this Gotify communication channel.", HelpKey: "config.name.help", Placeholder: "Operations Gotify"},
		{Key: "server_url", Type: "string", Label: "Server URL", LabelKey: "serverUrlLabel", Required: true, Help: "Base URL of the Gotify server.", HelpKey: "config.server_url.help", Placeholder: "https://gotify.example.com"},
		{Key: "token", Type: "secret", Label: "Application token", LabelKey: "tokenLabel", Required: true, Help: "Gotify application token. Stored encrypted at rest.", HelpKey: "config.token.help", Placeholder: "••••••••"},
		{Key: "priority", Type: "string", Label: "Priority", LabelKey: "priorityLabel", Required: false, Help: "Optional Gotify priority integer.", HelpKey: "config.priority.help", Placeholder: "5"},
	}
}

// Init stores host context for audit events.
func (p *CommGotifyPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.comm-gotify.init", map[string]any{"slug": p.manifest.Slug})
	}
	return nil
}

// Shutdown emits a best-effort audit event.
func (p *CommGotifyPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.comm-gotify.shutdown", nil)
	}
	return nil
}

// Plugin is the symbol the McHarbor plugin runtime loads.
var Plugin plugin.Plugin = &CommGotifyPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "comm-gotify",
		Name:    "Gotify",
		Version: "2026.10.01",
		Author:  "McSparrow <support@mcsparrow.eu>",
		Desc:    "Gotify communication channel. Delivers workflow, alert, and backup notifications to a Gotify server through an application token.",
	},
}
