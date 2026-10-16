# WhatsApp: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- Whichever of the four you intend to use, already working and able to send to the target number.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `method` | string | Yes | — | Which WhatsApp connection surface to use. Cloud = official Meta Cloud API. Gateway = your own Baileys/wa-gateway server. Bot = WhatsApp Business API platform (360dialog/Twilio-style). SaaS = a third-party messaging provider. |
| `recipient_phone` | string | Yes | — | Recipient phone number in E.164 format (e.g. +14155551234). Used by every method. |
| `phone_number_id` | string | No | — | Meta Cloud API only. The numeric Phone Number ID from the Meta Business Manager (WhatsApp > Phone Numbers). Used to build /v21.0/<phone_number_id>/messages. |
| `access_token` | secret | No | — | Meta Cloud API only. Permanent system-user access token from the Meta Business Manager. Stored encrypted at rest. |
| `gateway_url` | string | No | — | Self-hosted gateway only. Base URL of your own WhatsApp gateway (Baileys, wa-gateway, etc.). The plugin POSTs {title, message, phone} to <gateway_url>/send. |
| `gateway_token` | secret | No | — | Self-hosted gateway only. Bearer / API-key token presented as Authorization: Bearer <token>. Stored encrypted at rest. |
| `business_api_url` | string | No | — | Business Bot Platform only. Full endpoint of the platform's send-message route (e.g. https://waba.360dialog.com/v1/messages or your Twilio WhatsApp API base). |
| `api_key` | secret | No | — | Business Bot Platform only. Platform API key sent in the D360-API-KEY header (or X-API-Key, depending on the platform). Stored encrypted at rest. |
| `provider_url` | string | No | — | Third-party SaaS only. Base URL of the provider's send-message endpoint. The plugin POSTs {to, from, body} to <provider_url>/messages. |
| `provider_api_key` | secret | No | — | Third-party SaaS only. Provider API key (or Account SID + Auth Token for Twilio). Stored encrypted at rest. |
| `provider_from` | string | No | — | Third-party SaaS only. Sender identifier expected by the provider (Twilio WhatsApp 'From' number, MessageBird originator, Gupshup app id, etc.). |

Secrets are encrypted with the host's AES-256-GCM service before they are written, and
the API never returns them once saved. Editing an existing entry leaves a stored secret
in place unless you replace it.

## Setting it up

1. Install the plugin from **App Store → Plugins**.
2. Open **Settings → Integrations → Communications** and add a channel of this type.
3. Fill in the fields above and press **Send test notification**. This delivers a real message.
4. Grant the channel permission key to the roles allowed to route events here.
5. Select the channel on an alert rule, a backup plan, or a workflow notify node.

## Host scopes it requests

These are the capabilities the manifest asks the host for. The installer refuses a
manifest asking for a scope outside the known set.

| Scope token | What it allows |
|---|---|
| `notification.adapter.register` | Register a channel type with the host communications registry. |
| `encryption.handle.read` | Read the host encryption handle so it can decrypt the credentials it stored. |
| `audit.emit` | Write entries into the tamper-evident audit trail. |

## Related

- [WhatsApp](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
