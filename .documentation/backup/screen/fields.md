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
| Log tail lines | number | How much container log to capture with the archive |
| What to back up | group | Which layers and paths the archive includes |

Both retention values at zero means "keep all successful backups", and the form states
that outright rather than leaving you to infer it. Retention prunes at the destination as
well as locally, which is why storage plugins advertise a delete capability.

## Shared list behaviour

Every list screen inherits the same controls: free-text search, a filter mode of
**Contains**, **Exact** or **Regex** (an invalid expression says so rather than silently
matching nothing), saved filters and searches, selection with a bulk bar, and pagination
showing "X to Y of Z" with a rows-per-page control.

## Related

- [Backups](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what you can do with these fields
