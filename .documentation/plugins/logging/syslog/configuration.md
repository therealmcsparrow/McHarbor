# Syslog: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- A collector listening on the chosen port: 6514 is the IANA port for syslog over TLS, 514 the traditional plaintext port.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `host` | string | Yes | — | Hostname or IP of the syslog collector. |
| `port` | int | Yes | — | 6514 is the IANA port for syslog over TLS; 514 is the traditional plaintext UDP/TCP port. |
| `protocol` | string | No | `tls` | One of tls, tcp, udp. TLS is the only option that both encrypts the audit trail in transit and confirms the collector accepted it. Prefer it unless your collector cannot terminate TLS. |
| `facility` | string | No | `local0` | Syslog facility: local0-local7, auth, authpriv, daemon, syslog, or user. Determines which file or ruleset the collector routes these events into. |
| `tag` | string | No | `mcharbor` | RFC5424 APP-NAME. Identifies McHarbor in the collector's stream; keep it stable so existing filters keep matching. |
| `max_retries` | int | No | — | How many times one event is retried before it is given up on. Delivery history keeps the failed attempts either way. |
| `timeout_seconds` | int | No | — | Connect and write timeout. A collector slower than this counts the attempt as failed and retries on the next cycle. |

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
| `db.migrations.run` | Create and migrate its own tables at install time. |
| `audit.emit` | Write entries into the tamper-evident audit trail. |

## Related

- [Syslog](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
