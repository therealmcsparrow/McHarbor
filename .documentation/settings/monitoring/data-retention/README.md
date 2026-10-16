# Data retention

**Settings → Monitoring → Data retention** · `?tab=retention`

How long audit, activity, lifecycle, backup-log and scan history are kept. **Set any value
to 0 to keep forever.**

## Permissions

| Key | What it unlocks |
|---|---|
| `settings.view` | Read the windows |
| `settings.manage` | Change them, and purge now |

A section you lack the key for renders **disabled with a tooltip naming it**, rather than
opening onto an empty panel.

## What it governs

- A retention window per history: audit, activity, lifecycle, backup log, scan history.
- App-wide backup retention defaults, by count and by age.
- A **Clear now** per log, which purges immediately instead of waiting for the next sweep.
- A **Prometheus metrics** card with the scrape address for McHarbor's own metrics (below).

## Prometheus metrics

The **Prometheus metrics** card shows the endpoint a scraper uses, `<this McHarbor>/api/metrics`,
with a copy button, and a **Test scrape** button that requests it the way a scraper would and
reports how many lines of metrics came back. **Test scrape** needs `metrics.scrape`; without it
the button is disabled with a tooltip naming the key, and a refused request shows the server's
own message.

A real scraper authenticates with an API key that carries `metrics.scrape`. Creating one, the
Prometheus job and what the exposition contains are in [Observability](../../../observability/README.md).

## Audit retention is a governance decision

The audit trail is hash-chained and write-once so that it can be evidence. Shortening its
retention is the one legitimate way to remove entries, which is exactly why it is worth
agreeing the number with whoever asks for the evidence, before an incident rather than
during one.

Two ways to keep a long trail without a large database: **forward it** to a SIEM or syslog
collector, where retention is the collector’s problem and a McHarbor administrator cannot
shorten it, or **export it** as a signed export. Forwarding is the stronger of the two.

## One variable that does not exist

The shipped `docker-compose.yml` carries a commented-out `AUDIT_LOG_RETENTION_DAYS`,
described there as a recovery path for a misconfigured policy. **Nothing in the code reads
it.** Audit retention is set here and nowhere else.

## In this folder

| Page | What it covers |
|---|---|
| [Reference](./reference.md) | Every setting, its default, and what changing it costs |

## Related

- [Audit](../../../screens/audit/README.md): the trail, and exporting it
- [Integrations → Logging](../../integrations/logging/README.md): forwarding it out
- [Backups](../../../backup/screen/README.md): per-plan retention
