# Lifecycle Log: Actions

The one thing you can do on the Lifecycle Log, and the permission that gates it. The
screen is a record of what the daemon did: rows are expanded, not acted on.

| Action | Gated by | What it does |
|---|---|---|
| **Show details** | `events.view` | Expands a row to the event id, subject, event and action, and its source |

## Confirmation

Destructive actions confirm first and say how many items they will affect. A confirmation
that names a consequence ("this cannot be undone", "all its data") is quoting the
product, not padding.

## Related

- [Lifecycle Log](./README.md): what the screen is and what it shows
- [Fields](./fields.md): the inputs these actions read
- [Troubleshooting](./troubleshooting.md): when an action does not do what you expect
