# Lifecycle Log: Fields

Every column and filter on the Lifecycle Log: what each row records about a container,
image, volume, network or stack, and the four ways of narrowing the list.

## Columns

| Column | What it is |
|---|---|
| Subject | The container, image, volume, network, stack, virtual machine or storage pool |
| Detail | What happened, in words |
| Severity | Success, Info, Warning or Error |
| Time | When |

## Filters and facets

| Filter | What it narrows |
|---|---|
| Subject type | All subjects, Container, Image, Volume, Network, Stack, then one entry per subject type an installed plugin declares: Virtual machine (Virtual Machines) and Storage pool (Compute Foundation). A plugin's entry is offered only while that plugin is installed |
| Severity | Any severity, Success, Info, Warning, Error |
| Search | By container, image or volume name |
| Date range | Presets from Last 15 minutes to Last year, plus a custom range |

## Shared list behaviour

Every list screen inherits the same controls: free-text search, a filter mode of
**Contains**, **Exact** or **Regex** (an invalid expression says so rather than silently
matching nothing), saved filters and searches, selection with a bulk bar, and pagination
showing "X to Y of Z" with a rows-per-page control.

## Related

- [Lifecycle Log](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what you can do with these fields
