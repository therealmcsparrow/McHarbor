# Exchange

Sends notifications as email through Microsoft Graph (`/users/{upn}/sendMail`) using a client-credentials token: an application identity, with no signed-in user.

- **Slug** `comm-exchange`, **version** 2026.09.01, **channel type** `exchange`
- **Host range** `>=2026.09.01`
- **Where it appears** Settings → Integrations → Communications, as a channel type
- **Gated by** `communications.view` to see channels, `communications.manage` to add or change one, and `channels:use-comm-exchange` (global) to route events through this channel

## How it works

- Tokens come from `login.microsoftonline.com/{tenant}/oauth2/v2.0/token` with the `https://graph.microsoft.com/.default` scope. There is no interactive consent step.
- `Mail.Send` as an application permission lets the app send as **any** mailbox in the tenant unless an application access policy narrows it. Narrow it with `New-ApplicationAccessPolicy`, scoped to a mail-enabled security group containing only the notification mailbox.
- Large attachments use Graph's `createUploadSession` flow.

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
| [Developer](../../../development/plugins/communication/exchange.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Communication channels](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
