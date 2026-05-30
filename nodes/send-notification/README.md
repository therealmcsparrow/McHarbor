# Send Internal Notification

Send an in-app notification inside McHarbor

## What This Node Does

Creates an internal McHarbor notification that appears in the in-app notification system.

## When To Use It

Use this when a workflow should notify McHarbor users directly without relying on Slack, Discord, Teams, or other external transports.

## Requirements

No external setup is required.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: Primary success path.
- Output `error`: Failure path used when the node returns an error.

## Configuration Guide

- `Title` (text, optional): Single-line text value.
- `Message` (textarea, required): Multi-line text value for bodies, templates, or larger content.

For Slack, Discord, Teams, Gotify, ntfy, Telegram, Signal, WhatsApp, and email, use the dedicated transport nodes instead of this one.

## Execution Notes

The node receives the current `msg`, works with its configuration, and forwards the result on one of its output ports.
Use the Debug tab while running a workflow to inspect the data moving through this node.

## Backend Binding

This frontend node is bound to the workflow action key `send-notification` in `src/backend/modules/workflows/service.go`.
Category: `integration`.
