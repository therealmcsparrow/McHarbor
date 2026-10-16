# Workflow Reference

Routes, the data model, and the event stream.

## API routes

### Workflows

| Route | Permission |
|---|---|
| `GET /api/workflows` | _session only_ |
| `POST /api/workflows` | `workflows.manage` |
| `POST /api/workflows/import` | `workflows.manage` |
| `GET /api/workflows/runs` | _session only_ |
| `GET /api/workflows/link-outputs` | _session only_ |
| `GET /api/workflows/{id}` | _session only_ |
| `PUT /api/workflows/{id}` | `workflows.manage` |
| `DELETE /api/workflows/{id}` | `workflows.delete` |
| `GET /api/workflows/{id}/export` | _session only_ |
| `POST /api/workflows/{id}/execute` | `workflows.run` |
| `GET /api/workflows/{id}/live` | _session only_ (SSE) |

### Node availability

| Route | Permission |
|---|---|
| `GET /api/workflow-nodes` | `store_nodes.view` |
| `PUT /api/workflow-nodes/{key}` | `store_nodes.manage` |

### Custom nodes

| Route | Permission |
|---|---|
| `GET /api/custom-nodes` | `custom_nodes.view` |
| `POST /api/custom-nodes` | `custom_nodes.manage` |
| `POST /api/custom-nodes/test` | `custom_nodes.manage` |
| `GET /api/custom-nodes/{key}` | `custom_nodes.view` |
| `PUT /api/custom-nodes/{key}` | `custom_nodes.manage` |
| `DELETE /api/custom-nodes/{key}` | `custom_nodes.manage` |

### Incoming webhooks

| Route | Permission |
|---|---|
| `ANY /api/workflows/webhooks/*` | **None**: no session. Matched by path and method against active webhook triggers, and guarded by the trigger's own secret when one is set |

### A gap worth knowing about

`workflows.view` exists in the permission catalog, is described as "List and inspect
workflows", gates the sidebar entry, and is required by an AI assistant tool, but **it is not
enforced on the read routes above**. Listing workflows, reading one, reading its runs,
exporting it and subscribing to its live events are all session-protected only.

The mutating routes *are* gated. This is a real gap rather than a design choice, and it is
worth knowing before assuming a viewer cannot read a workflow's contents.

## The data model

### Workflow

| Field | |
|---|---|
| `id`, `name`, `description` | Identity |
| `status` | Active or inactive; arms automatic triggers |
| `canvasData` | The canvas as JSON |
| `variables` | A JSON object seeding the `flow` scope |
| `createdBy`, `updatedBy`, `lastRunAt`, `createdAt`, `updatedAt` | Bookkeeping |

### Canvas

```jsonc
{
  "nodes": [
    {
      "id": "node-1",
      "type": "action",
      "action": "container-action",     // the node key — what the runtime dispatches on
      "label": "Restart api",
      "config": { "environment": "…", "container": "api", "action": "restart" },
      "debug": false,
      "blockedPorts": ["out:error"],
      "skip": false,
      "disabled": false
    }
  ],
  "edges": [
    {
      "id": "edge-1",
      "sourceNodeId": "node-1", "sourcePort": "output",
      "targetNodeId": "node-2", "targetPort": "input",
      "sniffer": { "name": "after restart" }
    }
  ],
  "groups": [
    { "id": "g1", "name": "Nightly", "color": "#…", "nodeIds": ["node-1"], "blocked": false }
  ]
}
```

`blockedPorts` entries are prefixed `out:` or `in:`, and are enforced in both directions.

### Run

| Field | |
|---|---|
| `id`, `workflowId` | Identity |
| `status` | `completed`, `failed` or `cancelled` |
| `trigger` | What started it |
| `durationMs`, `nodeCount` | Cost |
| `error` | The first error recorded, if any |
| `startedAt`, `finishedAt` | Timing |

### Export

A workflow exports as `{ kind, version, exportedAt, workflow }`, where the workflow record
carries the original id, name, description, status, canvas, variables and the original
timestamps. Import takes the same shape.

## Event stream

`GET /api/workflows/{id}/live` is SSE. Each event has a name and a JSON payload:

| Event | Payload |
|---|---|
| `run.started`, `run.completed`, `run.cancelled` | `status`, `durationMs`, `error` |
| `node.started` | `nodeId`, `label`, `action` |
| `node.completed` | `nodeId`, `label`, `action`, `outputPort`, `input`, `config`, `output`, `durationMs` |
| `node.failed` | `nodeId`, `label`, `action`, `error` |
| `edge.traversed` | `edgeId`, `sourceNodeId`, `targetNodeId` |
| `debug` | `nodeId`, `label`, `level`, `message`, `data`, `source` |

`source` on a debug event distinguishes `debug-node` from `sniffer`.

## Runtime limits

| Limit | Value |
|---|---|
| Node executions per run | 2000 |
| Default run timeout | 5 minutes |
| Custom node script timeout | 10s default, 60s maximum |
| Custom node log lines | 100 per execution |
| Offline/link buffers | Per-run, not persisted between runs |

## Storage

| Thing | Where |
|---|---|
| Workflows, runs, link messages | The McHarbor database |
| Node availability overrides | `$DATA_DIR/workflow-nodes/state.json`; absent key means enabled |
| Custom node code | Stored by the host, loaded per execution |
| Built-in node definitions | The `nodes/` catalog in the repository |

## Related

- [Execution model](../execution/README.md): what the limits above mean in practice
- [The message contract](../messages/README.md): the shape being passed around
- [Developer](../../development/workflows/README.md): adding nodes
