# Networks: Actions

Everything you can do to a network (create, connect, disconnect and remove, singly or in
bulk) and the permission each one needs. A bulk removal fails per network, so an attached
one does not block the rest.

| Action | Gated by | What it does |
|---|---|---|
| **Create network** | `networks.manage` | The dialog adapts to the driver and says when one needs no extra configuration |
| **Edit** | `networks.manage` | The pencil in the list's actions column, or on the network's page. Opens the network in edit mode; saving recreates it and reconnects its containers. Disabled for built-in networks such as `bridge`, `host` and `none` |
| **Connect** | `networks.manage` | Attach a container to the network |
| **Disconnect** | `networks.manage` | Detach a container |
| **Remove** | `networks.delete` | Permanent, and confirms first |

## Bulk actions

Select rows and the bulk bar reports "N of M selected" with the actions that apply.
Progress is reported as it runs ("12 of 20 complete (11 succeeded, 1 failed, 0 running)"),
so a partial failure stays visible instead of being rolled into one toast.

| Action | What it does |
|---|---|
| **Remove** | Names the networks; warns up front where some are still attached to running containers. Removal fails per-network, so the rest of the selection still goes |

## Confirmation

Destructive actions confirm first and say how many items they will affect. A confirmation
that names a consequence ("this cannot be undone", "all its data") is quoting the
product, not padding.

## Related

- [Networks](./README.md): what the screen is and what it shows
- [Fields](./fields.md): the inputs these actions read
- [Troubleshooting](./troubleshooting.md): when an action does not do what you expect
