# Microsoft Teams

Delivers notifications into a Microsoft Teams channel as an Adaptive Card (the title as a bold header, the message as a wrapping body) through an Incoming Webhook connector.

- **Slug** `comm-teams`, **version** 2026.09.01, **channel type** `teams`
- **Host range** `>=2026.09.01`
- **Where it appears** Settings → Integrations → Communications, as a channel type
- **Gated by** `communications.view` to see channels, `communications.manage` to add or change one, and `channels:use-comm-teams` (global) to route events through this channel

## How it works

- `theme_color` is a hex value **without** the `#`, used for the card's accent stripe. It is the cheapest way to tell environments apart at a glance.
- This is the only channel in the family that does **not** advertise `EncryptedCredentials`: the connector URL is stored as a plain configuration string rather than as a secret. Treat the URL as sensitive anyway: it is a post-anything token for that channel.
- Microsoft has been retiring Office 365 connectors in stages. If sends start failing on a tenant that used to work, check whether connectors are still enabled there before debugging McHarbor.

## Capabilities it advertises

| Capability | Set | Meaning |
|---|---|---|
| `EncryptedCredentials` | No | The channel holds a secret, stored encrypted at rest and decrypted only at send time. |
| `SupportsRichAttachments` | No | Structured cards or blocks beyond title and message. |
| `SupportsFileAttachments` | No | Files, such as a report export, can ride along with the message. |

## In this folder

| Page | What it covers |
|---|---|
| [Configuration](./configuration.md) | Prerequisites, every field, and the setup order |
| [Troubleshooting](./troubleshooting.md) | Symptoms, causes and fixes |
| [Developer](../../../development/plugins/communication/microsoft-teams.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Communication channels](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
