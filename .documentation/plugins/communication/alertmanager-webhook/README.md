# Alertmanager webhook

Makes McHarbor look like a Prometheus Alertmanager to any on-call tool that ingests Alertmanager webhooks. Every McHarbor alert condition becomes one Alertmanager alert in its own group, so a receiver that auto-resolves Alertmanager alerts opens exactly one incident per condition and closes it when McHarbor recovers.

- **Slug** `comm-alertmanager-webhook`, **version** 2026.09.01, **channel type** `alertmanager_webhook`
- **Host range** `>=2026.09.01`
- **Where it appears** Settings → Communications, as a channel type
- **Gated by** `communications.view` to see channels, `communications.manage` to add or change one, and `channels.use_comm_alertmanager_webhook` (global) to route events through this channel

## How it works

The plugin sends the Alertmanager **webhook payload version 4** (the `webhook_config` format), one alert per request.

| Field | Value | Why |
|---|---|---|
| `fingerprint` | First 16 hex characters of SHA-256 of the host's dedup key | The length and alphabet of a real Alertmanager fingerprint |
| `groupKey` | `{}:{mcharbor_dedup="<fingerprint>"}` | Receivers that group on `groupKey` see one group per condition |
| `groupLabels` | `{"mcharbor_dedup": "<fingerprint>"}` | Same reason |
| `status` | `firing` or `resolved`, on the group and on the alert | Receivers auto-resolve on `resolved` |
| `startsAt` | The alert's first firing time, fixed for its whole life | A new `startsAt` reads as a new alert at some receivers |
| `endsAt` | `0001-01-01T00:00:00Z` while firing, the recovery time once resolved | Alertmanager's own convention |
| `notification_reason` | `mcharbor` | Receivers ignore unknown fields |

Labels on every alert: `alertname`, `severity`, `mcharbor_dedup`, `instance`, `job` (`mcharbor`) and, when known, `environment`, plus the alert's own labels. A label name that is not `[a-zA-Z_][a-zA-Z0-9_]*` is rewritten with `_`. When two names collide after rewriting, the first in sorted order wins. Annotations are `summary` (the title) and `description` (the message, or the title when the message is empty).

- **Open** sends a firing alert and stores `<fingerprint>@<startsAt>` as the link's external ID. The plugin keeps no state of its own.
- **Update** sends the same firing alert again, which is what a real Alertmanager does every `repeat_interval`. The host sends it on a repeat firing unless **Re-send firing on repeat** is off.
- **Resolve** sends the same alert with `status: resolved` and `endsAt` set.
- **Plain notifications** (a backup report, a workflow notify node) go out as a firing `McHarborNotification` alert followed immediately by its resolve, so they never leave an incident open at the receiver.

### One-way by design

The Alertmanager protocol has no return channel, and every receiver's outbound webhook has its own payload and signing, when it signs at all. So the plugin implements neither an inbound webhook parser nor a poller.

- Acknowledging or closing the incident in the on-call tool does **not** reach McHarbor.
- Settings → Alerts → Tickets shows no remote state for these links. That is correct, not a fault.
- A link stays open until McHarbor itself resolves the alert.

Use a dedicated PagerDuty, incident.io, Rootly, Splunk On-Call or xMatters channel when acknowledgement sync matters.

### Compatibility

| Receiver | URL | What it groups on | Verified | Profile |
|---|---|---|---|---|
| Grafana Cloud IRM / Grafana OnCall ("Alertmanager" integration) | The OnCall Integration URL; **the trailing slash is required** | `groupKey`; auto-resolves on `resolved` | Yes | `grafana_irm`: refuses a URL whose path does not end in `/` |
| Squadcast (SolarWinds Incident Response), Prometheus alert source | The Webhook URL under Services → Alert Sources → Prometheus | Auto-resolves on `resolved`; requires `labels.severity`, `annotations.summary`, `annotations.description` | Partially | `squadcast`: all three are always sent, the description never empty |
| Zenduty (Xurrent IMR), Prometheus Alertmanager | `https://events.zenduty.com/integration/<id>/prometheus/<key>/` | **`alertname`** | Partially | `zenduty`: `alertname` becomes `<alert> [<first 8 of fingerprint>]` so two conditions never share an incident |
| Keep | `<Keep backend>/alerts/event/prometheus`, Basic auth `api_key:<key>` | Keep's own fingerprinting | Yes, for the URL and auth | `keep`: Basic auth with username `api_key` unless you set another |
| Any other Alertmanager v4 receiver | Whatever it gives you | Usually `groupKey` or `fingerprint` | — | `generic` |

Jira Service Management's "Prometheus" integration uses Alertmanager's `opsgenie_configs`, not the webhook format, and is not a profile. incident.io and PagerDuty have their own Alertmanager receivers and their own McHarbor plugins.

### Outbound safety

The receiver URL is operator-supplied, so the plugin treats it as untrusted:

- **Blocked destinations.** The connection is refused, after DNS resolution, when the address is loopback, link-local (`169.254.0.0/16`, which includes cloud metadata services, and `fe80::/10`), unspecified or multicast. A hostname that resolves to one of these is refused like the address itself. Private RFC 1918 ranges are allowed, because self-hosted receivers live there. No HTTP proxy from the environment is used.
- **No redirects.** A `3xx` answer is not followed and is a permanent failure, so credentials and the signature never travel to another host.
- **The URL is a secret.** Errors name only `scheme://host` and the HTTP status. The path and query, where receivers put their token, never appear, and the receiver's response body is never echoed.
- **Plain `http://`** is refused unless **Allow plain http** is on.

### Signed requests for custom gateways

When **HMAC signing secret** is set, every request carries:

```
X-McHarbor-Signature: t=<unix seconds>,v1=<hex HMAC-SHA256 of "<t>.<raw body>">
```

A gateway recomputes the HMAC over the timestamp, a dot and the exact request body, compares it in constant time, and rejects a timestamp older than its replay window.

## Capabilities it advertises

| Capability | Set | Meaning |
|---|---|---|
| `EncryptedCredentials` | Yes | The channel holds secrets, stored encrypted at rest and decrypted only at send time. |
| `SupportsRichAttachments` | No | Structured cards or blocks beyond title and message. |
| `SupportsFileAttachments` | No | Files cannot ride along with the message. |
| Incident lifecycle | Open, update, resolve | The host opens and resolves one alert per condition through it. |
| Inbound updates / polling | No | One-way; see above. |

## In this folder

| Page | What it covers |
|---|---|
| [Configuration](./configuration.md) | Prerequisites, every field, the setup order per receiver |
| [Troubleshooting](./troubleshooting.md) | Symptoms, causes and fixes |

## Related

- [Communication channels](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
