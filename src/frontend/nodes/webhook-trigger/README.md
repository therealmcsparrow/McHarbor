# Webhook Trigger

Start a workflow from an incoming webhook request

## What This Node Does

Starts a workflow run and creates the first msg object in the chain.

## When To Use It

Use this node at the start of a workflow. It does not wait for upstream input.

## Requirements

No extra McHarbor-managed service setup is required. Configure this node directly inside the workflow.

## Port Behavior

- Inputs: none. This node starts a flow and does not accept upstream connections.
- Output `output`: Primary success path.

## Configuration Guide

- `Method` (select, optional): Choose one of the built-in options. Default: `POST`. Options: GET, POST, PUT, PATCH, DELETE.
- `Path` (text, optional): Single-line text value.
- `Secret` (text, optional): Single-line text value.

## Execution Notes

The node receives the current `msg`, works with its configuration, and forwards the result on one of its output ports.
Use the Debug tab while running a workflow to inspect the data moving through this node.

## Backend Binding

This frontend node is bound to the workflow action key `webhook-trigger` in `src/backend/modules/workflows/service.go`.
Category: `trigger`.
