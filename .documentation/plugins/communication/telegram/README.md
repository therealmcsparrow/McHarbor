# Telegram

Delivers notifications to a Telegram chat through the Bot API, using a bot token and a chat id.

- **Slug** `comm-telegram`, **version** 2026.09.01, **channel type** `telegram`
- **Host range** `>=2026.09.01`
- **Where it appears** Settings → Integrations → Communications, as a channel type
- **Gated by** `communications.view` to see channels, `communications.manage` to add or change one, and `channels:use-comm-telegram` (global) to route events through this channel

## How it works

- The token has the form `<bot_id>:<token>` and is stored encrypted.
- To find a private chat id, message the bot and read `https://api.telegram.org/bot<token>/getUpdates`. Channels take `@channelname` instead.
- `parse_mode` enables Markdown or HTML formatting. The host does **not** pre-escape message text, so a message containing `_` or `*` can fail to parse. Leave the mode blank if messages carry arbitrary text such as container names.
- `disable_notification` sends silently, which suits a high-volume low-priority channel.

## Capabilities it advertises

| Capability | Set | Meaning |
|---|---|---|
| `EncryptedCredentials` | Yes | The channel holds a secret, stored encrypted at rest and decrypted only at send time. |
| `SupportsRichAttachments` | No | Structured cards or blocks beyond title and message. |
| `SupportsFileAttachments` | Yes | Files, such as a report export, can ride along with the message. |

## In this folder

| Page | What it covers |
|---|---|
| [Configuration](./configuration.md) | Prerequisites, every field, and the setup order |
| [Troubleshooting](./troubleshooting.md) | Symptoms, causes and fixes |
| [Developer](../../../development/plugins/communication/telegram.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Communication channels](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
