# Image Manage

Pull, push, tag or remove a Docker image

## What This Node Does

One node that replaces 4 former nodes. The `action` field selects the behaviour; only the fields that apply to the chosen value are shown.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: Primary success path.
- Output `error`: Failure path used when the node returns an error.

## Configuration Guide

- `Environment` (environment-select, required)
- `Action` (select, required)
- `Registry` (select) — shown when `{"action": ["pull", "push"]}`
- `Registry` (registry-select) — shown when `{"action": ["pull", "push"], "registry_mode": "configured"}`
- `Registry URL` (text) — shown when `{"action": ["pull", "push"], "registry_mode": "custom"}`
- `Registry Username` (text) — shown when `{"action": ["pull", "push"], "registry_mode": "custom"}`
- `Registry Password` (text) — shown when `{"action": ["pull", "push"], "registry_mode": "custom"}`
- `Image` (text) — shown when `{"action": "pull"}`
- `Image` (image-select, custom value allowed) — shown when `{"action": ["push", "remove"]}`
- `Source Image` (image-select) — shown when `{"action": "tag"}`
- `Target Image` (text) — shown when `{"action": "tag"}`
- `Force` (toggle) — shown when `{"action": "remove"}`
- `Prune Children` (toggle) — shown when `{"action": "remove"}`

## Migration From The Old Nodes

| Old node | Rewritten to |
|---|---|
| `docker-image-pull` | `action: pull` |
| `image-push` | `action: push` |
| `image-tag` | `action: tag` |
| `image-remove` | `action: remove` |

Saved workflows that still reference an old key keep running: the backend rewrites the action to `image-manage` at execution time. Open the workflow and accept the migration prompt to persist the rewrite.

## Backend Binding

This frontend node is bound to the workflow action key `image-manage` in `src/backend/modules/workflows/service.go` (router in `merged_nodes.go`).
Category: `action`.
