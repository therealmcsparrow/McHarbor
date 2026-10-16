# Signal: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- A running gateway, and a Signal account registered on it to send from.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `method` | string | Yes | — | Which Signal transport to use. REST = official signal-cli-rest-api. Bot = token-based bot server. signald = signald daemon JSON-RPC. Simple = no-auth community relay. |
| `server_url` | string | Yes | — | Base URL of the Signal gateway / daemon. Example: https://signal.example.com or http://localhost:8080. |
| `sender_number` | string | Yes | — | Registered Signal account number in E.164 format. The bot's outbound identity. Used by every method. |
| `recipients` | string | Yes | — | Comma-separated list of recipient numbers in E.164 format. The plugin splits on commas and sends one Signal message per recipient. |
| `auth_username` | string | No | — | Optional. HTTP Basic auth username for signal-cli-rest-api. Leave blank if the daemon is unauthenticated. |
| `auth_password` | secret | No | — | Optional. HTTP Basic auth password for signal-cli-rest-api. Stored encrypted at rest. |
| `bot_token` | secret | No | — | Token-based Bot only. Bearer token for the Signal bot server. Issued when you register the bot with the platform. Stored encrypted at rest. |
| `account_id` | string | No | — | signald method only. signald account identifier: either the E.164 phone string or the UUID assigned at `signaldctl account register`. The plugin uses it in the JSON-RPC account path. |

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

- [Signal](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
