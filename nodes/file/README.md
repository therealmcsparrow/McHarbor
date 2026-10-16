# File

Read or write a file in the workflow data directory

## What This Node Does

One node that replaces 2 former nodes. The `operation` field selects the behaviour; only the fields that apply to the chosen value are shown.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: Primary success path.
- Output `error`: Failure path used when the node returns an error.

## Configuration Guide

- `Operation` (select, required)
- `Path` (text, required)
- `Encoding` (select)
- `Output Property` (expression) — shown when `{"operation": "read"}`
- `Property` (expression) — shown when `{"operation": "write"}`
- `Mode` (select) — shown when `{"operation": "write"}`

## Migration From The Old Nodes

| Old node | Rewritten to |
|---|---|
| `read-file` | `operation: read` |
| `write-file` | `operation: write` |

Saved workflows that still reference an old key keep running: the backend rewrites the action to `file` at execution time. Open the workflow and accept the migration prompt to persist the rewrite.

## Backend Binding

This frontend node is bound to the workflow action key `file` in `src/backend/modules/workflows/service.go` (router in `merged_nodes.go`).
Category: `utility`.
