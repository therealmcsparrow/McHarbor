# KV Store

Read, write or delete a value in the workflow key-value store

## What This Node Does

One node that replaces 3 former nodes. The `operation` field selects the behaviour; only the fields that apply to the chosen value are shown.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: Primary success path.

## Configuration Guide

- `Operation` (select, required)
- `Key` (text, required)
- `Output Property` (expression) — shown when `{"operation": "get"}`
- `Default Value` (text) — shown when `{"operation": "get"}`
- `Property` (expression) — shown when `{"operation": "set"}`
- `TTL (seconds)` (number) — shown when `{"operation": "set"}`

## Migration From The Old Nodes

| Old node | Rewritten to |
|---|---|
| `kv-get` | `operation: get` |
| `kv-set` | `operation: set` |
| `kv-delete` | `operation: delete` |

Saved workflows that still reference an old key keep running: the backend rewrites the action to `kv` at execution time. Open the workflow and accept the migration prompt to persist the rewrite.

## Backend Binding

This frontend node is bound to the workflow action key `kv` in `src/backend/modules/workflows/service.go` (router in `merged_nodes.go`).
Category: `utility`.
