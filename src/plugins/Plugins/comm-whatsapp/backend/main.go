// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the comm-whatsapp plugin. It
// exports a single symbol — `Plugin` — that the host loads via
// `plugin.Open` and dispatches to.
//
// Scope: this plugin owns a single ChannelType ("whatsapp"). The
// whatsappClient implements plugin.NotificationAdapterClient
// (Test + Send only). The Send surface branches on a per-channel
// `method` field so one channel can route through four different
// connection surfaces:
//
//  1. cloud   — Meta Cloud API
//  2. gateway — Self-hosted gateway
//  3. bot     — WhatsApp Business API platform
//  4. saas    — Third-party SaaS provider
//
// Why notification (not storage): WhatsApp channels deliver
// outgoing alerts, workflow events, and backup notifications —
// not files. The contract is intentionally narrow: Test to
// validate credentials + Send to deliver a Title/Message payload.
package main

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// whatsappChannelType is the ChannelType this plugin owns.
// Single value — the host's communication_channels.channel_type
// column accepts this string.
const whatsappChannelType = "whatsapp"

// Method identifiers written to cfg["method"]. The host's admin
// form renders a tab per method; the Send surface switches on
// these strings. Lowercase + stable — DO NOT rename without a
// data migration.
const (
	methodCloud   = "cloud"   // Official Meta Cloud API
	methodGateway = "gateway" // Self-hosted gateway
	methodBot     = "bot"     // WhatsApp Business API platform
	methodSaas    = "saas"    // Third-party SaaS provider
)

// CommWhatsappPlugin is the singleton the host dispatches to.
// It owns one ChannelType's worth of NotificationAdapterSpecs and
// routes host calls into whatsappClient.
type CommWhatsappPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the parsed plugin.yaml.
func (p *CommWhatsappPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes — comm-whatsapp does not register HTTP routes.
// The host's /api/channels CRUD is the single admin surface and
// routes are owned by the host module.
func (p *CommWhatsappPlugin) RegisterRoutes(_ plugin.Registry) error {
	return nil
}

// RegisterMenu is intentionally a no-op. WhatsApp channels are
// managed from the host's Settings > Notifications > Channels
// page, which lists every registered ChannelType (built-in +
// plugin). A dedicated sidemenu entry would just duplicate the
// surface.
func (p *CommWhatsappPlugin) RegisterMenu(_ plugin.MenuRegistry) error {
	return nil
}

// RegisterWidgets — this plugin has no dashboard widgets.
func (p *CommWhatsappPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error {
	return nil
}

// RegisterNodes — this plugin has no workflow nodes.
func (p *CommWhatsappPlugin) RegisterNodes(_ plugin.NodeRegistry) error {
	return nil
}

// RegisterPermissions registers the adds_permissions declared in
// the plugin manifest. The host consults this on Enable + on
// every permission check; without it the keys would never be
// visible to operators.
func (p *CommWhatsappPlugin) RegisterPermissions(reg plugin.AccessRegistry) error {
	// The single declared permission (channels.use_comm_whatsapp)
	// is registered by the host automatically from the manifest's
	// adds_permissions block, so this is a no-op for now. If we
	// ever add ABAC rules or dynamic scopes, wire them here.
	_ = reg
	return nil
}

// RegisterAdapters registers the WhatsApp client for its single
// ChannelType. The cfg map the host hands to Test/Send carries
// the channel row's columns keyed by ConfigFieldDecl.Key.
//
// SupportsRichAttachments is false in v1: WhatsApp template
// messages + media attachments are a follow-up — the host
// currently delivers Title + Message text only, and every
// connection method below supports plain text bodies.
//
// Slug + Icon + Description are populated from the plugin's
// own manifest so the host's "Add Communication" picker can
// render this plugin as a tile via /api/plugins/loader/icon.
func (p *CommWhatsappPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Notification().Register(
		plugin.NotificationAdapterSpec{
			ChannelType: whatsappChannelType,
			Slug:        p.manifest.Slug,
			DisplayName: "WhatsApp",
			Icon:        "plugin.svg",
			Description: p.manifest.Desc,
			Capabilities: plugin.NotificationAdapterCapabilities{
				EncryptedCredentials:    true,
				SupportsRichAttachments: false,
				SupportsFileAttachments: true,

				// The business number and token stay the channel's; only the number
				// being messaged moves.
				RecipientOverride: plugin.NotificationRecipientOverride{
					Key:           "recipient_phone",
					Kind:          "phone",
					LabelEN:       "Recipient number",
					PlaceholderEN: "+31612345678",
					Multiple:      false,
				},
			},
			ConfigSchema: whatsappConfigSchema(),
		},
		NewWhatsappClient(),
	)
}

// whatsappConfigSchema returns the form fields the admin UI
// renders when an operator picks this channel type.
//
// The schema is the UNION of fields across all four methods:
// the host's form renderer groups them under one "Method" tab
// selector and shows the relevant subset once the operator
// picks a method. Required + secret flags still apply — the
// form must validate per-method before Save.
//
// Fields summary (see whatsapp_client.go for endpoint shapes):
//
//	method             required  string   cloud|gateway|bot|saas
//	recipient_phone    required  string   E.164 phone number (+CCXXXXXXXXXX)
//
//	cloud (Meta Cloud API):
//	  phone_number_id  required  string   numeric, from Meta Business Manager
//	  access_token     required  secret   permanent system-user token
//
//	gateway (Self-hosted):
//	  gateway_url      required  string   full https://host:port base
//	  gateway_token    required  secret   bearer / x-api-key value
//
//	bot (Business API platform):
//	  business_api_url required  string   full https://provider endpoint
//	  api_key          required  secret   platform API key
//
//	saas (Third-party SaaS):
//	  provider_url     required  string   full https://provider base
//	  provider_api_key required  secret   provider API key
//	  provider_from    required  string   sender id (Twilio WhatsApp from, etc.)
func whatsappConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		// --- discriminator + common recipient ---
		{
			Key:      "method",
			Type:     "string",
			Label:    "Connection Method",
			LabelKey: "fields.method",
			Required: true,
			Default:  methodCloud,
			Help:     "Which WhatsApp connection surface to use. Cloud = official Meta Cloud API. Gateway = your own Baileys/wa-gateway server. Bot = WhatsApp Business API platform (360dialog/Twilio-style). SaaS = a third-party messaging provider.",
			HelpKey:  "config.method.help",
		},
		{
			Key:         "recipient_phone",
			Type:        "string",
			Label:       "Recipient Phone",
			LabelKey:    "fields.recipientPhone",
			Required:    true,
			Help:        "Recipient phone number in E.164 format (e.g. +14155551234). Used by every method.",
			HelpKey:     "config.recipient_phone.help",
			Placeholder: "+14155551234",
		},

		// --- method: cloud (Meta Cloud API) ---
		{
			Key:         "phone_number_id",
			Type:        "string",
			Label:       "Phone Number ID",
			LabelKey:    "fields.phoneNumberId",
			Required:    false,
			Help:        "Meta Cloud API only. The numeric Phone Number ID from the Meta Business Manager (WhatsApp > Phone Numbers). Used to build /v21.0/<phone_number_id>/messages.",
			HelpKey:     "config.phone_number_id.help",
			Placeholder: "123456789012345",
		},
		{
			Key:      "access_token",
			Type:     "secret",
			Label:    "Access Token",
			LabelKey: "fields.accessToken",
			Required: false,
			Help:     "Meta Cloud API only. Permanent system-user access token from the Meta Business Manager. Stored encrypted at rest.",
			HelpKey:  "config.access_token.help",
		},

		// --- method: gateway (self-hosted) ---
		{
			Key:         "gateway_url",
			Type:        "string",
			Label:       "Gateway URL",
			LabelKey:    "fields.gatewayUrl",
			Required:    false,
			Help:        "Self-hosted gateway only. Base URL of your own WhatsApp gateway (Baileys, wa-gateway, etc.). The plugin POSTs {title, message, phone} to <gateway_url>/send.",
			HelpKey:     "config.gateway_url.help",
			Placeholder: "https://wa-gateway.local:8443",
		},
		{
			Key:      "gateway_token",
			Type:     "secret",
			Label:    "Gateway Token",
			LabelKey: "fields.gatewayToken",
			Required: false,
			Help:     "Self-hosted gateway only. Bearer / API-key token presented as Authorization: Bearer <token>. Stored encrypted at rest.",
			HelpKey:  "config.gateway_token.help",
		},

		// --- method: bot (Business API platform) ---
		{
			Key:         "business_api_url",
			Type:        "string",
			Label:       "Business API URL",
			LabelKey:    "fields.businessApiUrl",
			Required:    false,
			Help:        "Business Bot Platform only. Full endpoint of the platform's send-message route (e.g. https://waba.360dialog.com/v1/messages or your Twilio WhatsApp API base).",
			HelpKey:     "config.business_api_url.help",
			Placeholder: "https://waba.360dialog.com/v1/messages",
		},
		{
			Key:      "api_key",
			Type:     "secret",
			Label:    "API Key",
			LabelKey: "fields.apiKey",
			Required: false,
			Help:     "Business Bot Platform only. Platform API key sent in the D360-API-KEY header (or X-API-Key, depending on the platform). Stored encrypted at rest.",
			HelpKey:  "config.api_key.help",
		},

		// --- method: saas (third-party SaaS) ---
		{
			Key:         "provider_url",
			Type:        "string",
			Label:       "Provider URL",
			LabelKey:    "fields.providerUrl",
			Required:    false,
			Help:        "Third-party SaaS only. Base URL of the provider's send-message endpoint. The plugin POSTs {to, from, body} to <provider_url>/messages.",
			HelpKey:     "config.provider_url.help",
			Placeholder: "https://api.twilio.com/2010-04-01",
		},
		{
			Key:      "provider_api_key",
			Type:     "secret",
			Label:    "Provider API Key",
			LabelKey: "fields.providerApiKey",
			Required: false,
			Help:     "Third-party SaaS only. Provider API key (or Account SID + Auth Token for Twilio). Stored encrypted at rest.",
			HelpKey:  "config.provider_api_key.help",
		},
		{
			Key:         "provider_from",
			Type:        "string",
			Label:       "Provider From",
			LabelKey:    "fields.providerFrom",
			Required:    false,
			Help:        "Third-party SaaS only. Sender identifier expected by the provider (Twilio WhatsApp 'From' number, MessageBird originator, Gupshup app id, etc.).",
			HelpKey:     "config.provider_from.help",
			Placeholder: "whatsapp:+14155550000",
		},
	}
}

// Init is called after RegisterAdapters. We stash the host
// context so the WhatsApp client can audit on Test + Send.
func (p *CommWhatsappPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.comm-whatsapp.init", map[string]any{"slug": p.manifest.Slug})
	}
	return nil
}

// Shutdown is called on graceful stop or uninstall.
func (p *CommWhatsappPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.comm-whatsapp.shutdown", nil)
	}
	return nil
}

// Compile-time check that we satisfy the plugin.Plugin interface.
var _ plugin.Plugin = (*CommWhatsappPlugin)(nil)

// Plugin is the exported symbol `plugin.Open` looks up. It must be
// a variable (not a const) for `-buildmode=plugin` to find it.
var Plugin plugin.Plugin = &CommWhatsappPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "comm-whatsapp",
		Name:    "WhatsApp",
		Version: "2026.10.01",
	},
}
