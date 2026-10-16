# Build a workflow node

A node is one step in a workflow. There are three ways to add one, and picking the wrong one
costs a rewrite.

| Kind | Write | Choose when |
|---|---|---|
| **Built-in** | A definition in the root `nodes/` catalog, plus Go execution | The node needs the Docker SDK, host services, or logic that belongs in the product |
| **Custom** | JavaScript, through the API, no Go at all | The node is glue: transform, call something, decide. Runs in a sandboxed VM |
| **Plugin** | A definition inside a plugin | The node belongs to a plugin's feature |

**Start by asking whether it needs Go.** If it does not, a custom node is a fraction of the
work and needs no rebuild.

## The message contract

Everything hinges on this. Nodes communicate by passing a `msg` object along the wires.

| Field | Meaning |
|---|---|
| `_msgid` | A unique id, generated when the flow starts |
| `topic` | A routing key, set by the trigger |
| `payload` | The main data container: what nodes read and write |
| `req`, `res` | HTTP request and response metadata, from webhook triggers |
| `statusCode`, `headers` | From HTTP request nodes |
| `parts` | Split and join metadata, from loop nodes |
| `_performance` | Per-node execution timing |

The rules by node kind are not stylistic. The engine and the debugger both depend on them:

| Kind | Does |
|---|---|
| **Trigger** | Creates the initial message and sets its topic |
| **Action** | Clones the incoming message and updates the payload with its result |
| **Logic** | Passes the message through, or transforms it; a loop sets the parts metadata |
| **Utility** | Side effect only (debug, log), and passes the message through unchanged |

**Clone, do not mutate.** An action that writes into the incoming message corrupts every other
branch, because branches share the object until it is copied.

Nodes that set variables can write into the message scope, the flow scope or the global scope
through the flow context. Reach for the narrowest one that works.

## Built-in nodes

### Layout

```
nodes/<node-key>/
├── index.ts
└── i18n/
    ├── en.json
    ├── nl.json
    ├── de.json
    ├── es.json
    ├── fr.json
    └── pt.json
```

Nodes are **auto-imported** by glob, and the i18n bundles are auto-loaded the same way. There
is no registry file to edit: create the folder and the node appears in the palette, sorted by
category then by translated label.

### Steps

1. **Choose a category**: `trigger`, `action`, `logic`, `utility` or `integration`.
2. **Choose an icon** from the Tabler set, and add it to the palette's icon map. This is the
   one registration step that is not automatic.
3. **Create the folder** with `index.ts` and all six locale files.
4. **Implement execution** in the backend's node execution switch, if the node does work the
   frontend cannot.

### Config fields

The definition declares its configuration fields, and the editor renders them by type. The
list is long because the specific types do real work:

`text`, `textarea`, `number`, `select`, `toggle`, `json`, `key-value`, `expression`, `cron`,
`code`, `container-select`, `environment-select`, `stack-select`, `image-select`,
`volume-select`, `registry-select`, `storage-location-select`, `communication-channel-select`,
`email-server-select`, `image-scanner-select`, `webhook-select`, `link-output-select`,
`metric-conditions`.

**Never use `text` for something that is not free text.** A message path is an `expression`; a
known set of choices is a `select`, with an escape hatch flag when a custom value is
legitimate; a list is a `multiselect`. Using `text` for these works in the demo and fails the
first time a user types a plausible variant.

Where a field selects a resource, it is environment-aware and needs the environment field
alongside it.

### Ports

A node declares its input and output ports. Multiple outputs are how a node branches: a
condition node with a true and a false port, a switch with one port per case. The engine walks
the wire from the port the node returned.

### Colours

Canvas colours come from the workflow theme tokens, never from hardcoded values. A node with
its own colour looks wrong in every theme but one.

## Custom nodes

Custom nodes need no Go and no rebuild. The definition and its JavaScript are stored on the
data volume and executed in a sandboxed VM.

| Endpoint | Does |
|---|---|
| `GET /api/custom-nodes` | List, with code and translations |
| `POST /api/custom-nodes` | Create one |
| `PUT /api/custom-nodes/{key}` | Update |
| `DELETE /api/custom-nodes/{key}` | Delete |
| `POST /api/custom-nodes/test` | **Run the script in the sandbox against a supplied message and config, saving nothing** |

The test endpoint is the whole development loop. It is what the editor's Run button calls.
Write, test, iterate, then save.

A custom node's script receives the message and the node's configuration, and returns the
message it wants to pass on. The same clone-do-not-mutate rule applies.

The sandbox is a sandbox: there is no filesystem, no arbitrary network client, no access to
host internals. If the node needs those, it is a built-in or a plugin node.

## Plugin nodes

```
nodes/<key>/index.ts
nodes/<key>/i18n/<lang>/<namespace>.json
```

Default-export a node definition matching the built-in contract, and list the key in the
manifest's `contributes.nodes`. The host picks it up after install through the same glob, and
the node bridge registers it in the palette.

See [Build a plugin](../plugin/README.md).

## Strings

Every label, description, field label, placeholder and help string goes through the node's
namespace. All six locales get the key in the same change, English real and the rest falling
back until translated. The palette, the config panel and the help tab all read from the same
bundle.

## Testing a node end to end

1. Drop it on a canvas with a manual trigger in front and a debug node behind.
2. Run the workflow and read the debug output. That is what the message actually looks like,
   as opposed to what you intended.
3. Check the run in the run list. **A `completed` run does not mean everything worked**: a
   failed node does not fail the run by default, which is deliberate and is also the single
   most common source of "it says it worked".

## After the change

A built-in or plugin node needs a rebuild:

```bash
docker compose build && docker compose up -d
```

A custom node does not; it takes effect as soon as it is saved.

## Related

- [Workflows](../../workflows/README.md): the engine, the message contract in full, every shipped node
- [Workflow endpoints](../../api/endpoints/workflows.md): including the custom-node API
- [Build a plugin](../plugin/README.md)
