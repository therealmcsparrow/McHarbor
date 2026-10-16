# Audit: Fields

Every column and filter on the audit trail, plus the two tabs that sit beside it: plugin
events, which show when each plugin last emitted one, and the Access Log, which records
reads rather than writes.

## Columns

| Column | What it is |
|---|---|
| Timestamp | When |
| Severity | How serious |
| User | Who made the request. Blank on a row a plugin emitted for work nobody asked for directly, such as a schedule firing or a conversion finishing after its request returned. A change made with an API key (the `mcharbor` CLI, Terraform, a script) reads "via API key <name>" under the user, and the entry's details carry an **API key** row |
| Action | The canonical action name, e.g. `container.stop` |
| Entity / Entity ID / Entity Name | What it acted on. The name is the one stored on the row when the event happened, so a deleted user, group, channel or backup plan still reads by name |
| Environment | Where. Also the stored name, so an environment that has since been deleted or renamed still reads as it was at the time |
| IP address | The caller’s address |
| Details | The full entry |

## Filters and facets

| Filter | What it narrows |
|---|---|
| Search | Free text across entries |
| Actor | A username (case-insensitive) or a user id. A username also matches rows written under that user's id before a rename. API: `actor=` |
| Severity | Any of **Info**, **Success**, **Warning**, **Error**: the four values rows are stored with. API: `severity=` (repeatable or comma-separated); any other value is refused |
| Action | One or more canonical actions, chosen from the browser |
| Entity type | All types, or one |
| Module | All modules, or one |
| Date range | The standard presets plus a custom range |

**Plugin events** are listed separately, with when each plugin was last seen emitting one
and how many it has written. A plugin that is no longer installed keeps its row, marked
**Uninstalled**; its history is not removed with it. A plugin that is installed, should be
auditing, and appears nowhere is telling you something.

A plugin's actions carry a severity from the same action catalog the host's own do, so
`plugin.kvm.vm.deleted` files as a warning and `plugin.kvm.snapshot_schedule.failed` as an
error. An action with no catalog entry falls back to a heuristic and lands on **Info**,
which is what a whole family of add-on actions used to do.

The **Access Log** tab records reads rather than writes (viewing a secret, opening a
terminal, downloading a backup), where the interesting fact is that someone looked.

## Export columns

The export dialog's **Columns** picker selects any of: ID, Timestamp, Severity, User ID,
Username, Action, Entity type, Entity ID, Entity name, Environment ID, Environment name,
IP address, API key ID, API key name, Details. Nothing selected means **All columns**.
Columns are always written in that order, whatever order they were picked in. API:
`GET /api/audit/export?fields=id,action,…`; an unknown field is refused.

The export's manifest lists the columns under `manifest.columns`, and its filters carry
`actor` and `severities`; all three are inside the signature.

## Shared list behaviour

Every list screen inherits the same controls: free-text search, a filter mode of
**Contains**, **Exact** or **Regex** (an invalid expression says so rather than silently
matching nothing), saved filters and searches, selection with a bulk bar, and pagination
showing "X to Y of Z" with a rows-per-page control.

## Related

- [Audit](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what you can do with these fields
