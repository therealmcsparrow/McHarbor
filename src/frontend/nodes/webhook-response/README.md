# Webhook Response

Prepare an HTTP response for a webhook-triggered flow

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

- `Status Code` (number, optional): Numeric value. Default: `200`.
- `Content Type` (text, optional): Single-line text value. Default: `application/json`.
- `Body` (textarea, optional): Multi-line text value for bodies, templates, or larger content.

## Execution Notes

The node receives the current `msg`, works with its configuration, and forwards the result on one of its output ports.
Use the Debug tab while running a workflow to inspect the data moving through this node.

## Backend Binding

This frontend node is bound to the workflow action key `webhook-response` in `src/backend/modules/workflows/service.go`.
Category: `utility`.
