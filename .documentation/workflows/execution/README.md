# Execution Model

What happens between pressing **Run** and a run appearing in the list. This is the page to
read before changing a node, and the page to reach for when a workflow does something you did
not expect.

## The loop

A run is a **breadth-first walk over a queue of messages**, not a graph traversal:

1. The canvas JSON is parsed into nodes, edges and groups. A canvas that will not parse fails
   the run immediately with `invalid workflow canvas`.
2. Legacy node keys are rewritten to their current equivalents.
3. An adjacency map is built from `<nodeID>:<port>` to the edges leaving it, skipping edges
   whose source port is blocked.
4. The start node is queued. `link-in` nodes are queued too when the run was started that way.
   An empty queue at this point fails with `no runnable entry nodes`.
5. The loop pops one item, executes the node, and queues the targets of whatever it emitted,
   **each with its own deep copy of the message**.
6. It stops when the queue is empty, the step limit is hit, or the context is cancelled.

Because every branch gets a deep copy, two branches cannot corrupt each other's message. It
also means mutating a message in one branch is invisible in the other.

## Limits

| Limit | Value | What happens |
|---|---|---|
| Steps per run | **2000** | The run fails with `workflow exceeded 2000 execution steps` |
| Run timeout | **5 minutes** by default | The run is marked `cancelled` and reports the context error |

The step limit is what stops a cycle from running forever. A loop that legitimately needs more
than two thousand node executions needs restructuring, usually by handling a batch per run
rather than an item per step.

## Ports and emissions

A node returns one output port and one message, or registers **emissions** (several
port-tagged messages) with the run-scoped runtime. The executor takes whichever exists:

- No emissions and a port → one message on that port.
- Emissions → each one is fanned out to the edges leaving that port.
- Several emissions and no single output → the node's display output becomes a `_batch`
  summary with a count and the port, so the editor shows something meaningful.

`range`, `loop`, `join` and `aggregate` are the runtime-aware nodes: the first two emit many
messages, and the last two buffer incoming ones until they are ready.

### Blocked ports

A node can have individual ports blocked, and a blocked port is enforced at both ends:

- `out:<port>`: the edge is dropped when the adjacency map is built, so nothing is ever sent.
- `in:<port>`: the message is not delivered even though the edge traversal is reported.

This is how you silence one branch without deleting the wiring.

## Skip, disable, and blocked groups

Three ways to take a node out of the flow, each with a different effect:

| State | What happens | Effect on the chain |
|---|---|---|
| **Skip** | The input message is passed through on the node's first output port, tagged `_skipped: true` | The chain keeps flowing |
| **Disable** | The node reports completion with `{_disabled: true}` and emits nothing | The chain stops here |
| **Blocked group** | Every node in the group behaves as disabled | Those branches stop |

Skip keeps the wiring and the downstream behaviour; disable is a wall. Both beat deleting a
node while you are testing, because the edges survive.

## Errors

A node can fail in two ways, and they are routed differently.

**It returns an error and emits nothing.** The executor publishes `node.failed` with
"Node execution failed", then looks for an enclosing try-catch frame. Found, the message is
routed to that guard's `catch` port. Not found, the run records the error and **continues with
the rest of the queue**. A failed node does not stop other branches.

**It emits on its `error` port.** If nothing is wired to that port, the executor still looks
for a try-catch frame before dropping it. So an unwired `error` output is not silently lost
when a guard is in scope.

### Try Catch

The `try-catch` node pushes a frame onto `_tryCatchFrames` in the message: the guard's node id
and the property the error should be written to. Frames stack, and the **innermost guard with
something wired to its `catch` port** wins.

When a frame catches, the message that reaches the catch branch carries:

```json
{
  "_caught": {
    "caught": true,
    "guardNodeId": "node-3",
    "nodeId": "node-9",
    "nodeLabel": "HTTP Request",
    "action": "http-request",
    "message": "dial tcp: lookup api.example.com: no such host"
  }
}
```

The same object is written to the configured error property, and the caught frame is popped so
a second failure looks for the next guard out.

## Run status

| Status | When |
|---|---|
| `completed` | The queue drained |
| `failed` | The canvas would not parse, there was no entry node, the step limit was hit, or the start node was missing |
| `cancelled` | The run timed out or the context was cancelled |

**A node failure does not fail the run.** If nodes failed but the walk finished, the status is
`completed` and the first error is recorded on the run. That is deliberate (a five-branch
workflow where one branch cannot reach a flaky API still did four useful things), but it means
"completed" is not the same as "everything worked". The run's error field and the Errors tab
are where that shows.

## What a run produces

Alongside the status: the duration, how many nodes executed, the last output message, and,
when a `webhook-response` or `http-response` node ran, a captured HTTP response the caller
receives.

Every node execution also stamps its own timing into `_performance` on the message, keyed by
node id, so a slow step is visible in the payload itself.

## Live events

The executor publishes events as it goes, which the editor consumes over SSE from
`GET /api/workflows/{id}/live`:

| Event | Carries |
|---|---|
| `run.started` | Status |
| `node.started` | Node id, label, action |
| `node.completed` | Port, input, config, output, duration |
| `node.failed` | Node id, label, action, error |
| `edge.traversed` | Edge id, source and target |
| `debug` | Node id, label, level, message, data, source |
| `run.cancelled`, `run.completed` | Status, duration, error |

`debug` events come from three places: a **Debug** node, a **Log** node, and a **sniffer** on
an edge, which reports "Sniffer [name]: data flowing from A to B" with the message attached.
Any node with its debug flag set also emits one.

## Environment resolution

A node's `environment` config field wins. Where it is empty, the run's fallback environment is
used: the environment the run was started against. This is why a workflow built on one
environment can often be pointed at another without editing every node.

## Related

- [The message contract](../messages/README.md): what is actually being copied between nodes
- [Triggers](../triggers/README.md): how a run starts
- [Runs and debugging](../runs/README.md): reading what happened
- [Node catalog](../nodes/README.md): every node's ports
