// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the comm-smtp plugin. It exports
// a communication-channel notification adapter, not an email-server
// adapter. The host dispatches Test and Send from the communications
// module when channel_type == "smtp".
package main

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const smtpChannelType = "smtp"

// CommSMTPPlugin is the singleton the host loads via plugin.Open.
type CommSMTPPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the plugin manifest.
func (p *CommSMTPPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes is a no-op; channels are managed by the host communications API.
func (p *CommSMTPPlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

// RegisterMenu is a no-op; this plugin appears as a channel type, not a page.
func (p *CommSMTPPlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

// RegisterWidgets is a no-op; this plugin contributes no dashboard widgets.
func (p *CommSMTPPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

// RegisterNodes is a no-op; this plugin contributes no workflow nodes.
func (p *CommSMTPPlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

// RegisterPermissions is satisfied by manifest adds_permissions.
func (p *CommSMTPPlugin) RegisterPermissions(reg plugin.AccessRegistry) error {
	_ = reg
	return nil
}

// RegisterAdapters registers SMTP as a notification channel type.
func (p *CommSMTPPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Notification().Register(
		plugin.NotificationAdapterSpec{
			ChannelType: smtpChannelType,
			Slug:        p.manifest.Slug,
			DisplayName: "SMTP email",
			Icon:        "plugin.svg",
			Description: p.manifest.Desc,
			Capabilities: plugin.NotificationAdapterCapabilities{
				EncryptedCredentials:    true,
				SupportsRichAttachments: false,
				// SMTP carries files as MIME multipart/mixed parts.
				SupportsFileAttachments: true,

				// A mail channel addressed to the ops list can carry one report to
				// somebody else without a second channel being configured.
				RecipientOverride: plugin.NotificationRecipientOverride{
					Key:           "to",
					Kind:          "email",
					LabelEN:       "Recipients",
					PlaceholderEN: "board@example.com, cfo@example.com",
					Multiple:      true,
				},
			},
			ConfigSchema: smtpConfigSchema(),
		},
		NewSMTPClient(),
	)
}

func smtpConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{Key: "name", Type: "string", Label: "Channel name", LabelKey: "nameLabel", Required: true, Help: "Display name for this SMTP communication channel.", HelpKey: "config.name.help", Placeholder: "Operations email"},
		{Key: "host", Type: "string", Label: "SMTP host", LabelKey: "hostLabel", Required: true, Help: "SMTP relay hostname or IP address.", HelpKey: "config.host.help", Placeholder: "smtp.example.com"},
		{Key: "port", Type: "string", Label: "SMTP port", LabelKey: "portLabel", Required: true, Default: "587", Help: "SMTP port. Use 587 for STARTTLS, 465 for implicit TLS, or 25 for plain/internal relays.", HelpKey: "config.port.help", Placeholder: "587"},
		{Key: "encryption", Type: "string", Label: "Encryption", LabelKey: "encryptionLabel", Required: true, Default: "starttls", Help: "One of starttls, ssl_tls, or none. STARTTLS is recommended for port 587.", HelpKey: "config.encryption.help", Placeholder: "starttls"},
		{Key: "auth_method", Type: "string", Label: "Authentication", LabelKey: "authMethodLabel", Required: true, Default: "plain", Help: "One of none, plain, login, or cram_md5.", HelpKey: "config.auth_method.help", Placeholder: "plain"},
		{Key: "username", Type: "string", Label: "Username", LabelKey: "usernameLabel", Required: false, Help: "SMTP AUTH username. Leave blank when authentication is none.", HelpKey: "config.username.help", Placeholder: "alerts@example.com"},
		{Key: "password", Type: "secret", Label: "Password", LabelKey: "passwordLabel", Required: false, Help: "SMTP AUTH password or app password. Stored encrypted at rest.", HelpKey: "config.password.help", Placeholder: "••••••••"},
		{Key: "from_address", Type: "string", Label: "From address", LabelKey: "fromAddressLabel", Required: true, Help: "Envelope sender and From header.", HelpKey: "config.from_address.help", Placeholder: "mcharbor@example.com"},
		{Key: "from_name", Type: "string", Label: "From name", LabelKey: "fromNameLabel", Required: false, Help: "Optional display name for the From header.", HelpKey: "config.from_name.help", Placeholder: "McHarbor"},
		{Key: "to", Type: "string", Label: "Recipients", LabelKey: "toLabel", Required: true, Help: "Comma-separated recipient email addresses.", HelpKey: "config.to.help", Placeholder: "ops@example.com, oncall@example.com"},
		{Key: "subject_prefix", Type: "string", Label: "Subject prefix", LabelKey: "subjectPrefixLabel", Required: false, Default: "[McHarbor]", Help: "Optional prefix added to every notification subject.", HelpKey: "config.subject_prefix.help", Placeholder: "[McHarbor]"},
	}
}

// Init stores host context for audit events.
func (p *CommSMTPPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.comm-smtp.init", map[string]any{"slug": p.manifest.Slug})
	}
	return nil
}

// Shutdown emits a best-effort audit event.
func (p *CommSMTPPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.comm-smtp.shutdown", nil)
	}
	return nil
}

// Plugin is the symbol the McHarbor plugin runtime loads.
var Plugin plugin.Plugin = &CommSMTPPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "comm-smtp",
		Name:    "SMTP",
		Version: "2026.10.01",
		Author:  "McSparrow <support@mcsparrow.eu>",
		Desc:    "SMTP communication channel. Delivers workflow, alert, and backup notifications as email through a configured SMTP relay.",
	},
}
