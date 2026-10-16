# SMTP

Sends notifications as email through an SMTP relay. This is the channel to use when the target is a mailbox, a distribution list or a ticketing address rather than a chat product.

- **Slug** `comm-smtp`, **version** 2026.09.01, **channel type** `smtp`
- **Host range** `>=2026.09.01`
- **Where it appears** Settings → Integrations → Communications, as a channel type
- **Gated by** `communications.view` to see channels, `communications.manage` to add or change one, and `channels:use-comm-smtp` (global) to route events through this channel

## How it works

- Port and encryption belong together: 587 with `starttls`, 465 with `ssl_tls`, 25 with `none` on an internal relay.
- `auth_method` is `none`, `plain`, `login` or `cram_md5`. Set it to `none` and leave the credentials blank for an internal relay that authenticates by IP.
- `to` is a comma-separated list, and `subject_prefix` defaults to `[McHarbor]` so mail rules can match on it.
- Attachments are supported, which makes this a workable delivery target for scheduled report exports.

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
| [Developer](../../../development/plugins/communication/smtp.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Communication channels](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
