# Logs: Actions

| Action | Gated by | What it does |
|---|---|---|
| **Live** | `logs.view` | Stream new lines as they arrive over a WebSocket. Off, the view is a snapshot |
| **Auto-scroll** | `logs.view` | Follow the tail. Turning it off holds your position while lines keep arriving |
| **Newest first** / **Oldest first** | `logs.view` | Order of the buffer |
| **Export** | `logs.view` | Download what is loaded |

Auto-scroll and Live are separate on purpose: streaming with auto-scroll off is how you
read something that happened ten seconds ago while the container keeps talking.

## Confirmation

Destructive actions confirm first and say how many items they will affect. A confirmation
that names a consequence ("this cannot be undone", "all its data") is quoting the
product, not padding.

## Related

- [Logs](./README.md): what the screen is and what it shows
- [Fields](./fields.md): the inputs these actions read
- [Troubleshooting](./troubleshooting.md): when an action does not do what you expect
