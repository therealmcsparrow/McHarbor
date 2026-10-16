# The Message Contract

Nodes do not call each other. They hand each other one object, `msg`, and everything else
follows from that.

## What `msg` is

A plain map. Two keys are conventional rather than enforced:

| Key | Meaning |
|---|---|
| `_msgid` | A unique id, assigned when the message is created and preserved as it travels |
| `payload` | The thing the message is *about*: the body, the result, the item |

Anything else a node wants to attach sits beside them. A webhook trigger adds `req` and
`headers`; a schedule trigger adds `cron` and `timezone`; every node adds its own timing to
`_performance`.

```json
{
  "_msgid": "cv1k2p8s9a",
  "topic": "webhook",
  "payload": { "action": "deploy", "service": "api" },
  "req": { "method": "POST", "headers": { "content-type": "application/json" } },
  "_performance": { "node-1": { "durationMs": 4 } }
}
```

## Copies, not references

Every message handed to a downstream node is a **deep copy**, produced by a JSON round trip.
Three consequences:

- Two branches from one port cannot corrupt each other.
- Anything that does not survive JSON (a function, a cyclic reference) does not survive the
  hop.
- Very large payloads are copied once per outgoing edge. A node fanning a 50 MB payload to
  four branches is copying 200 MB.

Where the copy fails, the executor falls back to a shallow copy rather than dropping the
message.

## Paths

Fields that reference data take **dot-notation paths**, resolved against the message:

```
payload.status
payload.items.0.name        →  no. See below.
req.headers.content-type
```

Path resolution walks maps only. `payload.items.0` does **not** index into an array; each
segment has to be a map key. To work with array elements, use a node that iterates (`loop`,
`range`) or one that reads structure (`json-path`, `object`).

Writing a path creates the intermediate maps it needs, so setting `result.summary.total` on a
message that has neither `result` nor `summary` works. Deleting one removes just the leaf.

## Scopes

Three places to keep a value, in increasing lifetime:

| Scope | Lives | Set by |
|---|---|---|
| `msg` | One message, one branch | Any node that writes to the message |
| `flow` | One run, shared across branches | **Set Variable**, **Change** |
| `global` | One run, alongside flow | **Change** with global scope |

Flow variables start from the workflow's own **Variables** (a JSON object saved on the
workflow), and the run adds `_workflowId` to them. A malformed variables document is treated
as empty rather than failing the run.

Flow and global are per run. Nothing carries between runs. For state that has to outlive a
run, use the **KV Store** node, or write it somewhere real.

## Topic

`topic` is a conventional label rather than routing: triggers set it (`manual-trigger`,
`webhook`, `schedule`) so downstream nodes and debug output can tell where a message came
from.

## Reserved keys

Keys beginning with `_` are the executor's own bookkeeping. Reading them is fine; writing them
is asking for trouble:

| Key | Set by | Meaning |
|---|---|---|
| `_msgid` | Message creation | Identity |
| `_performance` | Every node | `{ "<nodeId>": { "durationMs": n } }` |
| `_skipped` | A skipped node | The message passed through without executing |
| `_tryCatchFrames` | `try-catch` | The stack of active guards |
| `_caught` | A catch | Which node failed, under which guard, with what message |
| `_batch` | The executor | A summary when a node emitted several messages |
| `_triggerTimestamp` | Webhook trigger | When the request arrived |

## Working with the real shape

The fastest way to learn what a node emits is the **Debug** tab on
the node after a run, which shows input, config, output and data as they actually were. A
sniffer on an edge does the same for what is in flight.

When a downstream node reports a missing value, the question is almost always "what did the
previous node actually emit", and that tab answers it in one look.

## Related

- [Execution model](../execution/README.md): how these messages move
- [Node catalog](../nodes/README.md): what each node reads and writes
- [Runs and debugging](../runs/README.md): inspecting messages after the fact
- [Custom nodes](../../development/workflows/custom-nodes/README.md): writing JavaScript against `msg`
