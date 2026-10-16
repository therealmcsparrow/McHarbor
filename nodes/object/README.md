# Object

Pick or omit fields, or map an object to a new shape

## What This Node Does

One node that replaces 3 former nodes. The `operation` field selects the behaviour; only the fields that apply to the chosen value are shown.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: Primary success path.
- Output `error`: Failure path used when the node returns an error.

## Configuration Guide

- `Operation` (select, required)
- `Property` (expression)
- `Fields` (text) — shown when `{"operation": ["pick", "omit"]}`
- `Mapping` (key-value) — shown when `{"operation": "map"}`

## Migration From The Old Nodes

| Old node | Rewritten to |
|---|---|
| `pick` | `operation: pick` |
| `omit` | `operation: omit` |
| `map` | `operation: map` |

Saved workflows that still reference an old key keep running: the backend rewrites the action to `object` at execution time. Open the workflow and accept the migration prompt to persist the rewrite.

## Backend Binding

This frontend node is bound to the workflow action key `object` in `src/backend/modules/workflows/service.go` (router in `merged_nodes.go`).
Category: `utility`.
