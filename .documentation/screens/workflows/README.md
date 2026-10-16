# Workflows

`/workflows` · `/workflows/:id` · `/workflows/:id/runs`

A visual editor for automating container operations: nodes on a canvas, connected into a
chain, triggered by an event or a schedule.

## Permissions

| Key | What it unlocks |
|---|---|
| `workflows.view` | See workflows; the sidebar entry is gated on this key |
| `workflows.manage` | Create and edit |
| `workflows.run` | Execute a workflow |
| `workflows.delete` | Delete a workflow |

Everywhere except navigation, a control you lack the key for renders **disabled with a
tooltip naming it**, rather than disappearing. Navigation is the exception: a sidebar
entry you cannot use is hidden, because a dead-end page is not informative.

## What it shows

- One card per workflow: when it last ran, when it was last updated, its run count, and whether it is Active or Inactive.
- The editor: a node palette, a canvas, and a configuration panel with Settings, Help, Debug and Errors tabs.
- The run list for a workflow: status, trigger, nodes, duration, started and error.

## Active and inactive

Active and inactive is about **auto-triggers only**: an inactive workflow can still be run
by hand, it simply does not fire on its own. The toggle says so: "Workflow is inactive —
auto-triggers are disabled. Click to activate."

## The node palette

86 built-in node types, grouped as **Triggers**, **Actions**, **Logic**, **Utility**,
**Integrations**, and **Plugins** for anything an installed plugin contributes.

A node whose dependency is not configured shows as **Unavailable** with the reason in plain
words rather than failing at run time: "This node only works when at least one enabled
email server is configured in Settings", and similar for communication channels and
environments. **Show unavailable nodes** keeps them visible while you decide.

## Two switches instead of deleting a node

- **Skip node**: pass input through without executing it.
- **Disable node**: block the chain at this node.

Skip keeps the chain flowing; disable stops it. Both beat deleting a node to test
something, because the wiring survives.

## Why run is its own permission

`workflows.run` is separate from `workflows.manage` so an operator can run an automation
someone else built without being able to change what it does.

## In this folder

| Page | What it covers |
|---|---|
| [Actions](./actions.md) | Everything you can do here, what gates it, and what it really does |
| [Fields](./fields.md) | Every column, filter and form field the screen exposes |
| [Troubleshooting](./troubleshooting.md) | Symptom, cause and fix |

## Related

- [Workflows](../../workflows/README.md): the execution model, the message contract, every node, and building your own

- [Settings → Automation → Schedules](../../settings/automation/schedules/README.md): simpler recurring actions
- [Settings → Automation → Webhooks](../../settings/automation/webhooks/README.md): outbound callbacks
- [Store](../store/README.md): the node catalog, including plugin-contributed nodes
