# Stack Manage

Deploy, start, stop or remove a compose stack

## What This Node Does

One node that replaces 4 former nodes. The `action` field selects the behaviour; only the fields that apply to the chosen value are shown.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: Primary success path.
- Output `error`: Failure path used when the node returns an error.

## Configuration Guide

- `Environment` (environment-select, required)
- `Action` (select, required)
- `Stack Name` (stack-select, custom value allowed for new deployments, required)
- `Compose Source` (select) — shown when `{"action": "deploy"}`
- `Compose Content` (textarea) — shown when `{"action": "deploy", "compose_source": "inline"}`
- `Compose Path` (text) — shown when `{"action": "deploy", "compose_source": "file"}`
- `Remove Volumes` (toggle) — shown when `{"action": ["down", "remove"]}`

## Migration From The Old Nodes

| Old node | Rewritten to |
|---|---|
| `stack-deploy` | `action: deploy` |
| `compose-up` | `action: up` |
| `compose-down` | `action: down` |
| `stack-remove` | `action: remove` |

Saved workflows that still reference an old key keep running: the backend rewrites the action to `stack-manage` at execution time. Open the workflow and accept the migration prompt to persist the rewrite.

## Backend Binding

This frontend node is bound to the workflow action key `stack-manage` in `src/backend/modules/workflows/service.go` (router in `merged_nodes.go`).
Category: `action`.
