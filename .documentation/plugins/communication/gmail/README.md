# Gmail

Sends notifications as email through the Gmail API rather than SMTP, using an OAuth client with the `gmail.send` scope.

- **Slug** `comm-gmail`, **version** 2026.09.01, **channel type** `gmail`
- **Host range** `>=2026.09.01`
- **Where it appears** Settings → Integrations → Communications, as a channel type
- **Gated by** `communications.view` to see channels, `communications.manage` to add or change one, and `channels:use-comm-gmail` (global) to route events through this channel

## How it works

- The only scope requested is `https://www.googleapis.com/auth/gmail.send`: send, not read.
- Mail is posted to `gmail.googleapis.com/gmail/v1/users/{from}/messages/send`, so it appears in the sending account's Sent folder.
- `from_address` must be the authorising account (or an alias it is allowed to send as).
- If you only need plain mail delivery, [SMTP](../smtp/README.md) is less setup. Use this one when the organisation has disabled SMTP app passwords.

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
| [Developer](../../../development/plugins/communication/gmail.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Communication channels](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
