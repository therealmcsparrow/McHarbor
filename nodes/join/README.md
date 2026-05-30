# Join

Join multiple incoming branches into a single message

## What This Node Does

Inspects the current msg and decides which path should continue.

## When To Use It

Use this for branching, filtering, matching, or routing without calling an external service.

## Requirements

No extra McHarbor-managed service setup is required. Configure this node directly inside the workflow.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: Primary success path.

## Configuration Guide

- `Input Count` (number, optional): Numeric value. Default: `2`.
- `Combine Mode` (select, optional): Choose one of the built-in options. Default: `last`. Options: Last Message, Merge Objects, Array.

## Execution Notes

The node receives the current `msg`, works with its configuration, and forwards the result on one of its output ports.
Use the Debug tab while running a workflow to inspect the data moving through this node.

## Backend Binding

This frontend node is bound to the workflow action key `join` in `src/backend/modules/workflows/service.go`.
Category: `logic`.
