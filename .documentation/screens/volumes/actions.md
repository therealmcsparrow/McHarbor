# Volumes: Actions

The three things you can do to a volume (create, remove and prune) and the permission
each one needs. Both removals take the data with them, and prune takes every volume no
container is currently attached to.

| Action | Gated by | What it does |
|---|---|---|
| **Create volume** | `volumes.manage` | Name and driver; `local` unless you have a volume plugin |
| **Remove** | `volumes.delete` | "This will permanently remove this volume and all its data" |
| **Prune unused** | `volumes.manage` | Removes every volume no container is attached to |

## Confirmation

Destructive actions confirm first and say how many items they will affect. A confirmation
that names a consequence ("this cannot be undone", "all its data") is quoting the
product, not padding.

## Related

- [Volumes](./README.md): what the screen is and what it shows
- [Fields](./fields.md): the inputs these actions read
- [Troubleshooting](./troubleshooting.md): when an action does not do what you expect
