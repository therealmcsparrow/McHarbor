// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the comm-exchange plugin. It exports
// a communication-channel notification adapter, not an email-server
// adapter. The host dispatches Test and Send from the communications
// module when channel_type == "exchange".
package main

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const exchangeChannelType = "exchange"

// CommExchangePlugin is the singleton the host loads via plugin.Open.
type CommExchangePlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the plugin manifest.
func (p *CommExchangePlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes is a no-op; channels are managed by the host communications API.
func (p *CommExchangePlugin) RegisterRoutes(_ plugin.Registry) error { return nil }

// RegisterMenu is a no-op; this plugin appears as a channel type, not a page.
func (p *CommExchangePlugin) RegisterMenu(_ plugin.MenuRegistry) error { return nil }

// RegisterWidgets is a no-op; this plugin contributes no dashboard widgets.
func (p *CommExchangePlugin) RegisterWidgets(_ plugin.WidgetRegistry) error { return nil }

// RegisterNodes is a no-op; this plugin contributes no workflow nodes.
func (p *CommExchangePlugin) RegisterNodes(_ plugin.NodeRegistry) error { return nil }

// RegisterPermissions is satisfied by manifest adds_permissions.
func (p *CommExchangePlugin) RegisterPermissions(reg plugin.AccessRegistry) error {
	_ = reg
	return nil
}

// RegisterAdapters registers Exchange Online as a notification channel type.
func (p *CommExchangePlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Notification().Register(
		plugin.NotificationAdapterSpec{
			ChannelType: exchangeChannelType,
			Slug:        p.manifest.Slug,
			DisplayName: "Exchange Online",
			Icon:        "plugin.svg",
			Description: p.manifest.Desc,
			Capabilities: plugin.NotificationAdapterCapabilities{
				EncryptedCredentials:    true,
				SupportsRichAttachments: false,
				// Graph sendMail carries files as #microsoft.graph.fileAttachment
				// entries while the whole request stays under 4 MB; past that
				// the adapter uploads them into a draft through an upload
				// session, up to Graph's 150 MB message ceiling.
				SupportsFileAttachments: true,

				// Graph sendMail takes its recipients per request, so one send can go
				// somewhere other than the channel's configured list.
				RecipientOverride: plugin.NotificationRecipientOverride{
					Key:           "to",
					Kind:          "email",
					LabelEN:       "Recipients",
					PlaceholderEN: "board@example.com, cfo@example.com",
					Multiple:      true,
				},
			},
			ConfigSchema: exchangeConfigSchema(),
		},
		NewExchangeClient(),
	)
}

func exchangeConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{Key: "name", Type: "string", Label: "Channel name", LabelKey: "nameLabel", Required: true, Help: "Display name for this Exchange Online communication channel.", HelpKey: "config.name.help", Placeholder: "Operations Exchange"},
		{Key: "tenant_id", Type: "string", Label: "Tenant ID", LabelKey: "tenantIdLabel", Required: true, Help: "Microsoft Entra tenant ID used for the client-credentials token request.", HelpKey: "config.tenant_id.help", Placeholder: "00000000-0000-0000-0000-000000000000"},
		{Key: "client_id", Type: "string", Label: "Client ID", LabelKey: "clientIdLabel", Required: true, Help: "Microsoft Entra app registration client ID with Microsoft Graph Mail.Send permission.", HelpKey: "config.client_id.help", Placeholder: "00000000-0000-0000-0000-000000000000"},
		{Key: "client_secret", Type: "secret", Label: "Client secret", LabelKey: "clientSecretLabel", Required: true, Help: "Microsoft Entra client secret. Stored encrypted at rest.", HelpKey: "config.client_secret.help", Placeholder: "••••••••"},
		{Key: "from_address", Type: "string", Label: "From address", LabelKey: "fromAddressLabel", Required: true, Help: "Exchange Online mailbox used as the Graph sendMail user.", HelpKey: "config.from_address.help", Placeholder: "mcharbor@example.com"},
		{Key: "from_name", Type: "string", Label: "From name", LabelKey: "fromNameLabel", Required: false, Help: "Optional display name retained for compatibility with other email channels.", HelpKey: "config.from_name.help", Placeholder: "McHarbor"},
		{Key: "to", Type: "string", Label: "Recipients", LabelKey: "toLabel", Required: true, Help: "Comma-separated recipient email addresses.", HelpKey: "config.to.help", Placeholder: "ops@example.com, oncall@example.com"},
		{Key: "subject_prefix", Type: "string", Label: "Subject prefix", LabelKey: "subjectPrefixLabel", Required: false, Default: "[McHarbor]", Help: "Optional prefix added to every notification subject.", HelpKey: "config.subject_prefix.help", Placeholder: "[McHarbor]"},
	}
}

// Init stores host context for audit events.
func (p *CommExchangePlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.comm-exchange.init", map[string]any{"slug": p.manifest.Slug})
	}
	return nil
}

// Shutdown emits a best-effort audit event.
func (p *CommExchangePlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.comm-exchange.shutdown", nil)
	}
	return nil
}

// Plugin is the symbol the McHarbor plugin runtime loads.
var Plugin plugin.Plugin = &CommExchangePlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "comm-exchange",
		Name:    "Exchange",
		Version: "2026.10.01",
		Author:  "McSparrow <support@mcsparrow.eu>",
		Desc:    "Exchange Online communication channel. Delivers workflow, alert, and backup notifications as email through Microsoft Graph.",
	},
}
