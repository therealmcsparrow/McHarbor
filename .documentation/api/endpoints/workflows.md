# Workflows, nodes, schedules and webhooks

32 endpoints: the visual workflow engine, the custom JavaScript nodes it can run, the cron
schedules that fire actions, and the outbound webhooks that report them.

| Key | Covers |
|---|---|
| `workflows.manage` | Create, edit, import a workflow |
| `workflows.delete` | Delete a workflow |
| `workflows.run` | Trigger a run |
| `store_nodes.view` / `store_nodes.manage` | Read and set node availability |
| `custom_nodes.view` / `custom_nodes.manage` | Custom JavaScript nodes |
| `schedules.view` / `.manage` / `.delete` | Cron schedules |
| `webhooks.view` / `.manage` / `.delete` | Outbound webhooks |

## Things worth knowing first

**Reading a workflow takes no permission.** List, get, export, the run list, the link-output
list and the live event stream are authenticated but ungated. `workflows.view` exists in the
catalog and gates nothing. Any signed-in user can read every workflow, including whatever is
in its node configuration. Writing, deleting and running are all gated.

**Execution streams.** `POST /api/workflows/{id}/execute` answers with SSE and emits an event
per step as the run walks the graph. `GET /api/workflows/{id}/live` is the passive version,
for a canvas watching a run someone else started.

**Incoming webhooks are matched, not routed.** `/api/workflows/webhooks/*` is one catch-all
in the rate-limited auth group; the handler matches the path and method against the workflow
triggers that are armed, and compares any configured secret in constant time.

**Export and import are portable.** `GET /api/workflows/{id}/export` produces a versioned
JSON document; `POST /api/workflows/import` creates a **draft** from one. Import never
overwrites an existing workflow.

**Custom nodes run in a sandbox.** `POST /api/custom-nodes/test` executes a script in a
sandboxed VM against a supplied `msg` and `config` without saving anything, which is how the
editor's Run button works.

**Node availability is an override, not a catalog.** `PUT /api/workflow-nodes/{key}` records
whether a node type is offered in the palette; the catalog itself comes from the built-in
node folders plus whatever plugins contribute.

**Schedules and webhooks here are estate-wide.** A stack's own webhooks are separate, under
`/api/stacks/{name}/webhooks`; see [Stacks](./stacks.md).

## Endpoints

| Method | Path | Permission | What it does | Parameters |
|---|---|---|---|---|
| `GET` | `/api/custom-nodes` | `custom_nodes.view` | Returns all custom node definitions with code and translations | — |
| `POST` | `/api/custom-nodes` | `custom_nodes.manage` | Creates a new custom node | — |
| `POST` | `/api/custom-nodes/test` | `custom_nodes.manage` | Runs a script in a sandboxed VM for testing without saving | — |
| `DELETE` | `/api/custom-nodes/{key}` | `custom_nodes.manage` | Deletes a custom node | `{key}` |
| `GET` | `/api/custom-nodes/{key}` | `custom_nodes.view` | Returns a single custom node definition with code | `{key}` |
| `PUT` | `/api/custom-nodes/{key}` | `custom_nodes.manage` | Updates an existing custom node | `{key}` |
| `GET` | `/api/schedules` | `schedules.view` | Returns a paginated list of schedules | — |
| `POST` | `/api/schedules` | `schedules.manage` | Creates a new schedule | — |
| `DELETE` | `/api/schedules/{id}` | `schedules.delete` | Removes a schedule | `{id}` |
| `GET` | `/api/schedules/{id}` | `schedules.view` | Returns a single schedule | `{id}` |
| `PUT` | `/api/schedules/{id}` | `schedules.manage` | Updates an existing schedule | `{id}` |
| `GET` | `/api/schedules/{id}/executions` | `schedules.view` | Returns execution history for a schedule | `{id}` |
| `GET` | `/api/webhooks` | `webhooks.view` | Returns a paginated list of webhooks | — |
| `POST` | `/api/webhooks` | `webhooks.manage` | Creates a new webhook | — |
| `DELETE` | `/api/webhooks/{id}` | `webhooks.delete` | Removes a webhook | `{id}` |
| `GET` | `/api/webhooks/{id}` | `webhooks.view` | Returns a single webhook | `{id}` |
| `PUT` | `/api/webhooks/{id}` | `webhooks.manage` | Updates an existing webhook | `{id}` |
| `GET` | `/api/webhooks/{id}/deliveries` | `webhooks.view` | Returns delivery history for a webhook | `{id}` |
| `POST` | `/api/webhooks/{id}/test` | `webhooks.manage` | Sends a test delivery to the webhook URL | `{id}` |
| `GET` | `/api/workflow-nodes` | `store_nodes.view` | Returns workflow node availability overrides | — |
| `PUT` | `/api/workflow-nodes/{key}` | `store_nodes.manage` | Stores workflow node availability overrides | `{key}` |
| `GET` | `/api/workflows` | — | Returns all workflows | `?status=` |
| `POST` | `/api/workflows` | `workflows.manage` | Creates a new workflow | — |
| `POST` | `/api/workflows/import` | `workflows.manage` | Creates a draft workflow from a portable workflow export file | — |
| `GET` | `/api/workflows/link-outputs` | — | Returns all link-out nodes across workflows | — |
| `GET` | `/api/workflows/runs` | — | Returns paginated runs for a workflow (or all workflows) | `?workflow_id=` |
| `DELETE` | `/api/workflows/{id}` | `workflows.delete` | Removes a workflow | `{id}` |
| `GET` | `/api/workflows/{id}` | — | Returns a single workflow by ID | `{id}` |
| `PUT` | `/api/workflows/{id}` | `workflows.manage` | Updates an existing workflow | `{id}` |
| `POST` | `/api/workflows/{id}/execute` | `workflows.run` | Runs a workflow and streams execution events via SSE | `{id}` |
| `GET` | `/api/workflows/{id}/export` | — | Downloads a single workflow as a portable JSON file | `{id}` |
| `GET` | `/api/workflows/{id}/live` | — | Streams background execution events for a workflow via SSE | `{id}` |


## Request bodies

**`POST` `/api/custom-nodes`**: `definition`, `code`, `translations`

**`POST` `/api/custom-nodes/test`**: `code`, `config`, `msg`

**`PUT` `/api/custom-nodes/{key}`**: `definition`, `code`, `translations`

**`POST` `/api/schedules`**: `name`, `description`, `cron`, `action`, `target`, `envId`

**`PUT` `/api/schedules/{id}`**: `name`?, `description`?, `cron`?, `action`?, `target`?, `envId`?, `enabled`?

**`POST` `/api/webhooks`**: `name`, `url`, `secret`, `events`

**`PUT` `/api/webhooks/{id}`**: `name`?, `url`?, `secret`?, `events`?, `isActive`?

**`PUT` `/api/workflow-nodes/{key}`**: `enabled`?

**`POST` `/api/workflows`**: `name`, `description`

**`POST` `/api/workflows/import`**: `kind`, `version`, `exportedAt`, `workflow`

**`PUT` `/api/workflows/{id}`**: `name`?, `description`?, `status`?, `canvasData`?, `variables`?

**`POST` `/api/workflows/{id}/execute`**: `triggerNodeId`

## Related

- [Workflows](../../workflows/README.md): the engine, the message contract and every node
- [Streaming](../conventions/streaming.md): the execution stream
