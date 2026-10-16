# Tickets

**Settings → Monitoring → Alerts → Tickets** · `?tab=alerts`

An alert rule whose destination is a ticketing channel does not send a message; it opens a
ticket in the ITSM, adds notes while the alert keeps firing, and resolves the ticket when the
alert recovers. The ITSM reports back (acknowledged, resolved, closed) and McHarbor acts on
what it hears. The Tickets tab lists every ticket McHarbor opened and where each one stands.

A channel is a ticketing channel when its plugin implements the incident lifecycle. Which
plugins do is listed under [Communication plugins](../../../../plugins/communication/README.md);
the channel picker in the workflow nodes shows only those channels.

## Permissions

| Key | What it unlocks |
|---|---|
| `alerts.view` | See the Tickets tab |
| `alerts.manage` | **Re-sync** a ticket, **Try again** after a stopped delivery, and save a workflow containing an Open, Update or Resolve Ticket node |
| `communications.view` | See which channels can open tickets |
| `communications.manage` | Create or rotate a channel's inbound URL, and store the ITSM's signing secret |

## How a ticket follows an alert

Every alert condition has a **ticket key**, `<alertId>:<environmentId>:<subject>`, and one
channel holds at most one live ticket per key.

| The alert | What McHarbor queues |
|---|---|
| Fires, no live ticket | **Open** |
| Fires again while the ticket is open | **Update** (a note), unless the channel's *Add a note on repeat firing* is off |
| Fires again after someone **acknowledged** the ticket in the ITSM | Nothing: an acknowledgement silences repeat notes until the alert recovers |
| Recovers | **Resolve** |
| Recovers after the ITSM already closed the ticket | Nothing is sent; the link is marked resolved |

Nothing is delivered from the alert engine itself. Each operation goes into the ticket
**outbox**, which delivers them in order per ticket: a Resolve never overtakes the Open it
depends on, and an Update waits for the ticket to exist.

## Link states

| State | Meaning |
|---|---|
| Pending | Queued; the ITSM has not confirmed a ticket yet |
| Open | The ticket exists and McHarbor still owns its lifecycle |
| Closed remotely | Someone closed the ticket in the ITSM while the alert is still firing. McHarbor does not reopen it or open a second one |
| Resolved | Closed, by the recovery or in the ITSM after the alert recovered |
| Orphaned | The ticket or the channel was deleted. Nothing more is sent |

The **ITSM** column shows the status the ITSM last reported (Open, Acknowledged, Resolved,
Closed, Cancelled) and, where it says, who the ticket is assigned to.

## When delivery fails

- A transient failure (the ITSM is down, answers 5xx, or rate-limits) is retried with
  exponential backoff from 10 seconds up to 30 minutes. When the ITSM sends `Retry-After`,
  that wait is used instead.
- After 12 attempts, or on the first failure that cannot succeed by retrying (a rejected
  field, a missing permission in the ITSM), the delivery **stops retrying**. The row shows
  **Stopped retrying** and an error icon beside its state; the icon opens the full message.
  The message is the ITSM's own text when it is safe to show, and a generic one otherwise;
  credentials are never part of it.
- **Try again** puts a stopped delivery back in the queue. **Re-sync** reads the ticket's
  current status from the ITSM now.
- A ticket that the ITSM reports as deleted moves to **Orphaned** and its queue is dropped.

## Status coming back

A plugin reports ticket status in one or both of two ways:

- **Inbound webhook.** The ITSM calls McHarbor when a ticket changes. See *Ticket status from the ITSM*
  under [Communications](../../../integrations/communications/README.md) for creating the URL.
- **Polling.** McHarbor reads the status of every open ticket on the channel's *Poll interval*
  (default 5 minutes, `0` turns it off).

A status older than the one last applied to a ticket is ignored, so a replayed webhook cannot
reopen what a newer update closed. Every applied status can start a workflow through the
**Ticket Status** trigger.

## The inbound route

`POST /api/incident-webhooks/{token}` is public by necessity: the ITSM cannot log in. It is
protected by:

- a per-channel token of 256 random bits, stored only as a hash and shown once when created;
- the plugin's own verification of each request: the ITSM's signature where it signs, or the
  channel's shared secret compared in constant time;
- a limit of 30 requests per minute per client and a 1 MiB body cap;
- a uniform `401` for every failure (an unknown token, a disabled channel, a bad signature),
  so the response never tells a caller which part was wrong.

Each accepted call writes an `incident.webhook.received` audit entry with the number of
statuses applied.

## Workflows

Four nodes drive tickets from a workflow: **Open Ticket**, **Update Ticket**, **Resolve
Ticket** and the **Ticket Status** trigger. They go through the same outbox. See
[Integration nodes](../../../../workflows/nodes/integrations/README.md) and
[Trigger nodes](../../../../workflows/nodes/triggers/README.md).

## Routes

| Route | Key |
|---|---|
| `GET /api/itsm/links` | `alerts.view` |
| `POST /api/itsm/links/{id}/resync` | `alerts.manage` |
| `POST /api/itsm/links/{id}/retry` | `alerts.manage` |
| `GET /api/itsm/channels` | `communications.view` |
| `POST /api/itsm/channels/{id}/webhook-token` | `communications.manage` |
| `POST /api/itsm/channels/{id}/webhook-secret` | `communications.manage` |
| `POST /api/incident-webhooks/{token}` | none; authenticated per channel, as above |

## Related

- [Alerts](../README.md): rules and destinations
- [Communications](../../../integrations/communications/README.md): channels and the inbound URL
