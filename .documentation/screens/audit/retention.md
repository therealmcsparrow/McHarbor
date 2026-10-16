# Audit: Retention

`/audit?view=retention`

How long audit events of each severity are kept. Routine **Info** events can expire in weeks
while **Error** events are kept for years.

## How the values combine

| Severity | Retention used |
|---|---|
| One with **Use default** on | The audit retention under [Settings → Monitoring → Data retention](../../settings/monitoring/data-retention/README.md) |
| One with its own value | That value |

`0` keeps events of that severity forever. The range is 0 to 3,650 days. A default of `0`
no longer means nothing expires: a severity with its own value is still pruned.

The four severities are the values rows are stored with: **Info**, **Success**,
**Warning**, **Error**. A row with any other stored severity follows the default.

## When it takes effect

The pruner runs at startup and hourly, on the cluster leader only (singleton role
`audit-pruner`). **Save retention** changes nothing immediately; the next pass applies it.
**Purge** under Data retention applies the same per-severity policy at once.

Each pass removes up to 20,000 rows per batch and at most 50 batches, so a very large
backlog is cleared over several passes.

## Integrity

Deleting rows from a hash chain would break it. The pruner writes a sealed **tombstone** for
every run of consecutive removed rows, in the same transaction as the delete, so
verification crosses the gap and reports it. See [Integrity](./integrity.md).

## Storage

Each override is a row in the `settings` table, key `audit.retention.<severity>_days`
(category `audit`). Clearing an override (**Use default**) deletes the row.

## Permissions and API

| Action | Route | Gated by |
|---|---|---|
| Read the policy | `GET /api/audit/retention` | `audit.view` |
| Save it | `PUT /api/audit/retention` | `audit.view` and `settings.manage` (global) |

The body is `{ "overrides": { "<severity>": <days> | null } }`; only the severities
present are changed, `null` returns one to the default. An unknown severity or a value out
of range is refused with `data.reason = "invalid_retention"`. Every save writes an
`audit.retention.updated` row naming the effective value of each severity before and after.

## Related

- [Audit](./README.md): the screen as a whole
- [Settings → Monitoring → Data retention](../../settings/monitoring/data-retention/README.md): the default
