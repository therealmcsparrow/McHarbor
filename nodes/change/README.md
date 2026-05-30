# Change

Set or delete a value on msg, flow, or global scope

## What This Node Does

Helps inspect, reshape, or annotate the msg while keeping the workflow readable.

## When To Use It

Usually passes the message onward after a small internal operation or side effect.

## Requirements

No extra McHarbor-managed service setup is required. Configure this node directly inside the workflow.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: Primary success path.

## Configuration Guide

- `Action` (select, optional): Choose one of the built-in options. Default: `set`. Options: Set, Delete.
- `Scope` (select, optional): Choose one of the built-in options. Default: `msg`. Options: msg, flow, global.
- `Property` (expression, required): Evaluated against msg, flow, and global values at runtime. Default: `payload`.
- `Value` (json, optional): Provide valid JSON. Objects and arrays stay structured.

## Execution Notes

The node receives the current `msg`, works with its configuration, and forwards the result on one of its output ports.
Use the Debug tab while running a workflow to inspect the data moving through this node.

## Backend Binding

This frontend node is bound to the workflow action key `change` in `src/backend/modules/workflows/service.go`.
Category: `utility`.
