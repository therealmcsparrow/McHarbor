# Workflows

A workflow is a graph of nodes on a canvas. Something triggers it, a message travels along the
edges, and each node does its part of the work.

It is the answer when a schedule or a webhook is not enough: when the automation has to branch,
retry, wait for several things, handle its own failures, and leave a record of what it did.

**Gated by** `workflows.view` to see them, `workflows.manage` to build, `workflows.run` to
execute, `workflows.delete` to remove. Run is separate from manage on purpose: an operator can
run an automation someone else built without being able to change what it does.

## In this section

| Folder | What it covers |
|---|---|
| [Getting started](./getting-started/README.md) | A worked first workflow, and the five things worth knowing early |
| [The editor](./editor/README.md) | Canvas, palette, the configuration panel, sniffers, groups, skip and disable |
| [Execution model](./execution/README.md) | The queue, ports and emissions, limits, error routing, run statuses |
| [The message contract](./messages/README.md) | `msg`, paths, scopes, reserved keys |
| [Triggers](./triggers/README.md) | The nine trigger types, webhooks, schedules, links |
| [Runs and debugging](./runs/README.md) | Reading what happened, and the four debug surfaces |
| [Self-protection](./self-protection/README.md) | Nodes that act on McHarbor's own containers, stacks or host, and the `host_self.manage` check on save |
| [Node catalog](./nodes/README.md) | All 90 built-in node types, by category, with ports and fields |
| [Developer](../development/workflows/README.md) | Custom nodes, core nodes, plugin nodes |
| [Reference](./reference/README.md) | API routes and gates, the data model, the event stream, limits |
| [Troubleshooting](./troubleshooting/README.md) | Symptom, cause, fix |

## The model in six sentences

A run is a **queue**, not a traversal: the executor pops a message, runs a node, and queues
whatever that node emitted to the nodes downstream of the port it emitted on.

Every downstream message is a **deep copy**, so branches cannot corrupt one another.

A node returns **one port and one message**, or registers several port-tagged messages when it
needs to fan out.

A node that **fails** routes to the nearest enclosing **Try Catch** guard if there is one; if
not, the error is recorded and the rest of the queue keeps going, so a failed node does not
fail the run.

A run is capped at **2000 node executions** and **five minutes** by default.

Everything the editor shows you while it runs (nodes lighting up, edges animating, the Debug
tab filling) is one SSE stream of events the executor publishes as it goes.

## What makes this different from the other automation

| Feature | Use it when |
|---|---|
| [Schedules](../settings/automation/schedules/README.md) | One container action on a cron expression |
| [Webhooks](../settings/automation/webhooks/README.md) | Fire an HTTP callback when something happens |
| **Workflows** | Anything with branching, several steps, error handling, or a record of each run |

A schedule and a webhook have no run history and no error handling.

## Where nodes come from

| Source | Key shape | Added by |
|---|---|---|
| Built-in | `container-action` | Ships with McHarbor (90 of them) |
| Custom | `my-node` | Written in the interface, JavaScript, sandboxed |
| Plugin | `plugin:<slug>:<key>` | Contributed by an installed plugin |

All three appear in the same palette and the same Store tab, and all three can be toggled off
per key.

## Related

- [Workflows screen](../screens/workflows/README.md): the operator's view of the list and the editor
- [Plugins](../plugins/README.md): plugin-contributed nodes and triggers
- [Settings → Automation](../settings/automation/README.md): the simpler cousins
