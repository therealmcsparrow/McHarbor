# Container Action

Start, stop, restart, remove, rename or wait for a container

## What This Node Does

One node that replaces 4 former nodes. The `action` field selects the behaviour; only the fields that apply to the chosen value are shown.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: Primary success path.
- Output `error`: Failure path used when the node returns an error.

## Configuration Guide

- `Environment` (environment-select, required)
- `Container` (container-select, required)
- `Action` (select, required)
- `Timeout (seconds)` (number) — shown when `{"action": ["stop", "restart", "wait"]}`
- `New Name` (text) — shown when `{"action": "rename"}`
- `Unlock Protected Container` (toggle) — shown when `{"action": "rename"}`
- `Condition` (select) — shown when `{"action": "wait"}`

## Migration From The Old Nodes

| Old node | Rewritten to |
|---|---|
| `docker-action` | `action: (unchanged)` |
| `container-power` | `action: (unchanged)` |
| `container-rename` | `action: rename` |
| `container-wait` | `action: wait` |

Saved workflows that still reference an old key keep running: the backend rewrites the action to `container-action` at execution time. Open the workflow and accept the migration prompt to persist the rewrite.

## Backend Binding

This frontend node is bound to the workflow action key `container-action` in `src/backend/modules/workflows/service.go` (router in `merged_nodes.go`).
Category: `action`.
