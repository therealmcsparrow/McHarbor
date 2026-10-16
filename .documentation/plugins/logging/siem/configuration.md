# SIEM: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- A collector endpoint that accepts a JSON POST, and a shared secret both sides agree on.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `endpoint_url` | string | Yes | — | HTTPS endpoint that accepts a JSON POST body. Splunk HEC, Microsoft Sentinel, Elastic, or any collector that returns 2xx on success. |
| `signing_secret` | secret | Yes | — | Shared secret used to compute the X-McHarbor-Signature HMAC-SHA256 header, so the collector can verify each event came from this McHarbor. Encrypted at rest. Leave blank when saving to keep the stored value. |
| `max_retries` | int | No | — | How many times one event is retried before it is given up on. Delivery history keeps the failed attempts either way. |
| `timeout_seconds` | int | No | — | Per-request timeout. A collector slower than this counts the attempt as failed and retries on the next cycle. |

Secrets are encrypted with the host's AES-256-GCM service before they are written, and
the API never returns them once saved. Editing an existing entry leaves a stored secret
in place unless you replace it.

## Setting it up

1. Install the plugin from **App Store → Plugins**.
2. Open **Settings → Integrations → Logging** and add a forwarder.
3. Fill in the endpoint and credential, then press **Test**.
4. Watch delivery history after the first real events to confirm the collector is accepting them.

## Host scopes it requests

These are the capabilities the manifest asks the host for. The installer refuses a
manifest asking for a scope outside the known set.

| Scope token | What it allows |
|---|---|
| `logging.adapter.register` | Register an audit-log forwarder with the host logging registry. |
| `reporting.adapter.register` | Contribute its delivery report to the Reporting plugin's Report Studio. |
| `encryption.handle.read` | Read the host encryption handle so it can decrypt the credentials it stored. |
| `db.migrations.run` | Create and migrate its own tables at install time. |
| `audit.emit` | Write entries into the tamper-evident audit trail. |

## Related

- [SIEM](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
