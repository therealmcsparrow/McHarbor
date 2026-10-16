# Dashboard: Actions

Everything you can do to the dashboard grid (arrange it, add and remove widgets, resize
them, reset to a chosen layout), all of it behind one permission, `widgets.manage`.

| Action | Gated by | What it does |
|---|---|---|
| **Edit dashboard** | `widgets.manage` | Turns on arrangement: drag to move, resize, remove |
| **Add widget** | `widgets.manage` | The catalog, grouped by category |
| **Remove widget** | `widgets.manage` | Takes it off your grid |
| **Increase / decrease height** | `widgets.manage` | Per widget, in grid units |
| **Reset** | `widgets.manage` | Returns to the chosen layout |
| **Done editing** | `widgets.manage` | Locks the grid again |

## Confirmation

Destructive actions confirm first and say how many items they will affect. A confirmation
that names a consequence ("this cannot be undone", "all its data") is quoting the
product, not padding.

## Related

- [Dashboard](./README.md): what the screen is and what it shows
- [Fields](./fields.md): the inputs these actions read
- [Troubleshooting](./troubleshooting.md): when an action does not do what you expect
