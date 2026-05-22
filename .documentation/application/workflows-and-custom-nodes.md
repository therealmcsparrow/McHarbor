# Workflows and Custom Nodes

McHarbor includes a visual workflow engine for automating operational tasks.
The workflow system is one of the main application subsystems beyond raw resource
management.

## Workflow Model

Workflows are stored in the backend and represented by:

- metadata such as name, description, and status
- serialized canvas data
- serialized variable state
- workflow runs history

The backend service lives in:

- `src/backend/modules/workflows/service.go`

## Execution Model

The workflow engine executes node-based flows by traversing canvas nodes and edges.
It supports:

- trigger nodes
- condition and control-flow nodes
- Docker actions
- notification actions
- HTTP-style actions
- variable and transform nodes
- monitoring nodes
- external integration stubs
- custom-node execution through a bridge

## Message Shape

Workflow nodes operate on a `msg` object pattern similar to flow-based automation tools.
Typical fields include:

- `_msgid`
- `topic`
- `payload`
- `req`
- `headers`
- `statusCode`
- `parts`
- `_performance`

## Trigger Sources

Examples of trigger types include:

- manual trigger
- webhook trigger
- schedule trigger
- container status trigger
- metric trigger

## Workflow API Surface

Key workflow API routes:

- `GET /api/workflows/`
- `POST /api/workflows/`
- `GET /api/workflows/runs`
- `GET /api/workflows/{id}`
- `PUT /api/workflows/{id}`
- `DELETE /api/workflows/{id}`
- `POST /api/workflows/{id}/execute`
- `GET /api/workflows/{id}/live`

## Link-In / Link-Out Behavior

The workflow service supports link-style message passing:

- link-out nodes store messages
- downstream workflows can be auto-triggered from those stored outputs
- a callback is wired during backend startup to connect these flows

## Custom Nodes

Custom nodes are implemented with sandboxed JavaScript executed in goja.

Executor file:

- `src/backend/modules/custom_nodes/executor.go`

Capabilities:

- fresh VM per run
- `msg` and `config` exposed to the script
- `node.log`, `node.warn`, and `node.error` helpers
- `console.log`, `console.warn`, and `console.error`
- timeout enforcement
- capped log capture

Accepted script behaviors:

- return `{ port, msg }`
- return a plain message object
- mutate `msg` directly and return nothing

## Storage Model

Custom node definitions are file-backed and stored under the application data area.
They include:

- definition metadata
- JavaScript execute file
- translation files

## Frontend Workflow Editor

The frontend workflow module provides:

- workflow list and run history pages
- visual editor
- node registry and palette
- live execution event view
- custom node synchronization

## Why It Matters

The workflow engine lets McHarbor move beyond inspection and manual actions into
repeatable operational automation within the same platform.
