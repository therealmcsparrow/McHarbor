# Workflows: Actions

| Action | Gated by | What it does |
|---|---|---|
| **New workflow** | `workflows.manage` | Name and optional description |
| **Activate** / **Deactivate** | `workflows.manage` | Turns auto-triggers on or off. Manual runs work either way |
| **Run** | `workflows.run` | Executes the workflow now and opens the run |
| **Stop** | `workflows.run` | Halts a running execution |
| **Open editor** | `workflows.manage` | The canvas |
| **Undo / Redo** | `workflows.manage` | `Ctrl+Z` and `Ctrl+Y` |
| **Add sniffer** | `workflows.manage` | Right-click a connection to tap what flows through that edge |
| **Delete** | `workflows.delete` | "This will permanently delete this workflow and all its data" |

Changes save as you work; the header shows "Saving…" while it does.

## Confirmation

Destructive actions confirm first and say how many items they will affect. A confirmation
that names a consequence ("this cannot be undone", "all its data") is quoting the
product, not padding.

## Related

- [Workflows](./README.md): what the screen is and what it shows
- [Fields](./fields.md): the inputs these actions read
- [Troubleshooting](./troubleshooting.md): when an action does not do what you expect
