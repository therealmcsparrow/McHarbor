# Health Check

Check an HTTP endpoint and branch on health

## What This Node Does

Receives the current msg, performs work, and usually passes an updated msg to the next step.

## When To Use It

Use action nodes for state changes, API calls, or data transformation.

## Requirements

No extra McHarbor-managed service setup is required. Configure this node directly inside the workflow.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `healthy`: Sends the outgoing msg on this port.
- Output `unhealthy`: Sends the outgoing msg on this port.

## Configuration Guide

- `URL` (text, required): Single-line text value.
- `Method` (select, optional): Choose one of the built-in options. Default: `GET`. Options: GET, POST, PUT, PATCH, DELETE.
- `Expected Status` (number, optional): Numeric value. Default: `200`.
- `Timeout (seconds)` (number, optional): Numeric value. Default: `10`.
- `Retries` (number, optional): Numeric value. Default: `3`.

## Execution Notes

The node receives the current `msg`, works with its configuration, and forwards the result on one of its output ports.
Use the Debug tab while running a workflow to inspect the data moving through this node.

## Backend Binding

This frontend node is bound to the workflow action key `health-check` in `src/backend/modules/workflows/service.go`.
Category: `action`.
