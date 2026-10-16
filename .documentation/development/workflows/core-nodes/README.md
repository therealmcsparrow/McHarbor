# Core Nodes

A core node ships with McHarbor: a folder in the `nodes/` catalog that defines it for the
editor, and a case in the backend dispatcher that executes it.

Reach for this when the node needs the host's own credentials, its Docker clients, its
database or its services, which a sandboxed script deliberately cannot have.

## The two halves

### 1. The catalog folder

```
nodes/<key>/
  index.ts            the NodeDefinition the editor reads
  README.md           what it does, port behaviour, configuration guide
  backend-action.md   optional: notes on the backend binding
  i18n/<lang>.json    per-locale strings, six locales
```

`index.ts` exports one `NodeDefinition`:

```ts
export const containerAction: NodeDefinition = {
  key: 'container-action',
  label: 'Container Action',
  category: 'action',
  description: 'Start, stop, restart, remove, rename or wait for a container',
  icon: 'IconPlayerPlay',
  configSchema: [
    { key: 'environment', label: 'Environment', type: 'environment-select', required: true },
    { key: 'container',   label: 'Container',   type: 'container-select',   required: true },
    { key: 'action',      label: 'Action',      type: 'select', required: true, default: 'start',
      options: [{ value: 'start', label: 'Start' }, { value: 'stop', label: 'Stop' }] },
    { key: 'timeout',     label: 'Timeout (seconds)', type: 'number', default: 10,
      showWhen: { action: ['stop', 'restart', 'wait'] } },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
```

| Field | Notes |
|---|---|
| `key` | Lowercase, hyphenated. This is what the canvas stores and the backend dispatches on |
| `category` | `trigger`, `action`, `logic`, `utility` or `integration`. Decides the palette group |
| `configSchema` | Field types include `text`, `number`, `toggle`, `select`, and the resource pickers `environment-select` and `container-select`. A picker over a plugin's objects is never a core field type: a plugin node uses `plugin-select` against the plugin's own `picker-source` |
| `showWhen` | Reveals a field conditionally, keyed on another field's value or values |
| `inputPorts` / `outputPorts` | Triggers have no input. Most actions carry `output` and `error` |

The README beside it is not decoration: the editor's **Help** tab is the node's own
documentation, and a node without one is a node nobody can configure confidently.

### 2. The backend case

`ExecuteNode` in `src/backend/modules/workflows/service.go` dispatches on `node.Action` and
returns `(port string, out Msg, err error)`.

```go
case "container-action":
    envID, _ := node.Config["environment"].(string)
    if envID == "" {
        envID = fallbackEnvID
    }
    // …do the work…
    out := CloneMsg(msg)
    out = EnsureMsgID(out)
    SetPath(out, "payload.result", result)
    return "output", out, nil
```

Conventions that matter:

- **Resolve the environment the standard way:** the node's own `environment` field first,
  the run's fallback second. This is what lets a workflow be pointed at another environment
  without editing every node.
- **Clone, do not mutate.** `CloneMsg` then `EnsureMsgID`, or `NewMsg` for a fresh one.
- **Return a port, or an error, not both.** Returning `("", nil, err)` with no emissions is
  what routes into a try-catch guard.
- **Prefer an `error` port** over a bare error where the failure is something a flow author
  might reasonably branch on.
- **Emit many messages** by registering emissions on the run runtime rather than returning
  them. That is how `range` and `loop` fan out.

Nodes that need per-run state (buffering, counting, holding) use the execution runtime's
join and aggregate buffers rather than package-level state. Package state would leak between
concurrent runs.

## Registering it

The node folder is picked up by the catalog; the backend case makes it executable. A key
present in one and missing from the other is the usual cause of "the node is in the palette
and does nothing" or "the backend has a case nobody can reach".

## Renaming a node

Old canvases keep the old key. The dispatcher rewrites legacy keys to their current
equivalent at execution time, and the editor offers to persist the rewrite when the workflow
is opened. That is why a workflow saved against `docker-action` still runs after that node
became `container-action`.

If you rename a key, add the alias. Do not leave saved workflows to discover it.

## Availability

Every node key can be toggled in the Store's **Nodes** tab. Overrides persist to
`$DATA_DIR/workflow-nodes/state.json`, and an absent key counts as enabled, so shipping a
new node does not require touching that file.

Toggling is `store_nodes.manage`; reading the list is `store_nodes.view`.

## Checklist

- [ ] `nodes/<key>/index.ts` with a complete `configSchema` and honest ports
- [ ] `nodes/<key>/README.md`: what it does, port behaviour, every field
- [ ] `nodes/<key>/i18n/*.json` for the six locales, English carrying the real strings
- [ ] A case in `ExecuteNode` that resolves the environment, clones the message and returns a port
- [ ] A legacy alias if you renamed anything
- [ ] Run it once and read the Debug tab; the output shape is part of the contract

## Related

- [Custom nodes](../custom-nodes/README.md): the no-rebuild option
- [Plugin nodes](../plugin-nodes/README.md): nodes that ship with a plugin
- [Execution model](../../../workflows/execution/README.md): ports, emissions and error routing
- [The message contract](../../../workflows/messages/README.md): `CloneMsg`, `GetPath`, `SetPath`
