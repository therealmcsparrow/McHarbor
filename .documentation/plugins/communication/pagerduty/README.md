# PagerDuty

Opens, re-notifies and resolves PagerDuty incidents for McHarbor alerts, and reads
acknowledge and resolve back. Writes go through PagerDuty's **Events API v2**; status comes
back through signed **Webhooks v3**, or through the REST API when a read-only key is given.

- **Slug** `comm-pagerduty`, **version** 2026.09.01, **channel type** `pagerduty`
- **Host range** `>=2026.09.01`
- **Where it appears** Settings → Integrations → Communications, as a channel type; its
  incidents are listed under Settings → Monitoring → Alerts → Tickets
- **Gated by** `communications.view` to see channels, `communications.manage` to add or
  change one or to create its inbound URL, and `channels.use_comm_pagerduty` (global) to
  route events through this channel

## How it works

- **Events API v2, not REST, for writes.** A trigger needs only the service integration's
  routing key, not a user-scoped REST token. PagerDuty deduplicates natively on
  `dedup_key`, and the event runs through the service's event rules, suppression and
  grouping like any other monitoring tool.
- **The ticket key is the dedup key.** McHarbor's ticket key
  (`<alertId>:<environmentId>:<subject>`) is sent verbatim as `dedup_key`. A retried open
  after a lost response lands on the same open alert, so it never creates a second
  incident. The Events API returns no incident ID, so the incident number and link appear
  on the Tickets tab only once a webhook or a poll has reported them.
- **Open** sends `trigger`. **Update** (an alert firing again while its incident is open)
  sends another `trigger` on the same key, which PagerDuty adds to the alert's log.
  **Resolve** sends `resolve`; a resolve for an already-resolved key is dropped by
  PagerDuty and still succeeds.
- **Severity** maps `critical` → `critical`, `warning` → `warning`, `info` → `info`.
  `severity_map` may remap any of the three to PagerDuty's fourth value, `error`.
- **Region.** PagerDuty has a US and an EU service region. The region field picks both
  hosts: `events.pagerduty.com` / `api.pagerduty.com` or `events.eu.pagerduty.com` /
  `api.eu.pagerduty.com`. No base URL can be typed in.
- **Test never pages anyone.** It checks the region, the routing key's length and the
  severity map, and, when a REST key is set, calls `GET /abilities` on the region's REST
  host. The routing key itself cannot be proven without creating an alert; use
  **Send Test Event** on the PagerDuty integration page for that.
- **Plain notifications** (a workflow notify node, a backup outcome) become a trigger with
  a key derived from the title and message, so the same notification repeated while the
  first alert is open does not page twice.

## Status coming back

| PagerDuty status | Tickets tab |
|---|---|
| `triggered` (also after `incident.reopened` / `incident.unacknowledged`) | Open |
| `acknowledged` | Acknowledged |
| `resolved` | Resolved |

Any other value is dropped rather than guessed.

- **Webhooks v3.** A Generic Webhook (v3) subscription posts to the channel's inbound URL.
  Each delivery is checked against `X-PagerDuty-Signature`: any one `v1=` entry that is a
  valid HMAC-SHA256 of the body passes, which is how PagerDuty rotates secrets; other
  schemes are ignored. PagerDuty signs no timestamp, so the event's `occurred_at` is what
  McHarbor orders by, so a replayed old delivery cannot undo a newer status.
- **Polling.** With a REST key, each open incident is looked up by
  `GET /incidents?incident_key=<ticket key>`, five at a time, every *Poll interval*.
  Without a REST key the channel does not poll.

### Alert grouping

When the PagerDuty service uses alert grouping (intelligent, content-based or time-based),
a McHarbor alert can join an incident that another alert opened. That incident carries the
other alert's key, so neither a webhook nor a poll matches it to the McHarbor ticket, and
its acknowledge and resolve are not read back. The ticket stays **Open** (a lookup that
finds nothing is never treated as a deleted ticket), and the resolve is still sent when
the alert recovers.

## Capabilities it advertises

| Capability | Set | Meaning |
|---|---|---|
| `EncryptedCredentials` | Yes | The routing key, REST key and signing secret are stored encrypted and decrypted only at call time. |
| `SupportsRichAttachments` | No | Structured cards or blocks beyond title and message. |
| `SupportsFileAttachments` | No | Files cannot ride along with an event. |
| Incident lifecycle | Yes | Open, Update and Resolve, an inbound webhook, and polling. |

## In this folder

| Page | What it covers |
|---|---|
| [Configuration](./configuration.md) | Prerequisites, every field, and the setup order in PagerDuty and McHarbor |
| [Troubleshooting](./troubleshooting.md) | Symptoms, causes and fixes |

## Related

- [Communication channels](../README.md): the family, and what the host does with it
- [Tickets](../../../settings/monitoring/alerts/tickets/README.md): how a ticket follows an alert
- [Communications](../../../settings/integrations/communications/README.md): channels and the inbound URL
- [Plugin catalog](../../catalog.md): every shipped plugin
