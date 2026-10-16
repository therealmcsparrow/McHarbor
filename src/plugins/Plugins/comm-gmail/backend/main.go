// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the comm-gmail plugin. It exports
// a communication-channel notification adapter, not an email-server
// adapter. The host dispatches Test and Send from the communications
// module when channel_type == "gmail".
package main

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const gmailChannelType = "gmail"

// CommGmailPlugin is the singleton the host loads via plugin.Open.
type CommGmailPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the plugin manifest.
func (p *CommGmailPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes is a no-op; channels are managed by the host communications API.
func (p *CommGmailPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

// RegisterMenu is a no-op; this plugin appears as a channel type, not a page.
func (p *CommGmailPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

// RegisterWidgets is a no-op; this plugin contributes no dashboard widgets.
func (p *CommGmailPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

// RegisterNodes is a no-op; this plugin contributes no workflow nodes.
func (p *CommGmailPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

// RegisterPermissions is satisfied by manifest adds_permissions.
func (p *CommGmailPlugin) RegisterPermissions(reg plugin.AccessRegistry) error {
	_ = reg
	return nil
}

// RegisterAdapters registers Gmail as a notification channel type.
func (p *CommGmailPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Notification().Register(
		plugin.NotificationAdapterSpec{
			ChannelType: gmailChannelType,
			Slug:        p.manifest.Slug,
			DisplayName: "Gmail",
			Icon:        "plugin.svg",
			Description: p.manifest.Desc,
			Capabilities: plugin.NotificationAdapterCapabilities{
				EncryptedCredentials:    true,
				SupportsRichAttachments: false,
				// Gmail carries files as base64 MIME parts inside the
				// multipart/mixed message handed to messages.send.
				SupportsFileAttachments: true,

				// Gmail addresses the message from the same `to` field a caller may
				// retarget for a single send.
				RecipientOverride: plugin.NotificationRecipientOverride{
					Key:           "to",
					Kind:          "email",
					LabelEN:       "Recipients",
					PlaceholderEN: "board@example.com, cfo@example.com",
					Multiple:      true,
				},
			},
			ConfigSchema: gmailConfigSchema(),
		},
		NewGmailClient(),
	)
}

func gmailConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{Key: "name", Type: "string", Label: "Channel name", LabelKey: "nameLabel", Required: true, Help: "Display name for this Gmail communication channel.", HelpKey: "config.name.help", Placeholder: "Operations Gmail"},
		{Key: "client_id", Type: "string", Label: "Client ID", LabelKey: "clientIdLabel", Required: true, Help: "Google OAuth client ID with Gmail API send permissions.", HelpKey: "config.client_id.help", Placeholder: "1234567890-abc.apps.googleusercontent.com"},
		{Key: "client_secret", Type: "secret", Label: "Client secret", LabelKey: "clientSecretLabel", Required: true, Help: "Google OAuth client secret. Stored encrypted at rest.", HelpKey: "config.client_secret.help", Placeholder: "••••••••"},
		{Key: "from_address", Type: "string", Label: "From address", LabelKey: "fromAddressLabel", Required: true, Help: "Gmail user address used as the API sender.", HelpKey: "config.from_address.help", Placeholder: "mcharbor@example.com"},
		{Key: "from_name", Type: "string", Label: "From name", LabelKey: "fromNameLabel", Required: false, Help: "Optional display name for the From header.", HelpKey: "config.from_name.help", Placeholder: "McHarbor"},
		{Key: "to", Type: "string", Label: "Recipients", LabelKey: "toLabel", Required: true, Help: "Comma-separated recipient email addresses.", HelpKey: "config.to.help", Placeholder: "ops@example.com, oncall@example.com"},
		{Key: "subject_prefix", Type: "string", Label: "Subject prefix", LabelKey: "subjectPrefixLabel", Required: false, Default: "[McHarbor]", Help: "Optional prefix added to every notification subject.", HelpKey: "config.subject_prefix.help", Placeholder: "[McHarbor]"},
	}
}

// Init stores host context for audit events.
func (p *CommGmailPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.comm-gmail.init", map[string]any{"slug": p.manifest.Slug})
	}
	return nil
}

// Shutdown emits a best-effort audit event.
func (p *CommGmailPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.comm-gmail.shutdown", nil)
	}
	return nil
}

// Plugin is the symbol the McHarbor plugin runtime loads.
var Plugin plugin.Plugin = &CommGmailPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "comm-gmail",
		Name:    "Gmail",
		Version: "2026.10.01",
		Author:  "McSparrow <support@mcsparrow.eu>",
		Desc:    "Gmail communication channel. Delivers workflow, alert, and backup notifications as email through the Gmail API.",
	},
}
