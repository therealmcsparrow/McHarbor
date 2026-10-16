# Docker: Actions

| Action | Gated by | What it does |
|---|---|---|
| **Add registry** | `registry.manage` | Name, URL, username, password, and whether it is the default |
| **Edit registry** | `registry.manage` | Update connection settings |
| **Test connection** | `registry.manage` | A real round trip, reporting latency in milliseconds |
| **Remove registry** | `registry.manage` | Confirms first |

Registry passwords are encrypted at rest and never returned by the API once saved.

One registry can be marked **Default**, which is what an unqualified pull uses.

## Confirmation

Destructive actions confirm first and say how many items they will affect. A confirmation
that names a consequence ("this cannot be undone", "all its data") is quoting the
product, not padding.

## Related

- [Docker](./README.md): what the screen is and what it shows
- [Fields](./fields.md): the inputs these actions read
- [Troubleshooting](./troubleshooting.md): when an action does not do what you expect
