# Your First Workflow

A worked example, and the handful of things that make the difference between a workflow that
runs and one that runs reliably.

**You need** `workflows.manage` to build it and `workflows.run` to execute it.

## Build it

**Workflows → New workflow**, give it a name, open the editor.

### 1. A trigger

Drag **Manual Trigger** onto the canvas. Every workflow needs an entry node. Without one the
run fails immediately with `no runnable entry nodes`.

Start manual even when the real trigger will be a schedule or a webhook. You want to run this
on demand a dozen times while you build it.

### 2. Something that does work

Drag **Container Action** on, and drag from the trigger's `output` port to its `input` port.
Set the environment, the container, and `action: restart`.

The edge is the only thing that makes one node run after another. There is no implicit order.

### 3. A failure path

Container Action has two output ports, `output` and `error`. Wire `error` to a
**Send Internal Notification** node.

This is the step people skip. An unwired `error` port means a failure is recorded on the run
and nothing tells anybody.

### 4. Run it

**Run**. The canvas animates as the run walks it, and the **Debug** tab on each node fills
with what that node received and emitted.

Read the Debug tab on the action node even when it worked. What it put on `msg` is what every
downstream node will have to work with.

## Then make it real

### Swap the trigger

Add **Schedule Trigger**, wire it to the same action, and set its cron. Its **timezone** field
is its own; it does not read the app-wide time zone the way schedules elsewhere do.

Then **Activate** the workflow. Active arms automatic triggers; inactive still runs by hand.

### Keep the manual trigger

Leave it wired in. Being able to run the thing on demand at 3am is worth the canvas space.

## Five things worth knowing early

**Every branch gets its own copy of the message.** Two branches from one port cannot corrupt
each other, and a change made in one is invisible in the other.

**A failed node does not fail the run.** If the walk finishes, the status is `completed` and
the first error is recorded on the run. Read the Error column, not just the status.

**Skip and disable are different.** Skip passes the message through untouched; disable stops
the chain there. Both beat deleting a node while you are testing.

**A run is capped** at 2000 node executions and five minutes. If you are iterating a large
list, handle a batch per run rather than an item per step.

**Paths are map-only.** `payload.items.0.name` does not index into an array. Use **Loop**,
**Range** or **JSON Path** for array work.

## What to reach for next

| You want to | Use |
|---|---|
| Branch on a value | **Condition**, **Switch**, **Filter** |
| Guard a whole region against failure | **Try Catch**, with something wired to `catch` |
| Work through a list | **Loop** or **Range** |
| Wait for several branches | **Join** |
| Call something outside McHarbor | **HTTP Request**, **GraphQL Request**, **SSH Exec** |
| Tell somebody | **Send Communication**, **Send Email**, **Send Internal Notification** |
| Reshape data with no built-in for it | A [custom node](../../development/workflows/custom-nodes/README.md) |

## Before you rely on it

- Wire the `error` ports on anything that talks to the outside world.
- Put the environment-specific values in the workflow's **Variables** rather than in each
  node, so the canvas exports cleanly to another install.
- Run it once with a sniffer on the branch you are least sure about.
- Check the run list afterwards: a `completed` run with an error in it is the case to look at.

## Related

- [The editor](../editor/README.md): the canvas, the palette, sniffers and groups
- [Execution model](../execution/README.md): what happens when you press Run
- [The message contract](../messages/README.md): what flows between nodes
- [Node catalog](../nodes/README.md): all 90 built-in node types
