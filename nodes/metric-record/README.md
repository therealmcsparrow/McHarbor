# Metric Record

Persist a workflow metric sample from the current message

## What This Node Does

Reads a value from the current msg, writes a metric sample to SQLite, and forwards the msg with `_metric` metadata describing the stored record.

## When To Use It

Use this to keep lightweight workflow telemetry such as durations, counts, sizes, or state-change events without calling an external metrics service.

## Requirements

No extra McHarbor-managed service setup is required. Configure this node directly inside the workflow.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: Metric sample stored successfully.
- Output `error`: Used when the source property cannot be read or the metric row cannot be persisted.

## Configuration Guide

- `Metric Name` (text, required): Stable metric identifier stored with the sample.
- `Property` (expression, optional): Msg path to read. Default: `payload`.
- `Metric Type` (select, optional): `gauge`, `counter`, or `event`. Default: `gauge`.
- `Unit` (text, optional): Human-readable unit such as `ms`, `bytes`, or `count`.
- `Labels` (key-value, optional): Extra structured tags stored with the metric row.

## Execution Notes

Samples are written to the `workflow_metrics` table.
The node keeps the original msg and adds `_metric` details such as record id, stored type, labels, and timestamp.

## Backend Binding

This frontend node is bound to the workflow action key `metric-record` in `src/backend/modules/workflows/service.go`.
Category: `utility`.
