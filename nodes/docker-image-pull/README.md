# Docker Image Pull

Pull an image from a registry

## What This Node Does

Receives the current msg, performs work, and usually passes an updated msg to the next step.

## When To Use It

Use action nodes for state changes, API calls, or data transformation.

## Requirements

Requires at least one compatible environment in McHarbor before this node can run successfully.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: Primary success path.
- Output `error`: Failure path used when the node returns an error.

## Configuration Guide

- `Environment` (environment-select, required): Pick one of the configured environments.
- `Registry` (select, required): Choose a configured registry or custom registry auth. Default: `custom`. Options: Configured Registry, Other.
- `Registry` (registry-select, optional): Select a configured registry, or leave it empty to use the default registry.
- `Registry URL` (text, optional): Custom registry server address.
- `Registry Username` (text, optional): Custom registry username.
- `Registry Password` (text, optional): Custom registry password.
- `Image` (text, required): Single-line text value.

## Execution Notes

The node receives the current `msg`, works with its configuration, and forwards the result on one of its output ports.
Use the Debug tab while running a workflow to inspect the data moving through this node.

## Backend Binding

This frontend node is bound to the workflow action key `docker-image-pull` in `src/backend/modules/workflows/service.go`.
Category: `action`.
