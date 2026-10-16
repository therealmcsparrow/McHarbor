# Container Trigger

Start a workflow when a container changes state or health

## What This Node Does

One node that replaces 2 former nodes. The `watch` field selects the behaviour; only the fields that apply to the chosen value are shown.

## Port Behavior

- No input ports: this is a trigger.
- Output `output`: Primary success path.

## Configuration Guide

- `Environment` (environment-select, required)
- `Container` (container-select, required)
- `Watch` (select, required)
- `Status Event` (select) — shown when `{"watch": "status"}`
- `Health Status` (select) — shown when `{"watch": "health"}`

## Migration From The Old Nodes

| Old node | Rewritten to |
|---|---|
| `container-status-trigger` | `watch: status` |
| `container-health-trigger` | `watch: health` |

Saved workflows that still reference an old key keep running: the backend rewrites the action to `container-trigger` at execution time. Open the workflow and accept the migration prompt to persist the rewrite.

## Backend Binding

This frontend node is bound to the workflow action key `container-trigger` in `src/backend/modules/workflows/service.go` (router in `merged_nodes.go`).
Category: `trigger`.
