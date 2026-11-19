# Backups: Fields

Every column, filter and form field on Backups: what a run shows while it happens, and
what a plan asks for: contents, destinations, schedule, consistency mode and retention.

## Columns

| Column | What it is |
|---|---|
| Container | What was backed up |
| Environment | Which host |
| Status | Running, complete, failed or cancelled |
| Destinations | Where the archive was written; a plan can write to several |
| Size | Archive size; "—" on a restore run, which writes no archive |
| Started at | When the run began |
| Duration | How long it took |

## Filters and facets

| Filter | What it narrows |
|---|---|
| Search | Container and plan name |
| Severity | On the log tab: info, success, warning, error |
| Action | On the log tab: start, stop, delete, upload, fail, complete, create, destroy, phase, audit |

## Form fields

| Field | Type | What it is |
|---|---|---|
| Name | text | Plan name; the first field of the section |
| Container | select | What the plan backs up |
| Destination | multi-select | **Local storage** or any configured storage destination, more than one at a time |
| Schedule | cron | Read in the configured time zone, with the expression rendered back in words |
| Consistency | select | Pause, Snapshot or Live |
| Snapshot configuration | select | Shown for Snapshot; unavailable when the environment has none |
| Retention (count) | number | 0 keeps every successful run by count |
| Retention (days) | number | 0 keeps successful runs regardless of age |
| Keep a copy on this server | switch | Off by default for a new plan. On keeps every archive in `<DATA_DIR>/backups/containers/<run>/` as well as on the destinations |
| Log tail lines | number | How much container log to capture with the archive |
| What to back up | group | Which layers and paths the archive includes |

Both retention values at zero means "keep all successful backups", and the form states
that outright rather than leaving you to infer it. Retention prunes at the destination as
well as locally, which is why storage plugins advertise a delete capability.

A run builds its archive in its own folder under the backup temp root (see
[Configuration](../../getting-started/configuration/README.md#backup-temporary-files)) and uploads it to
every destination from there. When the run ends, the archive goes to one of three places:

- **Nowhere on this server** when every destination holds it and **Keep a copy on this server** is off.
  A manual one-off backup counts as off.
- **`<DATA_DIR>/backups/containers/<run>/mcharbor.tar`** (`/var/lib/mcharbor/backups/containers/` on a
  native install) when the switch is on, or when any destination failed, so **Retry upload** has
  something to send. A failed or cancelled run's archive is removed seven days after the run ended,
  and **Retry upload** then reports that the archive is gone. A successful retry removes the copy again
  when the switch is off.
- **Deleted** when the archive was not finished: the run failed while writing it, or was cancelled.

Restore, the restore browser, download and **Migrate to local storage** fetch an archive that is not on
this server back from the first destination that holds it, which needs the storage plugin that wrote it
and takes as long as the download. A restore shows this as "Fetching the archive from a backup
destination." The fetched copy is removed again after an hour without use.

The hourly backup sweep also clears copies left in `<DATA_DIR>/backups/containers/` that nothing needs:
copies every destination holds from plans with the switch off and from one-off runs, failed runs older
than seven days, and run folders older than a day that no run refers to and that hold only archive
files. Turning the switch off on an existing plan therefore frees its earlier copies within the hour.

## Shared list behaviour

Every list screen inherits the same controls: free-text search, a filter mode of
**Contains**, **Exact** or **Regex** (an invalid expression says so rather than silently
matching nothing), saved filters and searches, selection with a bulk bar, and pagination
showing "X to Y of Z" with a rows-per-page control.

## Related

- [Backups](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what you can do with these fields
