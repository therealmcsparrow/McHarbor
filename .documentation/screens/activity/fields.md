# Activity: Fields

Every column and filter on Activity (the live feed of Docker events the daemon reports),
with what each column holds and what narrows the list.

## Columns

| Column | What it is |
|---|---|
| Timestamp | When the daemon reported it |
| Environment | Which host |
| Action | `start`, `die`, `kill`, `oom`, `pull` and the rest of Docker’s vocabulary |
| Container | Container name |
| Image | Image the container runs |
| Exit | Exit code, where the event carries one |

## Filters and facets

| Filter | What it narrows |
|---|---|
| Filter events | Free text over the visible rows |

## Shared list behaviour

Every list screen inherits the same controls: free-text search, a filter mode of
**Contains**, **Exact** or **Regex** (an invalid expression says so rather than silently
matching nothing), saved filters and searches, selection with a bulk bar, and pagination
showing "X to Y of Z" with a rows-per-page control.

## Related

- [Activity](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what you can do with these fields
