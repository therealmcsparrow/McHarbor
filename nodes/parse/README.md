# Parse

Parse JSON, YAML, XML, CSV or HTML text into structured data

## What This Node Does

One node that replaces 5 former nodes. The `format` field selects the behaviour; only the fields that apply to the chosen value are shown.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: Primary success path.
- Output `error`: Failure path used when the node returns an error.

## Configuration Guide

- `Format` (select, required)
- `Property` (expression)
- `Delimiter` (text) — shown when `{"format": "csv"}`
- `Has Headers` (toggle) — shown when `{"format": "csv"}`
- `Selector` (text) — shown when `{"format": "html"}`
- `Output Type` (select) — shown when `{"format": "html"}`
- `Attribute` (text) — shown when `{"format": "html", "output_type": "attribute"}`

## Migration From The Old Nodes

| Old node | Rewritten to |
|---|---|
| `parse-json` | `format: json` |
| `parse-yaml` | `format: yaml` |
| `parse-xml` | `format: xml` |
| `parse-csv` | `format: csv` |
| `parse-html` | `format: html` |

Saved workflows that still reference an old key keep running: the backend rewrites the action to `parse` at execution time. Open the workflow and accept the migration prompt to persist the rewrite.

## Backend Binding

This frontend node is bound to the workflow action key `parse` in `src/backend/modules/workflows/service.go` (router in `merged_nodes.go`).
Category: `utility`.
