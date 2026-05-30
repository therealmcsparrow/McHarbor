# Condition

Route a message based on a condition

## What This Node Does

Inspects the current msg and decides which path should continue.

## When To Use It

Use this for branching, filtering, matching, or routing without calling an external service.

## Requirements

No extra McHarbor-managed service setup is required. Configure this node directly inside the workflow.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `true`: Used when the condition evaluates to true.
- Output `false`: Used when the condition does not match the primary branch.

## Configuration Guide

- `Field` (expression, required): Evaluated against msg, flow, and global values at runtime. Default: `payload`.
- `Operator` (select, required): Choose one of the built-in options. Default: `==`. Options: Equals, Not equal, Contains, Matches regex, Is empty, Is not empty.
- `Value` (text, optional): Single-line text value.

## Execution Notes

The node receives the current `msg`, works with its configuration, and forwards the result on one of its output ports.
Use the Debug tab while running a workflow to inspect the data moving through this node.

## Backend Binding

This frontend node is bound to the workflow action key `condition` in `src/backend/modules/workflows/service.go`.
Category: `logic`.
