# Microsoft Teams: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- An Incoming Webhook connector on the target Teams channel: **Channel menu (…) → Connectors → Incoming Webhook → Configure**.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `webhook_url` | string | Yes | — | Incoming Webhook connector URL from Microsoft Teams. Channel menu (...) → Connectors → Incoming Webhook → Configure → copy the URL. The UUID in the URL is the auth credential; treat it as a secret. |
| `theme_color` | string | No | `5059C9` | Optional. Hex color (no #) used for the Adaptive Card's left accent stripe. Defaults to Teams brand purple. Useful for color-coding by environment or severity. |

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
| `audit.emit` | Write entries into the tamper-evident audit trail. |

## Related

- [Microsoft Teams](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
