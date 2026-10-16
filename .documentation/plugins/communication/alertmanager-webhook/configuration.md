# Alertmanager webhook: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- An integration in your on-call tool that accepts Prometheus Alertmanager webhooks, and the URL it gives you.
- For a receiver that does not put its token in the URL, a bearer token or Basic credentials.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `name` | string | Yes | — | Channel name shown in McHarbor. |
| `webhook_url` | secret | Yes | — | **Receiver URL.** The Alertmanager webhook URL from the receiver. It usually contains a token, so it is stored encrypted and never returned by the API. |
| `profile` | string | No | `generic` | **Receiver profile.** One of `generic`, `grafana_irm`, `squadcast`, `zenduty`, `keep`. Adjusts validation and payload details per receiver; see the table below. |
| `receiver_name` | string | No | `mcharbor` | Sent as the payload's `receiver` field. |
| `auth_mode` | string | No | `none` (`basic` on the `keep` profile) | **Authentication.** `none` (token in the URL), `bearer` or `basic`. |
| `bearer_token` | secret | With `bearer` | — | Sent as `Authorization: Bearer <token>`. |
| `basic_username` | string | With `basic` | `api_key` on the `keep` profile | Basic auth username. |
| `basic_password` | secret | No | — | Basic auth password, or the Keep API key. |
| `hmac_secret` | secret | No | — | Optional. Adds `X-McHarbor-Signature: t=<unix>,v1=<hex>` to every request, for custom gateways that verify it. |
| `allow_http` | bool | No | `false` | **Allow plain http.** Only for a receiver on a trusted private network, such as self-hosted Keep. |
| `update_on_repeat` | bool | No | `true` | **Re-send firing on repeat.** When an alert fires again while it is open, send the same alert again, as Alertmanager does on its repeat interval. |
| `test_sends_alert` | bool | No | `false` | **Test connection sends a test alert.** When on, Test fires and immediately resolves an info alert named `McHarborConnectionTest`. Off by default because it can page someone. |
| `severity_map` | string | No | — | **Severity map (JSON).** Renames the `severity` label values. Keys must be `critical`, `warning` or `info`, values must be non-empty, for example `{"critical":"P1"}`. |

Secrets are encrypted with the host's AES-256-GCM service before they are written, and
the API never returns them once saved. Editing an existing channel leaves a stored secret
in place unless you type a new one.

### Profiles

| Profile | What changes |
|---|---|
| `generic` | Nothing; standard Alertmanager webhook v4. |
| `grafana_irm` | The URL's path must end in `/`, or the channel is refused with "Grafana IRM integration URLs end in /". |
| `squadcast` | No change needed: `labels.severity`, `annotations.summary` and a non-empty `annotations.description` are always sent. |
| `zenduty` | `alertname` is suffixed with the first 8 characters of the fingerprint, because Zenduty groups by `alertname`. |
| `keep` | `auth_mode` defaults to `basic` and `basic_username` to `api_key`. |

### Severity

| McHarbor | `labels.severity` |
|---|---|
| `critical` | `critical` |
| `warning` | `warning` |
| `info` | `info` |

These are Prometheus convention values. Receivers map them with their own label rules; use `severity_map` when yours expects other values.

## Setting it up

1. Install the plugin from **App Store → Plugins**.
2. Create the integration in your on-call tool (per receiver below) and copy its URL.
3. Open **Settings → Communications**, add a channel of type **Alertmanager webhook**, choose the profile, paste the URL and save.
4. Press **Test connection**. By default it resolves the host, connects and, for `https`, completes the TLS handshake. It sends nothing. Turn on **Test connection sends a test alert** to send a real firing-then-resolved alert instead.
5. Grant `channels.use_comm_alertmanager_webhook` to the roles allowed to route events here.
6. Point an alert rule at this channel. Settings → Alerts → Tickets lists every alert McHarbor opened through it.

The channel's **Inbound updates** section does not apply: this plugin has no inbound webhook.

### Grafana Cloud IRM / Grafana OnCall

1. Open IRM (OnCall) → Integrations → New integration and choose **Alertmanager Prometheus**.
2. Copy the **OnCall Integration URL** from the HTTP Endpoint section, including the trailing slash.
3. Keep the default grouping and autoresolution templates; they match one group per condition.

### Squadcast

1. Open Services → your service → Alert Sources → Add, and choose **Prometheus**.
2. Copy the Webhook URL. Recoveries in McHarbor resolve the Squadcast incident.

### Zenduty (Xurrent IMR)

1. Open your team's service → Integrations → Add Integration, and choose **Prometheus**.
2. Copy the webhook URL, of the form `https://events.zenduty.com/integration/<id>/prometheus/<key>/`.
3. Use the `zenduty` profile so two conditions of the same rule never share an incident.

### Keep

1. Create an API key in Keep.
2. Set the URL to `<your Keep backend>/alerts/event/prometheus` and use the `keep` profile; put the API key in **Password**.
3. If Keep runs on your private network over `http`, turn on **Allow plain http**.

## Host scopes it requests

These are the capabilities the manifest asks the host for. The installer refuses a
manifest asking for a scope outside the known set.

| Scope token | What it allows |
|---|---|
| `notification.adapter.register` | Register a channel type with the host communications registry. |
| `encryption.handle.read` | Read the host encryption handle so it can decrypt the credentials it stored. |
| `audit.emit` | Write entries into the tamper-evident audit trail. |

## Related

- [Alertmanager webhook](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
