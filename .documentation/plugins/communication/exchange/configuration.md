# Exchange: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- A Microsoft Entra app registration with the **application** permission `Mail.Send`, admin-consented.
- A client secret, and the UPN of the mailbox that will send.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `name` | string | Yes | — | Display name for this Exchange Online communication channel. |
| `tenant_id` | string | Yes | — | Microsoft Entra tenant ID used for the client-credentials token request. |
| `client_id` | string | Yes | — | Microsoft Entra app registration client ID with Microsoft Graph Mail.Send permission. |
| `client_secret` | secret | Yes | — | Microsoft Entra client secret. Stored encrypted at rest. |
| `from_address` | string | Yes | — | Exchange Online mailbox used as the Graph sendMail user. |
| `from_name` | string | No | — | Optional display name retained for compatibility with other email channels. |
| `to` | string | Yes | — | Comma-separated recipient email addresses. |
| `subject_prefix` | string | No | `[McHarbor]` | Optional prefix added to every notification subject. |

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

- [Exchange](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
