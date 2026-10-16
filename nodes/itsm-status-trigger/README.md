# Ticket Status

Starts the workflow when an ITSM reports that a linked ticket changed state.

## When To Use It

Use it to react to what people do in the ticketing system: post in chat when a ticket is acknowledged, restart a check when one is resolved, escalate when one is cancelled while the alert still fires.

## Requirements

A ticketing channel whose plugin reports status, either through the inbound webhook (Settings → Communication → the channel → Inbound updates) or by polling. Only statuses for tickets McHarbor opened are reported.

## Port Behavior

- Output `output`: `msg.payload` is `{ channelId, channelType, externalId, number, url, dedupKey, state, assignee, note }` and `msg.topic` is `itsm-status-trigger`.

## Configuration Guide

- `Ticketing Channel` (select, optional): Only statuses from this channel. Empty accepts every ticketing channel.
- `States` (multiselect, optional): Only these states — `open`, `acknowledged`, `resolved`, `closed`, `cancelled`. Empty accepts all.

A status older than the last one applied to the ticket is dropped before it reaches the trigger, so a replayed webhook does not start a run.
