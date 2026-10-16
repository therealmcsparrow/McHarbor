# The Editor

`/workflows/:id`

A canvas, a node palette, and a configuration panel.

**Gated by** `workflows.manage` to edit, `workflows.run` to execute.

## Building a flow

Drag a node from the palette onto the canvas, then drag from an **output port** on its right
to an **input port** on the left of another. That edge is the only thing that makes one node
run after another; there is no implicit order.

Changes save as you work; the header shows "Saving…" while it does.

| Shortcut | |
|---|---|
| `Ctrl+Z` / `Ctrl+Y` | Undo / redo |
| `Delete` | Delete selected |
| `Ctrl+S` | Save |
| Drag / scroll | Pan and zoom |

## The palette

Five categories (**Triggers**, **Actions**, **Logic**, **Utility**, **Integrations**) plus
**Plugins** for anything an installed plugin contributes. Search filters by name.

A node whose dependency is not configured shows as **Unavailable** with the reason spelled
out rather than failing later at run time. **Show unavailable nodes** keeps them visible while
you decide whether to configure the dependency.

## The configuration panel

Select a node and the right-hand panel offers four tabs:

| Tab | What is there |
|---|---|
| **Settings** | The node's own fields, plus its label and its port labels |
| **Help** | Requirements, behaviour and technical details for that node type |
| **Debug** | The execution trace and the payload at that node: input, config, output, data |
| **Errors** | Errors from execution, or "No errors." |

The **Help** tab is per node type and worth reading before guessing at a field; the **Debug**
tab is the fastest way to learn what a node actually emits.

## Skip and disable

Two switches that change how a node behaves without deleting it:

- **Skip node**: "Pass input through without executing".
- **Disable node**: "Block the chain at this node".

Skip keeps the chain flowing; disable stops it there. Both are better than deleting a node to
test something, because the wiring survives.

## Sniffers

Right-click an edge to add a **sniffer** or delete the edge. A sniffer publishes what flows
through that edge into the debug stream: "Sniffer [name]: data flowing from *Condition* to
*Send Email*" with the message attached.

It is the tool for "the wrong branch is firing": put one on each branch and watch which
carries the message.

## Groups

Nodes can be collected into a named, coloured **group**. A group can be **blocked**, and every
node inside a blocked group behaves as disabled for the whole run. That takes a whole
region of a flow out without touching each node.

## Ports

Port labels are editable per node, which matters on nodes with several outputs: renaming
`case_0` to `production` makes a Switch readable six months later.

Individual ports can be blocked. A blocked **output** port means the edge is never traversed;
a blocked **input** port means the message is not delivered even though the traversal shows.
Both survive in the saved canvas, so a blocked port is a deliberate state rather than an
accident of layout.

## Running from the editor

**Run** executes the workflow now, whether or not it is active, and the canvas animates:
nodes light as they start and complete, edges light as they are traversed, and the Debug tab
fills. **Stop** halts a running execution.

## Import and export

A workflow exports as a portable JSON document carrying its name, description, status, canvas
and variables, along with the original ids and timestamps. Import creates a new workflow from
one. Both need `workflows.manage`.

This is how a workflow moves between McHarbor installs, and how one gets into version control.

## Variables

A workflow carries a JSON **Variables** object, which seeds the `flow` scope for every run.
Use it for the things that differ between installs (a base URL, a channel name) so the
canvas itself stays portable.

## Related

- [Execution model](../execution/README.md): what the canvas means at run time
- [The message contract](../messages/README.md): what flows along those edges
- [Node catalog](../nodes/README.md): every node, its ports and its fields
- [Runs and debugging](../runs/README.md): after the run
