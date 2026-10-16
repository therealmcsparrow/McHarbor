# Signal

Delivers notifications to Signal numbers through one of four transports: the official `signal-cli-rest-api`, a token-based bot server, a `signald` JSON-RPC daemon, or an unauthenticated community relay. Signal has no official server-side API, so all four are gateways you run or trust yourself.

- **Slug** `comm-signal`, **version** 2026.09.01, **channel type** `signal`
- **Host range** `>=2026.09.01`
- **Where it appears** Settings → Integrations → Communications, as a channel type
- **Gated by** `communications.view` to see channels, `communications.manage` to add or change one, and `channels:use-comm-signal` (global) to route events through this channel

## How it works

- `method` decides which of the remaining fields are read: REST uses `auth_username`/`auth_password`, Bot uses `bot_token`, signald uses `account_id`.
- `sender_number` and every entry in `recipients` are E.164 (`+14155551234`). The plugin splits `recipients` on commas and sends one message per recipient.
- **Attachments.** A report export or other file rides along only over the REST method: it is sent in `base64_attachments` as data URIs. The bot, signald and simple methods cannot carry a file, so a send that includes one fails with "file attachments require the Signal REST API connection method" instead of dropping the file quietly.

## Capabilities it advertises

| Capability | Set | Meaning |
|---|---|---|
| `EncryptedCredentials` | Yes | The channel holds a secret, stored encrypted at rest and decrypted only at send time. |
| `SupportsRichAttachments` | No | Structured cards or blocks beyond title and message. |
| `SupportsFileAttachments` | Yes (REST method only) | Files, such as a report export, can ride along with the message. |

## In this folder

| Page | What it covers |
|---|---|
| [Configuration](./configuration.md) | Prerequisites, every field, and the setup order |
| [Troubleshooting](./troubleshooting.md) | Symptoms, causes and fixes |
| [Developer](../../../development/plugins/communication/signal.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Communication channels](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
