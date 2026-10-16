# Terminal: Actions

| Action | Gated by | What it does |
|---|---|---|
| **Connect** | `terminal.access` | Opens the session against the selected container and shell |
| **Disconnect** | `terminal.access` | Ends the session. Nothing is left running inside the container |
| **Reconnect** | `terminal.access` | Re-establishes a dropped session |

Terminal access is audited: what is recorded is that a session opened, by whom, against
which container, not a transcript of the keystrokes.

## Confirmation

Destructive actions confirm first and say how many items they will affect. A confirmation
that names a consequence ("this cannot be undone", "all its data") is quoting the
product, not padding.

## Related

- [Terminal](./README.md): what the screen is and what it shows
- [Fields](./fields.md): the inputs these actions read
- [Troubleshooting](./troubleshooting.md): when an action does not do what you expect
