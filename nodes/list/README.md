# List

List containers, images, networks, volumes or stacks in an environment

## What This Node Does

One node that replaces 5 former nodes. The `resource` field selects the behaviour; only the fields that apply to the chosen value are shown.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: Primary success path.
- Output `error`: Failure path used when the node returns an error.

## Configuration Guide

- `Environment` (environment-select, required)
- `Resource` (select, required)
- `Show All` (toggle) — shown when `{"resource": ["container", "image"]}`
- `Name Filter` (text) — shown when `{"resource": "container"}`
- `Only Dangling` (toggle) — shown when `{"resource": ["image", "volume"]}`

## Migration From The Old Nodes

| Old node | Rewritten to |
|---|---|
| `container-list` | `resource: container` |
| `image-list` | `resource: image` |
| `network-list` | `resource: network` |
| `volume-list` | `resource: volume` |
| `stack-list` | `resource: stack` |

Saved workflows that still reference an old key keep running: the backend rewrites the action to `list` at execution time. Open the workflow and accept the migration prompt to persist the rewrite.

## Backend Binding

This frontend node is bound to the workflow action key `list` in `src/backend/modules/workflows/service.go` (router in `merged_nodes.go`).
Category: `action`.
