# Encode

Base64 encode or decode, or hash a value

## What This Node Does

One node that replaces 2 former nodes. The `algorithm` field selects the behaviour; only the fields that apply to the chosen value are shown.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: Primary success path.
- Output `error`: Failure path used when the node returns an error.

## Configuration Guide

- `Algorithm` (select, required)
- `Property` (expression)
- `Output Property` (expression) — shown when `{"algorithm": ["md5", "sha1", "sha256", "sha512"]}`

## Migration From The Old Nodes

| Old node | Rewritten to |
|---|---|
| `base64` | `algorithm: (unchanged)` |
| `hash` | `algorithm: sha256` |

Saved workflows that still reference an old key keep running: the backend rewrites the action to `encode` at execution time. Open the workflow and accept the migration prompt to persist the rewrite.

## Backend Binding

This frontend node is bound to the workflow action key `encode` in `src/backend/modules/workflows/service.go` (router in `merged_nodes.go`).
Category: `utility`.
