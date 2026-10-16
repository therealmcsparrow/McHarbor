# Volumes: Fields

Every column, form field and detail tab on Volumes: what the list reports, what creating
one asks for, and what an open volume shows, including which containers mount it, where,
and read-write or read-only.

## Columns

| Column | What it is |
|---|---|
| Name | Volume name |
| Driver | Usually `local` |
| Usage | In use or unused |
| Mountpoint | Path on the host |
| Created | When it was created |

## Filters and facets

| Filter | What it narrows |
|---|---|
| Search | Volume name |
| View | Table or card |

## Form fields

| Field | Type | What it is |
|---|---|---|
| Volume name | text | Required |
| Driver | text | Defaults to `local` |

## Detail tabs

| Tab | Fields |
|---|---|
| **Overview** | Name, driver, scope, mountpoint, created, usage, status, size |
| **Containers** | Container, container id, mount path, access (read-write or read-only) |
| **Configuration** | Driver options and labels, or a statement that it carries neither |

## Shared list behaviour

Every list screen inherits the same controls: free-text search, a filter mode of
**Contains**, **Exact** or **Regex** (an invalid expression says so rather than silently
matching nothing), saved filters and searches, selection with a bulk bar, and pagination
showing "X to Y of Z" with a rows-per-page control.

## Related

- [Volumes](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what you can do with these fields
