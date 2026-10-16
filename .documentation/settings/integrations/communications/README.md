# Communications

**Settings → Integrations → Communications** · `?tab=communications`

Chat and messaging channels that alerts, events and system messages are delivered to.

## Permissions

| Key | What it unlocks |
|---|---|
| `communications.view` | See channels |
| `communications.manage` | Add or change one |
| `channels:use-<slug>` | Route events through that channel type |

A section you lack the key for renders **disabled with a tooltip naming it**, rather than
opening onto an empty panel.

## What it governs

- Email delivery servers: SMTP, Exchange Online, Gmail.
- Chat, webhook and push destinations.
- Which channel is the default, and whether each is enabled.

## Two kinds of destination

The section splits into **Email** ("SMTP, Exchange Online, and Google delivery servers")
and **Notification channels** ("chat, webhook, and push destinations"). Both are the same
mechanism; the split exists because an email server and a chat webhook are configured by
different people for different reasons.

## What sends through a channel

| Source | Configured in |
|---|---|
| Alert rules | [Monitoring → Alerts](../../monitoring/alerts/README.md) |
| Backup outcomes | The backup plan |
| Workflow notify nodes | [Workflows](../../../screens/workflows/README.md) |
| Schedule failures | [Automation → Schedules](../../automation/schedules/README.md) |
| New-release notices | [Updates → McHarbor](../../updates/mcharbor/README.md) |
| Password reset links | [Security](../../../screens/security/README.md) |

The last one is worth its own line: **until an email channel is picked there, reset links
are issued but never sent**, silently. Configuring an email channel here is only half of
that job.

## Ticket status from the ITSM

A channel whose plugin opens tickets shows a **Ticket status from the ITSM** section when it
is edited. When the plugin accepts webhooks:

- **Create inbound URL** issues the URL the ITSM calls and a **Signing secret**. Both are
  shown once: "Copy these now. They are not shown again." **Rotate inbound URL** replaces
  both after a confirmation; the ITSM cannot reach McHarbor until it has the new pair.
- **Signing secret issued by the ITSM** is for tools that generate their own secret; saving
  it replaces the one McHarbor issued.

When the plugin only polls, the section says so and needs nothing. Both actions need
`communications.manage`. What the ITSM's reports do is described under
[Tickets](../../monitoring/alerts/tickets/README.md).

Config keys that start with `_` are reserved for the host and a channel carrying one is
refused on save.

## In this folder

| Page | What it covers |
|---|---|
| [Reference](./reference.md) | Every setting, its default, and what changing it costs |

## Related

- [Communication channel plugins](../../../plugins/communication/README.md): all 18, with their fields
- [Monitoring → Alerts](../../monitoring/alerts/README.md): the biggest consumer of channels
