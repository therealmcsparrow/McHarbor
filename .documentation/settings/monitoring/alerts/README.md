# Alerts

**Settings → Monitoring → Alerts** · `?tab=alerts`

Rules that watch metrics and container state, and where they deliver.

## Permissions

| Key | What it unlocks |
|---|---|
| `alerts.view` | See rules and history |
| `alerts.manage` | Create and change rules |
| `alerts.delete` | Delete a rule |

A section you lack the key for renders **disabled with a tooltip naming it**, rather than
opening onto an empty panel.

## What it governs

- What to watch: CPU, memory, disk, container down, or image update, plus any condition a plugin contributes.
- How long it must hold before firing, and how often the rule may speak.
- Where a triggered alert goes: the in-app inbox, and any configured communication channel.

## The two fields that separate a useful alert from a pager that cries wolf

**Evaluation window** ("how much history is averaged before the threshold is compared")
and **Sustained for** ("the condition must hold this long before the alert fires", or
**Immediately**).

A CPU spike averaged over 30 seconds and sustained for 5 minutes is a problem; the same
spike evaluated instantaneously is a garbage-collection pause.

## Destinations

At least one is required before the rule saves. **In-app notifications** always works and
needs no plugin; external delivery needs a
[communication channel](../../integrations/communications/README.md).

With no channels configured the section says so and keeps going: "You can still send alerts
to the in-app notification center."

Destinations that have since been deleted are reported rather than silently dropped:
"2 destinations no longer exist and are ignored."

## History

"Every time a rule fired or resolved, and whether delivery succeeded." Delivery success is
recorded separately from firing: a rule that fires and fails to deliver looks identical to
a working one until you read this tab.

## Tickets

A destination that is a ticketing channel opens a ticket in the ITSM instead of sending a
message, notes repeat firings on it, and resolves it on recovery. Someone acknowledging the
ticket in the ITSM silences repeat notifications until the alert recovers. The **Tickets**
tab lists every ticket McHarbor opened, its state on both sides, and why a delivery stopped.

## In this folder

| Page | What it covers |
|---|---|
| [Reference](./reference.md) | Every setting, its default, and what changing it costs |
| [Tickets](./tickets/README.md) | The ticket lifecycle, link states, retries, status coming back, the inbound route |

## Related

- [Integrations → Communications](../../integrations/communications/README.md): external delivery
- [Tickets](./tickets/README.md): alerts as ITSM tickets
- [Notifications](../../../screens/notifications/README.md): the in-app inbox
- [Data retention](../data-retention/README.md): how long history is kept
