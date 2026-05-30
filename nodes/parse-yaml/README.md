# Parse YAML

Parse YAML into a nested object

## What This Node Does

Helps inspect, reshape, or annotate the msg while keeping the workflow readable.

## When To Use It

Usually passes the message onward after a small internal operation or side effect.

## Requirements

No extra McHarbor-managed service setup is required. Configure this node directly inside the workflow.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: Primary success path.
- Output `error`: Failure path used when the node returns an error.

## Configuration Guide

- `Property` (expression, optional): Evaluated against msg, flow, and global values at runtime. Default: `payload`.

## Execution Notes

The node receives the current `msg`, works with its configuration, and forwards the result on one of its output ports.
Use the Debug tab while running a workflow to inspect the data moving through this node.

## Backend Binding

This frontend node is bound to the workflow action key `parse-yaml` in `src/backend/modules/workflows/service.go`.
Category: `utility`.
