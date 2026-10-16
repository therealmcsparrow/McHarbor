# ntfy: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- A topic name and, on a protected server, either a topic-scoped access token or Basic credentials.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `server_url` | string | Yes | `https://ntfy.sh` | Base URL of your ntfy server. Defaults to https://ntfy.sh. Change to your self-hosted instance (e.g. https://ntfy.example.com) for on-prem setups. |
| `topic` | string | Yes | — | Topic name subscribers receive on the ntfy app. Use a unique name (e.g. mcharbor-alerts-prod). ntfy.sh allows up to 64 chars of [a-zA-Z0-9_-]. |
| `access_token` | secret | No | — | Topic-scoped token from ntfy. Click 'Get token' on the topic page in the ntfy web UI. Takes precedence over username/password when set. Encrypted at rest. |
| `username` | string | No | — | HTTP Basic auth username for self-hosted ntfy. Used only when access_token is blank. |
| `password` | secret | No | — | HTTP Basic auth password for self-hosted ntfy. Used only when access_token is blank. Encrypted at rest. |
| `priority` | string | No | — | Optional. One of min, low, default, high, urgent. Maps to ntfy's integer priority 1/2/3/4/5. Leave blank for server default. |
| `tags` | string | No | — | Optional. Comma-separated emoji shortcodes rendered as inline icons in the ntfy mobile app (e.g. 'warning,rocket'). See https://docs.ntfy.sh/emojis/ for the catalog. |

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

- [ntfy](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
