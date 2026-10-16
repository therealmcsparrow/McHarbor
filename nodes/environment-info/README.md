# Environment Info

Read Docker daemon, host or reachability information for an environment

## What This Node Does

One node that replaces 3 former nodes. The `scope` field selects the behaviour; only the fields that apply to the chosen value are shown.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: Primary success path.
- Output `error`: Failure path used when the node returns an error.

## Configuration Guide

- `Environment` (environment-select, required)
- `Information` (select, required)

## Migration From The Old Nodes

| Old node | Rewritten to |
|---|---|
| `docker-info` | `scope: docker` |
| `host-info` | `scope: host` |
| `environment-status` | `scope: status` |

Saved workflows that still reference an old key keep running: the backend rewrites the action to `environment-info` at execution time. Open the workflow and accept the migration prompt to persist the rewrite.

## Backend Binding

This frontend node is bound to the workflow action key `environment-info` in `src/backend/modules/workflows/service.go` (router in `merged_nodes.go`).
Category: `action`.
