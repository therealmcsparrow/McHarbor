# Logs: Fields

What the Logs screen shows and what narrows it. One thing is worth knowing before you
search a long history: the filter runs over the lines already loaded in the browser, not
on the server.

## Columns

| Column | What it is |
|---|---|
| Log line | One line of stdout or stderr, in the order the driver returned it, tinted by the severity word it contains |

## Filters and facets

| Filter | What it narrows |
|---|---|
| Container | Which container’s logs are shown |
| Filter log lines | Free text over the lines currently loaded |
| Order | Newest first or oldest first |

The filter runs over what is loaded in the browser, not server-side. Filtering a long
history means loading it first.

## Shared list behaviour

Every list screen inherits the same controls: free-text search, a filter mode of
**Contains**, **Exact** or **Regex** (an invalid expression says so rather than silently
matching nothing), saved filters and searches, selection with a bulk bar, and pagination
showing "X to Y of Z" with a rows-per-page control.

## Related

- [Logs](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what you can do with these fields
