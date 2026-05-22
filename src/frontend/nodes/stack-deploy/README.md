# Stack Deploy

Deploy a Docker Compose stack from workflow data, inline YAML, or a saved file

## What This Node Does

Builds a compose source, runs `docker compose up -d`, and forwards the deployment result on the workflow msg.

## When To Use It

Use this to promote generated compose files, deploy saved compose bundles, or react to events that carry compose YAML in the workflow message.

## Requirements

Requires at least one compatible environment in McHarbor before this node can run successfully.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: Deployment completed successfully.
- Output `error`: Deployment failed or no compose source could be resolved.

## Configuration Guide

- `Environment` (environment-select, required): Pick one of the configured environments.
- `Stack Name` (text, required): Project name passed to Docker Compose.
- `Compose Source` (select, optional): Choose `Message`, `Inline YAML`, or `File Path`. Default: `Message`.
- `Compose Content` (textarea, optional): Inline compose YAML used when `Compose Source` is `Inline YAML`.
- `Compose Path` (text, optional): Relative or absolute compose file path used when `Compose Source` is `File Path`.

## Execution Notes

`Message` mode accepts `msg.payload` as raw YAML, `msg.payload.compose`, or `msg.payload.compose_path`.
Agent environments are not supported for this node.

## Backend Binding

This frontend node is bound to the workflow action key `stack-deploy` in `src/backend/modules/workflows/service.go`.
Category: `action`.
