# Notifications: Fields

Every column and filter on the in-app inbox: the level and type of a notification, whether
it has been read, and what narrows the list.

## Columns

| Column | What it is |
|---|---|
| Level | Info, Success, Warning or Error |
| Title | What happened |
| Message | The detail |
| Type | Alert, Container or System |
| Status | Read or Unread |
| Date | When it arrived |

## Filters and facets

| Filter | What it narrows |
|---|---|
| Search | Title and message |
| Status | Read or unread |
| Type | Alert, Container, System |

## Shared list behaviour

Every list screen inherits the same controls: free-text search, a filter mode of
**Contains**, **Exact** or **Regex** (an invalid expression says so rather than silently
matching nothing), saved filters and searches, selection with a bulk bar, and pagination
showing "X to Y of Z" with a rows-per-page control.

## Related

- [Notifications](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what you can do with these fields
