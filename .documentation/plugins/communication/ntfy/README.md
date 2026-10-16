# ntfy

Publishes notifications to an ntfy topic, either on `ntfy.sh` or on your own server. The message is a POST to `<server_url>/<topic>` with the title, priority and tags as headers.

- **Slug** `comm-ntfy`, **version** 2026.09.01, **channel type** `ntfy`
- **Host range** `>=2026.09.01`
- **Where it appears** Settings → Integrations → Communications, as a channel type
- **Gated by** `communications.view` to see channels, `communications.manage` to add or change one, and `channels:use-comm-ntfy` (global) to route events through this channel

## How it works

- On public `ntfy.sh`, **a topic is a secret**. Anyone who guesses the name can read the notifications, so use something unguessable such as `mcharbor-alerts-7f2a91`.
- `access_token` takes precedence over `username`/`password` when both are filled in.
- `priority` accepts `min`, `low`, `default`, `high`, `urgent` and maps to ntfy's 1 to 5.
- `tags` are emoji shortcodes rendered as icons in the mobile app.

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
| [Developer](../../../development/plugins/communication/ntfy.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Communication channels](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
