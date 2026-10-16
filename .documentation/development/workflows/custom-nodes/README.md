# Custom Nodes

A custom node is JavaScript you write in McHarbor, stored by the host and run in a sandboxed
goja VM. No rebuild, no restart, no Go.

**Gated by** `custom_nodes.view` to see them, `custom_nodes.manage` to create, change, test
and delete.

## The shape of one

| Field | What it is |
|---|---|
| `key` | The node key the canvas stores |
| `label`, `description`, `icon`, `category` | How it appears in the palette |
| `source` | Always `custom` |
| `configSchema` | The fields the editor renders |
| `inputPorts`, `outputPorts` | Its ports |
| `code` | The script |
| `translations` | Optional per-locale strings |

A config field takes `key`, `label`, `type`, `required`, `options`, `default` and `showWhen`,
the same conditional-visibility mechanism the built-in nodes use.

## The script

Your code runs inside an IIFE, so `return` works at the top level.

```js
// Available: msg, config, node, console
const status = msg.payload && msg.payload.status;

node.log('status was ' + status);

if (!status) {
  return { port: 'error', msg: { ...msg, payload: { error: 'no status on payload' } } };
}

msg.payload.normalised = String(status).toUpperCase();
return { port: 'output', msg };
```

### What is in scope

| Binding | What it gives you |
|---|---|
| `msg` | The incoming message, the same object the rest of the workflow passes around |
| `config` | This node's configured field values |
| `node.log()`, `node.warn()`, `node.error()` | Captured log lines |
| `console.log()`, `.warn()`, `.error()` | The same, for muscle memory |

Up to **100 log lines** are captured per execution; beyond that they are dropped.

### What to return

Either an object with `port` and `msg`:

```js
return { port: 'output', msg };
```

…or modify `msg` in place and return it. Anything else fails with "script must return an
object with {port, msg} or modify msg directly".

## The sandbox

| Property | Value |
|---|---|
| Engine | goja, a pure-Go ECMAScript VM |
| Timeout | **10 seconds** by default, **60 seconds** maximum |
| On timeout | The VM is interrupted and the node fails with "script timed out" |
| Log lines | 100 per execution |

**There is no host bridge.** No `fetch`, no `require`, no filesystem, no network, no Docker.
The VM is given exactly `msg`, `config`, `node` and `console`. A custom node transforms data;
it does not reach out.

When you need to reach out, do it with a built-in node (**HTTP Request**, **SQL Query**,
**Send Communication**) and use a custom node to shape what goes in and comes out. That split
is also what keeps a script's failure modes to "bad data" rather than "half-finished network
call".

The timeout is real and the interrupt is real: an accidental infinite loop costs you ten
seconds and a failed node, not a wedged McHarbor.

## Testing

**Test** on the editing screen runs the script against a message and config you supply, in the
same VM, and returns the port, the message and the captured log lines. It needs
`custom_nodes.manage`.

Use it. A script that has never been run against a realistic `msg` is a guess about a shape.

## Where custom nodes appear

They show in the palette like built-ins, and in the Store's **Nodes** tab badged
**Downloaded**. Availability can be toggled per key, and those overrides live in
`$DATA_DIR/workflow-nodes/state.json`, where an absent key counts as enabled.

Plugin-contributed nodes use a composite key, `plugin:<slug>:<key>`, and are listed alongside
them; see [Plugin nodes](../plugin-nodes/README.md).

## When to use which

| Need | Use |
|---|---|
| Reshape, validate or branch on data | A custom node |
| Something built-in already does | The built-in node |
| A network call, a container action, a delivery | A built-in node, optionally around a custom one |
| Something the host must do, with host credentials | A [core node](../core-nodes/README.md) or a [plugin node](../plugin-nodes/README.md) |

## Related

- [The message contract](../../../workflows/messages/README.md): what `msg` is and how paths work
- [Core nodes](../core-nodes/README.md): adding a node in Go
- [Plugin nodes](../plugin-nodes/README.md): nodes a plugin contributes
- [Reference](../../../workflows/reference/README.md): the custom-node API routes
