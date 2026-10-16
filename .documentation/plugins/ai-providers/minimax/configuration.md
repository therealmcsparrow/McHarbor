# MiniMax: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- An API key from the MiniMax platform console.
- Outbound HTTPS from the McHarbor container to the provider.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `api_key` | secret | Yes | — | API key from the MiniMax platform. Stored encrypted at rest. |
| `base_url` | string | No | — | Absolute HTTP or HTTPS OpenAI-compatible API base URL, including /v1. |
| `default_model` | string | No | — | Model used when a request does not select one explicitly. |

Secrets are encrypted with the host's AES-256-GCM service before they are written, and
the API never returns them once saved. Editing an existing entry leaves a stored secret
in place unless you replace it.

## Setting it up

1. Install the plugin from **App Store → Plugins**.
2. Open **Settings → Integrations → AI** and add a connection of this type.
3. Paste the key, or point the base URL at your own server.
4. Press **Test**, then pick a default model.

## Host scopes it requests

These are the capabilities the manifest asks the host for. The installer refuses a
manifest asking for a scope outside the known set.

| Scope token | What it allows |
|---|---|
| `ai.adapter.register` | Register an AI provider type with the host AI registry. |
| `encryption.handle.read` | Read the host encryption handle so it can decrypt the credentials it stored. |
| `audit.emit` | Write entries into the tamper-evident audit trail. |

## Related

- [MiniMax](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
