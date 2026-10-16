# Node Catalog

90 built-in node types, in five categories. Plugins and custom nodes add more.

| Category | Nodes | What they are for |
|---|---|---|
| [Trigger Nodes](./triggers/README.md) | 10 | A trigger is where a run starts. Every workflow needs at least one, and a workflow with no runnable entry node fails imm… |
| [Action Nodes](./actions/README.md) | 31 | Action nodes do something to the world: containers, images, networks, volumes, stacks, backups, storage and hosts.… |
| [Logic Nodes](./logic/README.md) | 13 | Logic nodes decide where a message goes, hold messages until something is ready, or bound how fast a workflow runs.… |
| [Utility Nodes](./utility/README.md) | 24 | Utility nodes shape data, store it, render it, or make a run easier to understand.… |
| [Integration Nodes](./integrations/README.md) | 12 | Integration nodes talk to something outside McHarbor: a mail server, a chat channel, a remote host, a DNS resolver, an … |

## How to read a node entry

Every entry gives the same three things:

- **The key**: `container-action`, `http-request`. This is what the canvas stores and what
  the backend dispatches on, and it is what appears in a workflow export.
- **The ports**: the input ports it accepts and the output ports it can emit on. A node with
  an `error` port can branch on failure; a node with named ports like `pass` / `block` or
  `true` / `false` routes rather than fails.
- **The fields**: every configuration field with its type, whether it is required, its
  default, its options where it has a fixed set, and the condition that reveals it where it
  is conditional.

Ports and fields are taken from each node's own definition under `nodes/<key>/index.ts`, so
they match what the editor renders.

## Where the rest come from

| Source | Key shape | Added by |
|---|---|---|
| Built-in | `container-action` | Ships with McHarbor |
| Custom | `my-node` | Written in the interface; see [Custom nodes](../../development/workflows/custom-nodes/README.md) |
| Plugin | `plugin:<slug>:<key>` | Contributed by an installed plugin |

The Store's **Nodes** tab lists all three and badges each one **Built-in**, **Downloaded** or
**Plugin**, naming the plugin that provides it. Availability can be toggled per key, and the
overrides persist in `$DATA_DIR/workflow-nodes/state.json`; a key that is absent counts as
enabled.

## Nodes that are unavailable

A node whose dependency is not configured renders **Unavailable** with the reason, rather
than failing at run time:

- "This node only works when at least one enabled email server is configured in Settings."
- "This node can use McHarbor internal notifications or an enabled communication channel
  configured in Settings."
- "This node only works when at least one compatible environment is configured in McHarbor."

**Show unavailable nodes** keeps them in the palette while you decide whether to configure
the dependency.

## Related

- [Execution model](../execution/README.md): how a message travels between these nodes
- [The message contract](../messages/README.md): `msg`, paths, and scoped variables
- [Developer](../../development/workflows/README.md): adding a node of your own
