// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main is the Go side of the comm-telegram plugin. It
// exports a single symbol — `Plugin` — that the host loads via
// `plugin.Open` and dispatches to.
//
// Scope: this plugin owns a single ChannelType ("telegram"). The
// comm-telegramClient implements plugin.NotificationAdapterClient
// (Test + Send only) and dispatches messages via the Telegram
// Bot API at https://api.telegram.org/bot<token>/...
//
// Why notification (not storage): Telegram channels deliver
// outgoing alerts, workflow events, and backup notifications —
// not files. The contract is intentionally narrow: Test to
// validate credentials + Send to deliver a Title/Message payload.
package main

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// telegramChannelType is the ChannelType this plugin owns.
// Single value — the host's communication_channels.channel_type
// column accepts this string.
const telegramChannelType = "telegram"

// CommTelegramPlugin is the singleton the host dispatches to. It
// owns one ChannelType's worth of NotificationAdapterSpecs and
// routes host calls into telegramClient.
type CommTelegramPlugin struct {
	manifest plugin.Manifest
	host     plugin.HostContext
}

// Manifest returns the parsed plugin.yaml.
func (p *CommTelegramPlugin) Manifest() plugin.Manifest { return p.manifest }

// RegisterRoutes — comm-telegram does not register HTTP routes.
// The host's /api/channels CRUD is the single admin surface and
// routes are owned by the host module.
func (p *CommTelegramPlugin) RegisterRoutes(_ plugin.Registry) error {
	return nil
}

// RegisterMenu is intentionally a no-op. Telegram channels are
// managed from the host's Settings > Notifications > Channels
// page, which lists every registered ChannelType (built-in +
// plugin). A dedicated sidemenu entry would just duplicate the
// surface.
func (p *CommTelegramPlugin) RegisterMenu(_ plugin.MenuRegistry) error {
	return nil
}

// RegisterWidgets — this plugin has no dashboard widgets.
func (p *CommTelegramPlugin) RegisterWidgets(_ plugin.WidgetRegistry) error {
	return nil
}

// RegisterNodes — this plugin has no workflow nodes.
func (p *CommTelegramPlugin) RegisterNodes(_ plugin.NodeRegistry) error {
	return nil
}

// RegisterPermissions registers the adds_permissions declared in
// the plugin manifest. The host consults this on Enable + on
// every permission check; without it the keys would never be
// visible to operators.
func (p *CommTelegramPlugin) RegisterPermissions(reg plugin.AccessRegistry) error {
	// The single declared permission (channels.use_comm_telegram)
	// is registered by the host automatically from the manifest's
	// adds_permissions block, so this is a no-op for now. If we
	// ever add ABAC rules or dynamic scopes, wire them here.
	_ = reg
	return nil
}

// RegisterAdapters registers the Telegram client for its single
// ChannelType. The cfg map the host hands to Test/Send carries
// the channel row's columns (bot_token, chat_id, parse_mode,
// disable_notification) keyed by column name.
//
// SupportsRichAttachments is false in v1: Telegram inline
// keyboards + attachments are a follow-up — the host currently
// delivers Title + Message text only and Telegram's API renders
// them fine in either Markdown or HTML parse mode.
//
// Slug + Icon + Description are populated from the plugin's
// own manifest so the host's "Add Communication" picker can
// render this plugin as a tile via /api/plugins/loader/icon.
func (p *CommTelegramPlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
	return reg.Notification().Register(
		plugin.NotificationAdapterSpec{
			ChannelType: telegramChannelType,
			Slug:        p.manifest.Slug,
			DisplayName: "Telegram",
			Icon:        "plugin.svg",
			Description: p.manifest.Desc,
			Capabilities: plugin.NotificationAdapterCapabilities{
				EncryptedCredentials:    true,
				SupportsRichAttachments: false,
				// Telegram sendDocument takes a multipart document.
				SupportsFileAttachments: true,

				// The bot token stays the channel's; only the chat it posts into moves.
				// The bot must already be a member of the chat that is named.
				RecipientOverride: plugin.NotificationRecipientOverride{
					Key:           "chat_id",
					Kind:          "chat_id",
					LabelEN:       "Chat ID",
					PlaceholderEN: "-1001234567890",
					Multiple:      false,
				},
			},
			ConfigSchema: telegramConfigSchema(),
		},
		NewTelegramClient(),
	)
}

// telegramConfigSchema returns the form fields the admin UI
// renders when an operator picks this channel type.
//
// bot_token is the credential issued by @BotFather on Telegram.
// Treated as `secret` so the host encrypts it at rest via
// AES-256-GCM and renders the input as type=password.
//
// chat_id is the recipient identifier: numeric user/group ID or
// @channelname. The plugin passes it verbatim to sendMessage.
//
// parse_mode is optional — operators pick Markdown or HTML to
// enable rich formatting in Title/Message bodies. Empty string
// disables parse_mode and the body is rendered as plain text.
//
// disable_notification is optional — true silences the recipient
// notification (useful for low-priority alerts).
func telegramConfigSchema() []plugin.ConfigFieldDecl {
	return []plugin.ConfigFieldDecl{
		{
			Key:         "bot_token",
			Type:        "secret",
			Label:       "Bot Token",
			LabelKey:    "fields.botToken",
			Required:    true,
			Help:        "Bot API token issued by @BotFather on Telegram. Format: <bot_id>:<token>. Stored encrypted at rest.",
			HelpKey:     "config.bot_token.help",
			Placeholder: "0000000000:AAxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		},
		{
			Key:         "chat_id",
			Type:        "string",
			Label:       "Chat ID",
			LabelKey:    "fields.chatId",
			Required:    true,
			Help:        "Recipient chat identifier: numeric user/group ID or @channelname. Use getUpdates to discover a chat_id for private chats.",
			HelpKey:     "help.chatId",
			Placeholder: "123456789  or  @mychannel",
		},
		{
			Key:      "parse_mode",
			Type:     "string",
			Label:    "Parse Mode",
			LabelKey: "fields.parseMode",
			Required: false,
			Default:  "",
			Help:     "Optional. Markdown or HTML to enable rich formatting in Title + Message. Leave blank for plain text. The host does not pre-escape special characters — switch to plain text if your messages contain Telegram-reserved characters.",
			HelpKey:  "config.parse_mode.help",
		},
		{
			Key:      "disable_notification",
			Type:     "bool",
			Label:    "Silent Send",
			LabelKey: "fields.disableNotification",
			Required: false,
			Default:  "false",
			Help:     "Optional. When true, sends the message without a notification sound on the recipient device. Useful for low-priority alerts.",
			HelpKey:  "config.disable_notification.help",
		},
	}
}

// Init is called after RegisterAdapters. We stash the host
// context so the Telegram client can audit on Test + Send.
func (p *CommTelegramPlugin) Init(_ context.Context, host plugin.HostContext) error {
	p.host = host
	if host.Audit != nil {
		_ = host.Audit.Emit("plugin.comm-telegram.init", map[string]any{"slug": p.manifest.Slug})
	}
	return nil
}

// Shutdown is called on graceful stop or uninstall.
func (p *CommTelegramPlugin) Shutdown(_ context.Context) error {
	if p.host.Audit != nil {
		_ = p.host.Audit.Emit("plugin.comm-telegram.shutdown", nil)
	}
	return nil
}

// Compile-time check that we satisfy the plugin.Plugin interface.
var _ plugin.Plugin = (*CommTelegramPlugin)(nil)

// Plugin is the exported symbol `plugin.Open` looks up. It must be
// a variable (not a const) for `-buildmode=plugin` to find it.
var Plugin plugin.Plugin = &CommTelegramPlugin{
	manifest: plugin.Manifest{
		Schema:  "mcharbor.plugin/v1",
		Slug:    "comm-telegram",
		Name:    "Telegram",
		Version: "2026.10.01",
	},
}
