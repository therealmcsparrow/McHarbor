# Resolve Ticket

Resolves the ticket a key opened.

## When To Use It

Use it after the workflow has fixed or verified the condition it opened a ticket for.

## Requirements

An enabled channel under Settings → Communication backed by a ticketing plugin. Saving a workflow that contains this node needs `alerts.manage`: the server refuses the save otherwise, and the palette shows the node disabled with the permission named.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: The operation was queued. `msg.payload` is `{ queued, channelId, dedupKey, state }`.
- Output `error`: No channel was chosen or the channel cannot open tickets. `msg.payload.error` says which.

## Configuration Guide

- `Ticketing Channel` (select, required): A communication channel whose plugin can open tickets. The picker lists only those channels (`GET /api/itsm/channels`).
- `Ticket Key` (text, required): The dedup key that names the ticket. Open, Update and Resolve nodes that render the same key act on the same ticket, across runs. Accepts `{{path}}` placeholders against the incoming msg, for example `disk:{{payload.host}}`. A key an alert rule already uses (`<alertId>:<environmentId>:<subject>`) reaches that alert's ticket.
- `Message` (textarea, optional): A closing note. Accepts `{{path}}` placeholders.

## Delivery

The node does not call the ITSM itself. It queues the operation in the ticket outbox, which delivers it in order per ticket with retries and backoff — the same path alert rules use. `output` therefore means *queued*, not *accepted by the ITSM*; a delivery that keeps failing is parked and shown under Alerts → Tickets with its error.

