# DNS Lookup

Resolve DNS records for a hostname

## What This Node Does

Calls an external system or a configured transport and then continues with the result.

## When To Use It

Use the error output if you want failures to branch into retries, alerts, or recovery logic.

## Requirements

No extra McHarbor-managed service setup is required. Configure this node directly inside the workflow.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: Primary success path.
- Output `error`: Failure path used when the node returns an error.

## Configuration Guide

- `Hostname` (text, required): Single-line text value.
- `Record Type` (select, optional): Choose one of the built-in options. Default: `A`. Options: A, AAAA, MX, TXT, CNAME, NS.

## Execution Notes

The node receives the current `msg`, works with its configuration, and forwards the result on one of its output ports.
Use the Debug tab while running a workflow to inspect the data moving through this node.

## Backend Binding

This frontend node is bound to the workflow action key `dns-lookup` in `src/backend/modules/workflows/service.go`.
Category: `integration`.
