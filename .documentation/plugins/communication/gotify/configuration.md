# Gotify: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- A Gotify server reachable from the McHarbor container, and an **application** token (not a client token).

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `name` | string | Yes | — | Display name for this Gotify communication channel. |
| `server_url` | string | Yes | — | Base URL of the Gotify server. |
| `token` | secret | Yes | — | Gotify application token. Stored encrypted at rest. |
| `priority` | string | No | — | Optional Gotify priority integer. |

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

- [Gotify](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
