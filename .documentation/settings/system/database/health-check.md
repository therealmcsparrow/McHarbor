# Database health check

**Settings → System → Database → Database health check**

A sweep across the active database looking for four kinds of problem: storage corruption,
rows whose parent is gone, a schema that has drifted from what the migration set declares,
and space that is being held and not used. It reports what it found and, where something
can safely be done about it, offers to do it.

It reads the database McHarbor is running on right now, whichever of the four backends
that is. It is not a backup, and it does not repair anything unless you ask it to.

## Permissions

| Key | What it unlocks |
|---|---|
| `database.view` | Read the last report |
| `database.manage` | Start a run, cancel one, and apply a remedy |

Starting a run is `database.manage` rather than `database.view` because a full sweep across
every table is real load on the database every other request shares.

## What it checks

The four families run in order, and each one reports even when it finds nothing; an
operator must be able to tell *checked, clean* from *never checked*.

| Family | What it looks at |
|---|---|
| **Integrity** | The storage engine's own consistency check |
| **Orphaned rows** | Rows pointing at a parent row that no longer exists |
| **Schema** | Applied migrations, and indexes the migration set declares |
| **Size and retention** | Rows and bytes per table, reclaimable space, and tables holding rows older than their retention window |

### Integrity

On SQLite this is `PRAGMA integrity_check` plus `PRAGMA foreign_key_check`. On MySQL and
MariaDB it is `CHECK TABLE` on each table. PostgreSQL has no in-server equivalent without
the `amcheck` extension, which McHarbor cannot assume is installed, so it reports the honest
subset (invalid indexes) and **skips the rest with the reason stated on the finding**
rather than reporting a clean database it did not actually verify.

### Orphaned rows

Declared foreign keys are only part of the picture. Roughly 46 columns across the schema
hold an `environments.id` and almost none of them are constrained, so the check also
discovers relations by convention: a column whose name looks like it holds another row's id
is resolved to a parent table, with an override map for the ones convention gets wrong.

Two things it deliberately reports rather than hides:

- **Columns it could not map.** A discovery pass that silently discards what it cannot
  resolve has the same blind spot as a hand-maintained list and hides it better.
- **Columns it excluded by name**: an external identity provider's subject, a
  self-reference, a pointer that is allowed to dangle once the row it named was revoked.

### Schema

Two comparisons: the `schema_migrations` ledger against the migration files this build
carries, and every index the migration set nets out to against the indexes the database
actually holds.

*Nets out to* is the important word. Migrations run in name order, so a later `DROP INDEX`
or `DROP TABLE` removes an earlier `CREATE` from the set. An index the schema deliberately
retired is not missing, and the remedy will never offer to undo the migration that removed
it. A statement the parser could not read is counted and reported, not skipped in silence.

### Size and retention

Row counts and byte sizes per table, the space a `VACUUM` would return to disk, the size of
the write-ahead log, and the tables holding rows older than the window configured under
[Data retention](../../monitoring/data-retention/README.md).

Retention findings are advice only. The fix is adjusting the policy or letting the retention
job run, both of which live in the Retention tab; the health check does not delete on age.

## How a finding reads

| Severity | Meaning |
|---|---|
| **OK** | Checked, nothing wrong |
| **Info** | Worth knowing, no action implied: a row count, a table size, a governed record |
| **Warning** | Something is not as declared, and probably should be |
| **Failed** | The storage engine itself reported a problem |
| **Skipped** | Could not be checked here, **with the reason on the finding** |

A skip always names its cause: this driver has no built-in integrity check, the SQLite
pragma this build needs is absent, the table a retention window governs does not exist,
that window is set to keep rows forever, per-table size statistics are not exposed by this
driver. A skip is never silence.

## The repairs it offers

A finding carries a remedy only when there is something safe to do about it. Selecting
findings and applying them runs each remedy in a fixed order: non-destructive first,
then destructive, then `VACUUM` last, because vacuuming before the deletions would do the
work twice.

| Remedy | What it does |
|---|---|
| **Apply pending migrations** | Runs the migrations the ledger says are missing |
| **Create the missing index** | Creates the index from the migration set's own net declaration, not a reconstruction |
| **Delete the orphaned rows** | Deletes the child rows whose parent is gone |
| **Run VACUUM** | Returns reclaimable space to the disk |

Two guards worth knowing about:

- **Every finding is re-verified before it is acted on.** A report can be minutes old, and
  deleting rows from a stale count is exactly the failure this feature exists to catch. An
  orphan count that has fallen to zero in the meantime skips rather than runs.
- **A repair only ever runs against a finding the server produced and you have seen.** An
  id the current report does not carry is refused.

Deleting rows asks twice. The confirmation says so in as many words, and it cannot be
undone.

## Governed tables are never offered for deletion

`audit_logs` has dangling references by design: an audit trail is meant to outlive the user
or the environment it names, which is the whole point of keeping one. The check still
**reports** those references, because you should be able to see them, but it reports them
as **Info** with the reason *governed audit record*, carries no remedy, and the repair path
refuses the table outright even if a stale finding somehow arrived carrying one.

For the same reason `audit_logs` is absent from the retention overrun check. How long an
audit trail is kept is a compliance decision, not bloat. See
[Audit](../../../screens/audit/README.md).

## Cancelling, and what a failed run means

A run in progress reports the phase it is in and the table it is on, and can be cancelled;
it stops at the next table rather than mid-statement. A run that fails reports the phase it
failed in and the driver's own message, already redacted, behind the error icon; the report
is not silently replaced by an empty one.

## Related

- [Database](./README.md): the backends, and migrating between them
- [Reference](./reference.md): every setting on the tab
- [Data retention](../../monitoring/data-retention/README.md): where retention windows are set
- [Audit](../../../screens/audit/README.md): why `audit_logs` is governed
