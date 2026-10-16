# Git: Fields

Every column and form field on Git: the repositories McHarbor follows, the branch each one
tracks, and whether the last sync worked.

## Columns

| Column | What it is |
|---|---|
| Name | Your label |
| URL | Repository URL |
| Branch | Which branch is followed |
| Status | synced, pending or error |
| Last synced | Or "Never" |

## Filters and facets

| Filter | What it narrows |
|---|---|
| Search | Repository name |

## Form fields

| Field | Type | What it is |
|---|---|---|
| Name | text | Your label |
| Repository URL | text | HTTPS or SSH |
| Branch | text | The branch to follow |

## Shared list behaviour

Every list screen inherits the same controls: free-text search, a filter mode of
**Contains**, **Exact** or **Regex** (an invalid expression says so rather than silently
matching nothing), saved filters and searches, selection with a bulk bar, and pagination
showing "X to Y of Z" with a rows-per-page control.

## Related

- [Git](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what you can do with these fields
