# Runs and Debugging

`/workflows/:id/runs`

Every execution is recorded. This page is about reading what happened.

**Gated by** `workflows.run` to execute; the run list is visible to any authenticated session
(see the note in [Reference](../reference/README.md)).

## The run list

| Column | |
|---|---|
| Status | `completed`, `failed` or `cancelled` |
| Trigger | What started it |
| Nodes | How many node executions the run performed |
| Duration | Wall clock |
| Started | When |
| Error | The first error recorded, where there was one |

"No runs yet. Execute a workflow to see runs here." is literal: a workflow that should be
running on a schedule and has no runs is either inactive or its trigger never matched.

## "Completed" does not mean "everything worked"

A node failure does not fail the run. If a node failed but the walk finished, the status is
**`completed`** and the first error is recorded on the run.

That is deliberate (a five-branch workflow where one branch cannot reach a flaky API still
did four useful things), but it means the **Error** column is as important as the status. A
completed run with an error in it had a node fail.

The statuses that really are failures:

| Status | Cause |
|---|---|
| `failed` | The canvas would not parse, there was no entry node, the 2000-step limit was hit, or the named start node was missing |
| `cancelled` | The 5-minute run timeout elapsed, or the run was stopped |

## Watching a run live

The editor subscribes to `GET /api/workflows/{id}/live` over SSE and animates the canvas as
events arrive:

| Event | What you see |
|---|---|
| `run.started` | The run begins |
| `node.started` | The node lights up |
| `node.completed` | Its port, input, config, output and duration land in the Debug tab |
| `node.failed` | The node is marked failed with its error |
| `edge.traversed` | The edge animates |
| `debug` | A line in the debug stream |
| `run.completed` / `run.cancelled` | Final status, duration and error |

Because `node.completed` carries the input, the config **and** the output, the Debug tab is a
complete record of what each node saw and produced, not a reconstruction.

## The four debug surfaces

| Surface | Best for |
|---|---|
| **Debug tab** on a node | What this node received and emitted |
| **Errors tab** on a node | Why it failed |
| **Debug node** | A deliberate checkpoint in the flow |
| **Sniffer** on an edge | Which branch is actually carrying the message |

A **Log** node writes a line with the message attached, and any node with its debug flag set
emits one automatically. All four feed the same stream.

## A procedure that works

1. **Did it run at all?** No runs means the trigger did not fire. Check Active, then the
   trigger's own condition.
2. **Which node stopped it?** The canvas after a run shows where execution reached.
3. **What did that node actually receive?** Its Debug tab. That is usually the answer, because the
   path in a field does not match the shape the previous node emitted.
4. **Is the right branch firing?** Put a sniffer on each branch out of the deciding node.
5. **Is something disabled?** A disabled node or a blocked group stops a chain silently by
   design; a skipped node passes through with `_skipped`.
6. **Did it hit a limit?** `workflow exceeded 2000 execution steps` or a cancellation at five
   minutes both point at the shape of the flow rather than at one node.

## Timing

Every node execution stamps its own duration into `_performance` on the message, keyed by node
id, so the payload itself carries the timing. The Debug tab shows the same figure per node.

For a slow workflow, that map answers "which step" faster than the run duration does.

## Related

- [Execution model](../execution/README.md): why a failed node leaves a completed run
- [The message contract](../messages/README.md): reading what the Debug tab shows you
- [Troubleshooting](../troubleshooting/README.md): specific symptoms
