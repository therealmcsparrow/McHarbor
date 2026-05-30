# Container Status Trigger

Start a workflow when a container changes state

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
- `Status Event` (select, optional): Choose one of the built-in options. Default: `any`. Options: Any, Start, Stop, Die, Health Status.

## Execution Notes

The node receives the current `msg`, works with its configuration, and forwards the result on one of its output ports.
Use the Debug tab while running a workflow to inspect the data moving through this node.

## Backend Binding

This frontend node is bound to the workflow action key `container-status-trigger` in `src/backend/modules/workflows/service.go`.
Category: `trigger`.
