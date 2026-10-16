# SMTP: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- A relay that accepts mail from the McHarbor host and, unless the relay is open on an internal network, credentials for it.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `name` | string | Yes | — | Display name for this SMTP communication channel. |
| `host` | string | Yes | — | SMTP relay hostname or IP address. |
| `port` | string | Yes | `587` | SMTP port. Use 587 for STARTTLS, 465 for implicit TLS, or 25 for plain/internal relays. |
| `encryption` | string | Yes | `starttls` | One of starttls, ssl_tls, or none. STARTTLS is recommended for port 587. |
| `auth_method` | string | Yes | `plain` | One of none, plain, login, or cram_md5. |
| `username` | string | No | — | SMTP AUTH username. Leave blank when authentication is none. |
| `password` | secret | No | — | SMTP AUTH password or app password. Stored encrypted at rest. |
| `from_address` | string | Yes | — | Envelope sender and From header. |
| `from_name` | string | No | — | Optional display name for the From header. |
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

- [SMTP](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
