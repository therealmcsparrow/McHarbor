# Dashboard: Fields

What the dashboard reads and what you can set on it: the environment its widgets follow,
the per-widget size in grid units, and the starting layout.

## Filters and facets

| Filter | What it narrows |
|---|---|
| Environment | Widgets follow the environment switcher; some aggregate across all of them |

## Form fields

| Field | Type | What it is |
|---|---|---|
| Size (width, height) | number | Per widget, in grid units |
| Dashboard layout | select | Starter, Homelab, DevOps, Production or Monitoring |

## Shared list behaviour

Every list screen inherits the same controls: free-text search, a filter mode of
**Contains**, **Exact** or **Regex** (an invalid expression says so rather than silently
matching nothing), saved filters and searches, selection with a bulk bar, and pagination
showing "X to Y of Z" with a rows-per-page control.

## Related

- [Dashboard](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what you can do with these fields
