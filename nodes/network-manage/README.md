# Network Manage

Create or remove a network, or connect and disconnect containers

## What This Node Does

One node that replaces 3 former nodes. The `action` field selects the behaviour; only the fields that apply to the chosen value are shown.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: Primary success path.
- Output `error`: Failure path used when the node returns an error.

## Configuration Guide

- `Environment` (environment-select, required)
- `Action` (select, required)
- `Name` (text) — shown when `{"action": "create"}`
- `Driver` (text) — shown when `{"action": "create"}`
- `Internal` (toggle) — shown when `{"action": "create"}`
- `Subnet` (text) — shown when `{"action": "create"}`
- `Network` (network-select) — shown when `{"action": ["remove", "connect", "disconnect"]}`
- `Container` (container-select) — shown when `{"action": ["connect", "disconnect"]}`

## Migration From The Old Nodes

| Old node | Rewritten to |
|---|---|
| `network-create` | `action: create` |
| `network-remove` | `action: remove` |
| `network-connect` | `action: connect` |

Saved workflows that still reference an old key keep running: the backend rewrites the action to `network-manage` at execution time. Open the workflow and accept the migration prompt to persist the rewrite.

## Backend Binding

This frontend node is bound to the workflow action key `network-manage` in `src/backend/modules/workflows/service.go` (router in `merged_nodes.go`).
Category: `action`.
