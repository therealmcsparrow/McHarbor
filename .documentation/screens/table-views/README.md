# Table views

Every table built on `TablePage` (containers, images, volumes, networks, stacks,
environments, users, roles, groups, audit, activity, backups, and the rest of the list
screens) lets each signed-in user choose which columns it shows, in what order and how
wide, and save named filters to re-apply later. All of it is **personal**: it is stored
against the signed-in user on the server, not in the browser, so it follows that user from
one browser or device to the next.

## Edit table

The **Edit table** button in a table's toolbar opens a dialog titled "Edit table", with the
description "Choose which columns this table shows, in what order and how wide. Only you
see this layout."

- **Shown**: the visible columns, in their current order. Each row has a drag handle (or
  the up/down buttons for the keyboard path), the column's label, a column-settings button,
  and a remove button. Locked columns (the actions column) render without a handle, arrows
  or remove button, because they cannot be moved or removed.
- **Column settings**: the gear-like button on a shown column opens a popover with three
  fields:
  - **Width**: `Auto`, `Extra narrow`, `Narrow`, `Medium`, `Wide`, `Extra wide`, `Very
    wide`, `Widest`.
  - **Title alignment** and **Content alignment**: `Default`, `Left`, `Center`, `Right`.
    Content alignment also aligns badges and icons inside the cell, not only its text.
- **Add column**: the button above the Shown list. It is disabled ("No more columns to
  add.") when every available column is already shown. Otherwise it opens a popover titled
  "Available" with a search box and every hidden column, grouped by category, each with its
  own add button. The columns on offer are the table's own fields plus fields from related
  records, for example a user's roles and groups, a stack's owner, an image's "used by"
  containers, or an environment's agent status.
- **Reset to default**: in the dialog footer, next to Cancel and Save. It asks for
  confirmation ("This removes your column layout for this table and restores the default
  columns, order and widths.") before deleting your saved layout. The table then shows the
  default layout McHarbor ships for it. See below.

Edits are staged in the dialog and only take effect on **Save**; closing or cancelling
discards them.

### The shipped default layouts

McHarbor ships a default layout (shown columns, order, widths and alignment) for 23
tables, the host's own and the Virtual Machines and Compute plugin tables among them. A
user who has never saved a layout for a table sees the shipped one, so a fresh install and
every new account start from the same curated columns rather than the page's bare
built-in set. **Reset to default** returns to the shipped layout too. A table with no
shipped layout falls back to the page's built-in columns, order and widths.

Nothing is written for the user until they save: the shipped layout is served in place of
a missing row, so changing it in a later release reaches everyone who never customised
that table.

## Column resizing

Every resizable column header has a drag handle on its right edge.

- **Drag**: dragging the handle previews the nearest width token as you move, with the
  token's name shown under the handle; releasing commits that width.
- **Keyboard**: the handle is a focusable separator; pressing **←** or **→** while it is
  focused steps the column one width token narrower or wider.

The width tokens are the same set the Edit table column-settings popover offers: `Auto`,
`Extra narrow` through `Widest`.

## Saved filters

The saved-filters menu in a table's filter bar lists the user's filters for that table, with
a star marking the default and a dot marking the currently-applied filter when its state no
longer matches what was saved ("The current filter differs from the saved one").

- **Save current filter…** opens a dialog asking for a filter name (up to 80 characters;
  a longer name is rejected with "Use 80 characters or fewer.") and an "Apply this filter
  when the table opens" checkbox to make it the default.
- **Apply** applies a saved filter's state to the table.
- **Update with current filter** overwrites a saved filter with the table's current state.
- **Rename** changes a filter's name.
- **Set as default** / **Remove as default**: at most one filter per table can be the
  default.
- **Delete** asks for confirmation ("Delete the saved filter "\<name\>"? This cannot be
  undone.") before removing it.

**The default filter applies once, when the table opens**, and only when the page's filter
state is still at its initial value; a filter state coming from the URL always wins over
the saved default. A user can save up to **50 filters per table**. Date ranges are saved
only when they are a relative preset (for example "last 24 hours" or "last 7 days"); an
absolute date range is dropped from what gets saved, since replaying a specific past window
on a later day is never what was intended.

## Selecting rows

The checkbox in the header selects **the rows on the current page** and nothing else. Once
any row is selected, the filter bar is replaced by the bulk bar: "N of M selected", where M is
every row matching the current search and filters, across all pages.

While fewer than all of them are selected, the bar offers **Select all N matching**. It
selects every row that matches the current search and filters, on every page, skipping rows
the table disables for selection. Bulk actions then act on exactly that set, and a
destructive one confirms first with the count: "Are you sure you want to remove 49 items?".
The link is not offered on tables the server pages (audit, activity, notifications and the
other long logs), where only the current page is loaded.

## Rows per page

The pager's rows-per-page choice (25, 50, 75 or 100) is saved with the column layout and
applied when the table opens, including when that layout arrives after the first rows are
already on screen.

## Personal, and stored on the server

Layouts and filters are keyed to the signed-in user, not to a browser or a role. Nobody else
can read or change another user's table views; a foreign id is a 404, not a 403, so a
mistaken id gives no information about whether it exists.

## API

The `modules/table_views` backend module exposes:

| Route | Purpose |
|---|---|
| `GET /api/table-views/{tableId}` | The user's layout (or `null`) and their filters for that table |
| `PUT /api/table-views/{tableId}/layout` | Save the column layout |
| `DELETE /api/table-views/{tableId}/layout` | Delete the saved layout (Reset to default) |
| `POST /api/table-views/{tableId}/filters` | Create a saved filter |
| `PUT /api/table-views/{tableId}/filters/{id}` | Update a saved filter's name, state or default flag |
| `DELETE /api/table-views/{tableId}/filters/{id}` | Delete a saved filter |
| `POST /api/table-views/import` | One-time import of browser-saved layouts and filters (below) |

These routes need **no permission key beyond being signed in**. They act only on the calling
session's own rows (the same exemption as `PUT /auth/preferences`), so there is nothing to
gate: any permission requirement here would only ever lock a user out of their own table
settings.

## One-time browser import

Before this feature, column choices and saved filter presets lived in the browser's local
storage. The first time a user opens a table that has no server-side layout or filters yet,
the frontend reads whatever that browser has stored for it, sends it to
`POST /api/table-views/import`, and then removes it from local storage. This happens once
per table per browser. After that, the server copy is the only copy, and it is what the
user sees everywhere they sign in.

## Related

- [Screens](../README.md): the application frame, including the shared "Lists" behaviour
  (search, facets, saved filters, pagination) that table views builds on.
