# Automation

What McHarbor does on its own.

| Section | Covers | Gated by |
|---|---|---|
| [Schedules](./schedules/README.md) | Recurring container actions on a cron expression | `schedules.view`, `.manage`, `.delete` |
| [Webhooks](./webhooks/README.md) | Outbound HTTP callbacks fired on McHarbor events | `webhooks.view`, `.manage`, `.delete` |

## Time-driven and event-driven

A schedule fires because the clock said so; a webhook fires because something happened.
Neither has branching, retries or run history. When you need those, the answer is a
[workflow](../../screens/workflows/README.md).

## Everything else that runs on a timer

| Job | Configured in |
|---|---|
| Backups | The backup plan: [Backups](../../backup/screen/README.md) |
| Image update checks | [Updates → Containers](../updates/containers/README.md) |
| Release checks | [Updates → McHarbor](../updates/mcharbor/README.md) |
| Retention purges | [Data retention](../monitoring/data-retention/README.md) |
| Drift checks | [Reconciler](../../screens/reconciler/README.md) |
| Catalog sync | The catalog sync cron in [Configuration](../../getting-started/configuration/README.md) |

All of them read cron in the time zone set in [General](../general/general/README.md).

## Related

- [Settings](../README.md): the other five groups
