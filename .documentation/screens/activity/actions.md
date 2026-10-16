# Activity: Actions

The one thing you can do on Activity, and the permission that gates it. The screen is a
record rather than a control surface: events are read, not acted on.

| Action | Gated by | What it does |
|---|---|---|
| **View details** | `activity.view` | Opens the full event: type, container id, and the attribute map |

## Confirmation

Destructive actions confirm first and say how many items they will affect. A confirmation
that names a consequence ("this cannot be undone", "all its data") is quoting the
product, not padding.

## Related

- [Activity](./README.md): what the screen is and what it shows
- [Fields](./fields.md): the inputs these actions read
- [Troubleshooting](./troubleshooting.md): when an action does not do what you expect
