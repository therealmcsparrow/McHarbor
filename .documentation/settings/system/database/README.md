# Database

**Settings → System → Database** · `?tab=database`

Storage engine, size, and maintenance for the McHarbor database.

## Permissions

| Key | What it unlocks |
|---|---|
| `database.view` | Read the current database |
| `database.manage` | Migrate to another backend, run the health check, and apply a repair |

A section you lack the key for renders **disabled with a tooltip naming it**, rather than
opening onto an empty panel.

## What it governs

- Which backend McHarbor runs on, and where the configuration came from.
- Migrating every row to another backend, and restarting into it.
- Checking the active database for corruption, orphaned rows, schema drift and reclaimable
  space, and applying the repairs that are safe to apply.

## The backends

| Backend | What it is |
|---|---|
| **SQLite** | "A single file inside the McHarbor data directory. No server to run, no credentials to keep. **Single node only.**" |
| **PostgreSQL** | "An external PostgreSQL 12 or newer. The most complete choice for a shared database, including multi-node active/passive." |
| **MySQL / MariaDB** | "An external MySQL 8 or MariaDB 10.6 or newer. Same capabilities as PostgreSQL; pick the one you already run" |
| **Microsoft SQL Server** | "An external Microsoft SQL Server 2022 or newer. Validated for multi-node active/passive, like PostgreSQL, MySQL and MariaDB." |

SQLite is the default and the right answer for a single McHarbor. The external drivers
exist for running several nodes against one database, or for keeping McHarbor on the
database estate you already run, not as a performance upgrade.

## Choosing where to move

**Move to another database** opens on the backend McHarbor is running on, selected and
badged **In use**. While it is selected there is no connection form, and **Test connection**
and **Migrate data** are disabled, with the note "McHarbor is running on &lt;backend&gt; now.
Pick a different backend above to copy every row to it, or move it to another &lt;backend&gt;
database." Choosing any other card opens its connection form.

**Move to another &lt;backend&gt; database** under that note opens the connection form for the
same backend: a new SQLite **File path** on a faster disk or another volume, or another
PostgreSQL, MySQL or SQL Server server. It needs `database.manage`. The copy is refused with
an error when the target is the database McHarbor is running on: for SQLite that is the same
file under any spelling of its path, for a server the same connection string. Choosing another
card and coming back closes the form again.

## SQL Server

The form builds a `sqlserver://` URL with the database as `?database=`, the only
connection-string form McHarbor opens. A pasted connection string must be that URL form
and must name a database; the ODBC keyword form, and a string naming `master`, `tempdb`,
`model` or `msdb`, are refused with "The connection details are incomplete or malformed."

| Field | Default | Notes |
|---|---|---|
| **Port** | 1433 | |
| **Encryption** | `true` | `true`, `strict` (TDS 8), `false`, `disable` (the driver's `encrypt` values) |
| **Trust the server certificate** | Off | Needed for a default SQL Server install, whose certificate is self-signed. The connection stays encrypted but is no longer protected against someone impersonating the server |

Two settings of the database are read by **Test connection**, under **Version supported**:

- **XACT_ABORT** on for the login's sessions fails the step. McHarbor refuses to start on
  such a database, so the migration is refused before it copies anything.
- **READ_COMMITTED_SNAPSHOT** off is a warning, with the statement that turns it on:
  `ALTER DATABASE [<db>] SET READ_COMMITTED_SNAPSHOT ON WITH ROLLBACK IMMEDIATE`. Without
  it, a long read blocks writers.

During the copy the target's foreign keys and triggers are switched off, because SQL
Server has no per-session switch like MySQL's, and switched back on afterwards and
re-validated against the copied rows. A table whose rows reference rows that do not exist
(an orphan SQLite let through) keeps its foreign keys for new writes and is named under
**Worth knowing**. A failed or cancelled copy switches them back on too. The catch-up
passes after the copy do the same for the tables they write to, and size their batches
under SQL Server's 2,100-parameter limit like the main copy.

An install running on SQL Server can move off it the same way: pick SQLite, PostgreSQL or
MySQL / MariaDB and **Migrate data**. Tables are ordered by their foreign keys, including
the ones SQL Server carries as delete triggers because it refuses a second cascade path,
and every value arrives as the source held it: 64-bit integers, Unicode text, NULLs and
timestamps.

## What happens during the copy

> McHarbor holds itself still for the copy: mutating API requests are answered with 503, and
> every background collector and scheduler skips its tick. Ticks already running are waited
> out before the first row is read, and catch-up passes afterwards copy anything that still
> landed on the old database. **Reading the interface keeps working throughout.**

And what it does not do:

> The old database is never touched. Nothing is deleted from it, and until you say otherwise
> it stays exactly as it was.

That is the rollback: the old database is still there and still current as of the migration.

Large values copy as they are. Every whole-number column is 64-bit on every backend, so a
disk size, a backup size or a memory figure above 2 GiB lands in PostgreSQL or MySQL
unchanged. Earlier builds declared those columns 32-bit on the external drivers, and the
copy stopped at the first such row; see [Reference → Whole numbers](./reference.md#whole-numbers-are-64-bit-on-every-backend).

## Keeping queries fast

On SQLite, McHarbor refreshes the query planner's statistics by itself: once when the
database opens, and once a day after the retention clean-up. Only tables whose statistics
are missing or out of date are re-analyzed, so the run takes well under a second and never
holds up a request. PostgreSQL and MySQL keep their own statistics, and McHarbor leaves
them to it.

## In this folder

| Page | What it covers |
|---|---|
| [Health check](./health-check.md) | What the four check families look at, how a finding reads, and what each repair will and will not touch |
| [Reference](./reference.md) | Every setting, its default, and what changing it costs; which backends are verified; what each migration file adds |

## Related

- [Getting started → Configuration](../../../getting-started/configuration/README.md): `MCHARBOR_DB_DRIVER` and `MCHARBOR_DB_DSN`
- [Getting started → First run](../../../getting-started/first-run/README.md): choosing a database before the first account
