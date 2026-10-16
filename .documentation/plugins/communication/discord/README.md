# Discord

Delivers workflow, alert and backup notifications into a Discord channel through a channel webhook.

- **Slug** `comm-discord`, **version** 2026.09.01, **channel type** `discord`
- **Host range** `>=2026.09.01`
- **Where it appears** Settings → Integrations → Communications, as a channel type
- **Gated by** `communications.view` to see channels, `communications.manage` to add or change one, and `channels:use-comm-discord` (global) to route events through this channel

## How it works

- Both `discord.com/api/webhooks/...` and the older `discordapp.com` host are accepted.
- Unlike Slack, Discord webhooks do accept file uploads, so notifications that carry a file deliver it.

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
| [Developer](../../../development/plugins/communication/discord.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Communication channels](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
