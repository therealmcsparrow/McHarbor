# Volume Manage

Create, remove, back up or restore a Docker volume

## What This Node Does

One node that replaces 4 former nodes. The `action` field selects the behaviour; only the fields that apply to the chosen value are shown.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: Primary success path.
- Output `error`: Failure path used when the node returns an error.

## Configuration Guide

- `Environment` (environment-select, required)
- `Action` (select, required)
- `Name` (text) — shown when `{"action": "create"}`
- `Driver` (text) — shown when `{"action": "create"}`
- `Labels` (key-value) — shown when `{"action": "create"}`
- `Force` (toggle) — shown when `{"action": "remove"}`
- `Volume` (volume-select) — shown when `{"action": ["remove", "backup", "restore"]}`
- `Backup File Path` (text) — shown when `{"action": ["backup", "restore"]}`
- `Overwrite Existing Files` (toggle) — shown when `{"action": "restore"}`
- `Helper Image` (text) — shown when `{"action": ["backup", "restore"]}`

## Migration From The Old Nodes

| Old node | Rewritten to |
|---|---|
| `volume-create` | `action: create` |
| `volume-remove` | `action: remove` |
| `volume-backup` | `action: backup` |
| `volume-restore` | `action: restore` |

Saved workflows that still reference an old key keep running: the backend rewrites the action to `volume-manage` at execution time. Open the workflow and accept the migration prompt to persist the rewrite.

## Backend Binding

This frontend node is bound to the workflow action key `volume-manage` in `src/backend/modules/workflows/service.go` (router in `merged_nodes.go`).
Category: `action`.
