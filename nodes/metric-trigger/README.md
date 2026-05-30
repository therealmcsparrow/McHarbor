# Metric Trigger

Start a workflow when container metrics are checked

## What This Node Does

Starts a workflow run and creates the first msg object in the chain.

## When To Use It

Use this node at the start of a workflow. It does not wait for upstream input.

## Requirements

Requires at least one compatible environment in McHarbor before this node can run successfully.

## Port Behavior

- Inputs: none. This node starts a flow and does not accept upstream connections.
- Output `output`: Primary success path.

## Configuration Guide

- `Environment` (environment-select, required): Pick one of the configured environments.
- `Container` (container-select, required): Pick a container from the active environment.
- `Metric Conditions` (metric-conditions, optional): Define one or more metric threshold rules.
- `Condition Logic` (select, optional): Choose one of the built-in options. Default: `and`. Options: AND, OR.
- `Interval (seconds)` (number, optional): Numeric value. Default: `10`.
- `Cooldown (seconds)` (number, optional): Numeric value. Default: `60`.

## Execution Notes

The node receives the current `msg`, works with its configuration, and forwards the result on one of its output ports.
Use the Debug tab while running a workflow to inspect the data moving through this node.

## Backend Binding

This frontend node is bound to the workflow action key `metric-trigger` in `src/backend/modules/workflows/service.go`.
Category: `trigger`.
