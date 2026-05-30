# Send Email

Send an email through a configured email server

## What This Node Does

Calls an external system or a configured transport and then continues with the result.

## When To Use It

Use the error output if you want failures to branch into retries, alerts, or recovery logic.

## Requirements

Use a configured email server from Settings, or switch Delivery to Other and provide SMTP settings directly.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: Primary success path.
- Output `error`: Failure path used when the node returns an error.

## Configuration Guide

- `Delivery` (select, required): Choose a configured email server or custom SMTP settings. Default: `configured`. Options: Configured Email Server, Other.
- `Email Server` (email-server-select, optional): Select a configured email server, or leave it empty to use the default enabled server.
- `SMTP Host` (text, required): Custom SMTP host when Delivery is Other.
- `SMTP Port` (number, required): Custom SMTP port when Delivery is Other. Default: `587`.
- `Encryption` (select, optional): SMTP encryption mode. Default: `starttls`. Options: None, STARTTLS, SSL/TLS.
- `Auth Method` (select, optional): SMTP authentication method. Default: `plain`. Options: None, Plain, Login, CRAM-MD5.
- `Username` (text, optional): Custom SMTP username.
- `Password` (text, optional): Custom SMTP password.
- `From Address` (text, required): Sender address for custom SMTP.
- `From Name` (text, optional): Sender display name for custom SMTP.
- `To` (text, required): Single-line text value.
- `Subject` (text, optional): Single-line text value.
- `Body` (textarea, required): Multi-line text value for bodies, templates, or larger content.

## Execution Notes

The node receives the current `msg`, works with its configuration, and forwards the result on one of its output ports.
Use the Debug tab while running a workflow to inspect the data moving through this node.

## Backend Binding

This frontend node is bound to the workflow action key `send-email` in `src/backend/modules/workflows/service.go`.
Category: `integration`.
