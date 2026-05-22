# Automation and Delivery

This section covers higher-level operational features: blueprints, Git, workflows,
custom nodes, dashboard widgets, schedules, reconciler flows, and the app store.

## Blueprints

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/blueprints/` | Lists bundled and custom blueprints. |
| POST | `/api/blueprints/` | Creates a blueprint. |
| GET | `/api/blueprints/{id}` | Returns one blueprint. |
| PUT | `/api/blueprints/{id}` | Updates a blueprint. |
| DELETE | `/api/blueprints/{id}` | Deletes a blueprint. |
| POST | `/api/blueprints/{id}/deploy` | Deploys a blueprint. |

## Reconciler

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/reconciler/` | Lists reconciler definitions. |
| POST | `/api/reconciler/` | Creates a reconciler definition. |
| GET | `/api/reconciler/{id}` | Returns one reconciler definition. |
| PUT | `/api/reconciler/{id}` | Updates a reconciler definition. |
| DELETE | `/api/reconciler/{id}` | Deletes a reconciler definition. |
| POST | `/api/reconciler/{id}/reconcile` | Triggers reconciliation immediately. |
| GET | `/api/reconciler/{id}/drift` | Returns current drift information. |

## Git Integrations

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/git/` | Lists Git integration records. |
| POST | `/api/git/` | Creates a Git integration. |
| GET | `/api/git/{id}` | Returns one Git integration. |
| PUT | `/api/git/{id}` | Updates a Git integration. |
| DELETE | `/api/git/{id}` | Deletes a Git integration. |
| POST | `/api/git/{id}/sync` | Triggers a sync operation. |
| GET | `/api/git/{id}/deployments` | Lists deployment history for the integration. |

## Schedules

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/schedules/` | Lists schedules. |
| POST | `/api/schedules/` | Creates a schedule. |
| GET | `/api/schedules/{id}` | Returns one schedule. |
| PUT | `/api/schedules/{id}` | Updates a schedule. |
| DELETE | `/api/schedules/{id}` | Deletes a schedule. |
| GET | `/api/schedules/{id}/executions` | Lists execution history for a schedule. |

## Workflow Nodes

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/workflow-nodes/` | Lists workflow node availability information. |
| PUT | `/api/workflow-nodes/{key}` | Updates availability for a store-managed node. |

## Workflows

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/workflows/` | Lists workflows. |
| POST | `/api/workflows/` | Creates a workflow from `{ name, description }`. |
| GET | `/api/workflows/runs` | Lists stored workflow runs. |
| GET | `/api/workflows/link-outputs` | Lists stored link-out messages for link-in nodes. |
| GET | `/api/workflows/{id}` | Returns one workflow, including canvas data and variables. |
| PUT | `/api/workflows/{id}` | Updates workflow metadata, status, canvas JSON, or variables. |
| DELETE | `/api/workflows/{id}` | Deletes a workflow. |
| POST | `/api/workflows/{id}/execute` | Starts workflow execution from a trigger node. |
| GET | `/api/workflows/{id}/live` | Streams live workflow execution events over SSE. |

Workflow execute body:

```json
{
  "triggerNodeId": "manual-trigger-1"
}
```

## Custom Nodes

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/custom-nodes/` | Lists all custom node definitions. |
| POST | `/api/custom-nodes/` | Creates a custom JavaScript workflow node. |
| POST | `/api/custom-nodes/test` | Runs a sandboxed test execution without saving. |
| GET | `/api/custom-nodes/{key}` | Returns one custom node definition. |
| PUT | `/api/custom-nodes/{key}` | Updates a custom node definition. |
| DELETE | `/api/custom-nodes/{key}` | Deletes a custom node definition. |

## Widgets

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/widgets/definitions` | Lists built-in and installed widget definitions. |
| POST | `/api/widgets/` | Installs a widget package. |
| PUT | `/api/widgets/{key}` | Updates an installed widget definition. |
| DELETE | `/api/widgets/{key}` | Uninstalls a widget. |

## App Store

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/app-store/` | Lists catalog entries. |
| GET | `/api/app-store/categories` | Lists catalog categories. |
| GET | `/api/app-store/installed` | Lists installed catalog apps. |
| GET | `/api/app-store/sync/status` | Returns current sync state. |
| POST | `/api/app-store/install` | Runs a standard install flow. |
| POST | `/api/app-store/install/stream` | Streams install progress over SSE. |
| POST | `/api/app-store/sync` | Triggers a catalog sync. |
| GET | `/api/app-store/{slug}` | Returns one catalog item. |
