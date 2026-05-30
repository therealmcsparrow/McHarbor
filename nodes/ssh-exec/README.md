# SSH Exec

Run a remote command over SSH with password or key authentication

## What This Node Does

Opens an SSH session, executes the configured command, and returns the combined stdout and stderr output on the workflow msg.

## When To Use It

Use this for remote maintenance steps, deployment hooks, or environment checks that must happen outside the local Docker host.

## Requirements

No extra McHarbor-managed service setup is required. Configure this node directly inside the workflow.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: Command completed successfully.
- Output `error`: Connection or command execution failed.

## Configuration Guide

- `Host` (text, required): Hostname, IP, or SSH URL. Embedded credentials in the host string are supported.
- `Port` (number, optional): SSH port. Default: `22`.
- `User` (text, optional): SSH username. Overrides any username in the host string.
- `Password` (text, optional): Password authentication. Overrides any password in the host string.
- `Private Key` (textarea, optional): PEM private key used for key-based auth.
- `Command` (textarea, required): Remote command to execute.

## Execution Notes

Provide either a password or a private key.
The outgoing msg includes host, port, user, command, and command output.

## Backend Binding

This frontend node is bound to the workflow action key `ssh-exec` in `src/backend/modules/workflows/service.go`.
Category: `integration`.
