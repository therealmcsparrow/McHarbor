# Inspect

Inspect a container, image, network, volume or stack

## What This Node Does

One node that replaces 5 former nodes. The `resource` field selects the behaviour; only the fields that apply to the chosen value are shown.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: Primary success path.
- Output `error`: Failure path used when the node returns an error.

## Configuration Guide

- `Environment` (environment-select, required)
- `Resource` (select, required)
- `Container` (container-select) — shown when `{"resource": "container"}`
- `Image` (image-select) — shown when `{"resource": "image"}`
- `Network` (network-select) — shown when `{"resource": "network"}`
- `Volume` (volume-select) — shown when `{"resource": "volume"}`
- `Stack Name` (stack-select) — shown when `{"resource": "stack"}`

## Migration From The Old Nodes

| Old node | Rewritten to |
|---|---|
| `container-inspect` | `resource: container` |
| `image-inspect` | `resource: image` |
| `network-inspect` | `resource: network` |
| `volume-inspect` | `resource: volume` |
| `stack-status` | `resource: stack` |

Saved workflows that still reference an old key keep running: the backend rewrites the action to `inspect` at execution time. Open the workflow and accept the migration prompt to persist the rewrite.

## Backend Binding

This frontend node is bound to the workflow action key `inspect` in `src/backend/modules/workflows/service.go` (router in `merged_nodes.go`).
Category: `action`.
