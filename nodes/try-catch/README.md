# Try Catch

Guard downstream execution and route failures into a catch branch

## What This Node Does

Starts a guarded section of the workflow.
Messages leaving the `output` port carry an internal catch frame.
If a later node in that guarded path fails, or only emits an unhandled `error` output, execution is rerouted to the `catch` port instead of terminating that message path.

## When To Use It

Use this before risky sequences such as file reads, remote commands, uploads, or deployments when you want a local recovery path inside the workflow.

## Requirements

No extra McHarbor-managed service setup is required. Configure this node directly inside the workflow.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: Primary guarded path.
- Output `catch`: Receives the same msg with an added error object when a guarded downstream step fails.

## Configuration Guide

- `Error Property` (expression, optional): Dot-path where catch details are written on the msg. Default: `error`.

## Execution Notes

The catch branch keeps the failing message data and adds metadata about the failed node.
Nested try/catch sections resolve from the innermost guard to the outermost guard.
If a guarded node already has its own connected `error` output, that direct branch still wins.

## Backend Binding

This frontend node is bound to the workflow action key `try-catch` in `src/backend/modules/workflows/service.go`.
Category: `logic`.
