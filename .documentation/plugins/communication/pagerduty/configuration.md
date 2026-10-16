# PagerDuty: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- A PagerDuty service McHarbor alerts should page, and permission to add an integration to it.
- Optional: a read-only REST API key, for polling and for incident numbers and links.
- Optional: permission to create a Generic Webhook (v3) subscription, for status coming back
  without polling.
- Which service region your account is on. An account on the EU region signs in on an
  `eu.pagerduty.com` address.

## Fields

| Field | Type | Required | Default | What it is |
|---|---|---|---|---|
| `name` | string | Yes | — | Channel name shown in McHarbor. |
| `region` | string | Yes | `us` | `us` or `eu`. Picks the Events and REST hosts; must match the region the account lives in. |
| `routing_key` | secret | Yes | — | The 32-character Integration Key from the service's **Events API V2** integration. |
| `rest_api_key` | secret | No | — | A read-only REST API key. Enables polling, the incident number and the link. Sent as `Authorization: Token token=…`. |
| `webhook_signing_secret` | secret | No | — | The secret PagerDuty shows once when the v3 webhook subscription is created. Takes precedence over a signing secret stored under *Ticket status from the ITSM*. |
| `update_on_repeat` | bool | No | `true` | When the alert fires again while its incident is open, send another trigger on the same key. |
| `poll_interval_minutes` | int | No | `5` | 0 to 60. `0` turns polling off. Polling needs `rest_api_key`; without one the channel does not poll. |
| `severity_map` | string | No | — | JSON object remapping McHarbor severities, for example `{"warning":"error"}`. Keys are `critical`, `warning`, `info`; values are `critical`, `error`, `warning`, `info`. Anything else fails the test. |

Secrets are encrypted with the host's AES-256-GCM service before they are written, and
the API never returns them once saved. Editing an existing channel leaves a stored secret
in place unless you replace it. None of the three secrets ever appears in an error message,
a log line or an audit field.

## Setting it up

### In PagerDuty

1. Open **Services → Service Directory** and select (or create) the service McHarbor should
   page.
2. On the service, open **Integrations**, choose **Add an integration**, pick
   **Events API V2** and save. Copy the **Integration Key**.
3. Optional: under **Integrations → API Access Keys**, create a **read-only** API key.

### In McHarbor

4. Install the plugin from **App Store → Plugins**.
5. Open **Settings → Integrations → Communications**, add a channel of type **PagerDuty**,
   pick the region, paste the Integration Key into **Integration (routing) key** and, if you
   made one, the REST key into **REST API key (read-only)**.
6. Save and press **Test connection**. Nothing is paged; see
   [Troubleshooting](./troubleshooting.md) if it fails.
7. Edit the channel again. Under **Ticket status from the ITSM**, select
   **Create inbound URL** and copy the URL, which ends in `/api/incident-webhooks/` and a
   token. The signing secret shown beside it is not used by PagerDuty. This needs
   `communications.manage`.

### Back in PagerDuty

8. Open **Integrations → Generic Webhooks (v3) → New Webhook**. Paste the inbound URL,
   scope the subscription to the same service, and subscribe to `incident.triggered`,
   `incident.acknowledged`, `incident.unacknowledged`, `incident.resolved` and
   `incident.reopened`.
9. Copy the signing secret PagerDuty shows once, and paste it into **Webhook signing
   secret** on the channel (or into **Signing secret issued by the ITSM**).
10. Use **Send Test Event** on the PagerDuty integration page to prove the routing key.

### Routing

11. Grant `channels.use_comm_pagerduty` to the roles allowed to route events here.
12. Select the channel on an alert rule, or in an Open, Update or Resolve Ticket workflow
    node.

## Host scopes it requests

These are the capabilities the manifest asks the host for. The installer refuses a
manifest asking for a scope outside the known set.

| Scope token | What it allows |
|---|---|
| `notification.adapter.register` | Register a channel type with the host communications registry. |
| `encryption.handle.read` | Read the host encryption handle so it can decrypt the credentials it stored. |
| `audit.emit` | Write entries into the tamper-evident audit trail. |

## Outbound endpoints

| Host | Used for |
|---|---|
| `events.pagerduty.com` / `events.eu.pagerduty.com` | `POST /v2/enqueue`: trigger and resolve |
| `api.pagerduty.com` / `api.eu.pagerduty.com` | `GET /abilities` (test) and `GET /incidents` (poll), only with a REST key |

## Related

- [PagerDuty](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
