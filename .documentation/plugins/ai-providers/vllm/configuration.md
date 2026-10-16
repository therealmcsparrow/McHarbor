# vLLM: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- A running vLLM instance reachable from the McHarbor container.
- A model loaded on that server. The plugin lists what the server reports; it does not pull models for you.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `api_key` | secret | No | — | Optional bearer token for an authenticated vLLM (self-hosted) endpoint. Stored encrypted at rest. |
| `base_url` | string | Yes | — | Absolute HTTP or HTTPS API root. The plugin appends /models and /chat/completions. |
| `default_model` | string | No | — | Model used when a request does not select one explicitly. |

For a local provider, `localhost` inside the McHarbor container means the container
itself. Use the machine's LAN address, or `host.docker.internal` on Docker Desktop.

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

- [vLLM](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
