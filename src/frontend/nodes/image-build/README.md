# Image Build

Build a Docker image from a workflow-managed build context

## What This Node Does

Creates a tar archive from the selected build context and sends it to the Docker API as an image build request.

## When To Use It

Use this for workflow-driven image pipelines, generated Dockerfile contexts, or one-click rebuild tasks.

## Requirements

Requires at least one compatible environment in McHarbor before this node can run successfully.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: Image build completed successfully.
- Output `error`: Build context resolution or Docker build failed.

## Configuration Guide

- `Environment` (environment-select, required): Pick one of the configured environments.
- `Tag` (text, required): Image tag to build.
- `Dockerfile` (text, optional): Dockerfile path inside the build context. Default: `Dockerfile`.
- `Context Path` (text, optional): Relative or absolute build context path. Default: `.`.
- `Target Stage` (text, optional): Multi-stage build target name.
- `Build Args` (key-value, optional): Docker build arguments passed to the daemon.
- `No Cache` (toggle, optional): Disable Docker layer cache for this build.

## Execution Notes

Relative context paths are resolved from McHarbor's workflow files directory.
The resulting msg includes the tag, selected Dockerfile, build args, cache mode, target stage, and raw build output.

## Backend Binding

This frontend node is bound to the workflow action key `image-build` in `src/backend/modules/workflows/service.go`.
Category: `action`.
