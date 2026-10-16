# WhatsApp

Delivers notifications to a WhatsApp number through one of four surfaces: the official Meta Cloud API, a self-hosted gateway, a WhatsApp Business API platform, or a third-party SaaS provider.

- **Slug** `comm-whatsapp`, **version** 2026.09.01, **channel type** `whatsapp`
- **Host range** `>=2026.09.01`
- **Where it appears** Settings → Integrations → Communications, as a channel type
- **Gated by** `communications.view` to see channels, `communications.manage` to add or change one, and `channels:use-comm-whatsapp` (global) to route events through this channel

## How it works

- `method` selects the field group that is read: Cloud uses `phone_number_id` and `access_token`; Gateway uses `gateway_url` and `gateway_token`; Bot platform uses `business_api_url` and `api_key`; SaaS uses `provider_url`, `provider_api_key` and `provider_from`. Fields belonging to the other methods are ignored.
- `recipient_phone` is E.164 and is used by every method.
- On the Meta Cloud API, a business can only send freely inside a 24-hour window opened by the user's own message. Outside it, only approved message templates deliver. That is a poor fit for unscheduled alerting, and the reason the other three options exist.
- **Attachments.** Only the Cloud API method carries a file. The plugin uploads it to the phone number media endpoint (`/<phone_number_id>/media`) and then sends it to the recipient as a document by the returned media id, one message per file, after the text. The gateway, bot and SaaS methods cannot, so a send that includes a file fails with "file attachments require the Cloud API connection method" instead of dropping it quietly.

## Capabilities it advertises

| Capability | Set | Meaning |
|---|---|---|
| `EncryptedCredentials` | Yes | The channel holds a secret, stored encrypted at rest and decrypted only at send time. |
| `SupportsRichAttachments` | No | Structured cards or blocks beyond title and message. |
| `SupportsFileAttachments` | Yes (Cloud API method only) | Files, such as a report export, can ride along with the message. |

## In this folder

| Page | What it covers |
|---|---|
| [Configuration](./configuration.md) | Prerequisites, every field, and the setup order |
| [Troubleshooting](./troubleshooting.md) | Symptoms, causes and fixes |
| [Developer](../../../development/plugins/communication/whatsapp.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Communication channels](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
