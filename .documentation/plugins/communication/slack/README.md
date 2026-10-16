# Slack

Delivers workflow, alert and backup notifications into a Slack channel through an incoming webhook. One webhook is one channel; the channel is chosen in Slack when the webhook is created, not here.

- **Slug** `comm-slack`, **version** 2026.09.01, **channel type** `slack`
- **Host range** `>=2026.09.01`
- **Where it appears** Settings → Integrations → Communications, as a channel type
- **Gated by** `communications.view` to see channels, `communications.manage` to add or change one, and `channels:use-comm-slack` (global) to route events through this channel

## How it works

- The webhook URL is the credential *and* the destination. Anyone holding it can post to that channel, which is why it is stored as a secret and never returned by the API.
- No file attachments. Incoming webhooks cannot upload files; that needs a bot token and `files.upload`, which this channel type does not carry. A notification with an attachment is delivered as text.

## Capabilities it advertises

| Capability | Set | Meaning |
|---|---|---|
| `EncryptedCredentials` | Yes | The channel holds a secret, stored encrypted at rest and decrypted only at send time. |
| `SupportsRichAttachments` | No | Structured cards or blocks beyond title and message. |
| `SupportsFileAttachments` | No | Files, such as a report export, can ride along with the message. |

## In this folder

| Page | What it covers |
|---|---|
| [Configuration](./configuration.md) | Prerequisites, every field, and the setup order |
| [Troubleshooting](./troubleshooting.md) | Symptoms, causes and fixes |
| [Developer](../../../development/plugins/communication/slack.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Communication channels](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
