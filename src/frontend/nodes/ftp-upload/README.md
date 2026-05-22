# FTP Upload

Upload workflow data or a saved file with FTP or SFTP

## What This Node Does

Uploads either a selected workflow property or a saved file path to an FTP or SFTP destination by shelling out to `curl`.

## When To Use It

Use this to publish generated exports, send backups, or push workflow payloads into external file drops.

## Requirements

No extra McHarbor-managed service setup is required. Configure this node directly inside the workflow.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: Upload completed successfully.
- Output `error`: Source resolution or upload failed.

## Configuration Guide

- `Host` (text, required): Destination host or full FTP/SFTP URL.
- `Port` (number, optional): Overrides the port in the URL or host string.
- `Username` (text, optional): Destination username.
- `Password` (text, optional): Destination password.
- `Protocol` (select, optional): `FTP` or `SFTP`. Default: `FTP`.
- `Remote Path` (text, required): Destination path on the remote server.
- `Source Mode` (select, optional): Upload a workflow property or a file path. Default: `Message Property`.
- `Property` (expression, optional): Msg path used when `Source Mode` is `Message Property`. Default: `payload`.
- `Local Path` (text, optional): Relative or absolute file path used when `Source Mode` is `File Path`.

## Execution Notes

When uploading from a workflow property, the node writes a temporary file from the selected value before sending it.
Relative local paths are resolved from McHarbor's workflow files directory.

## Backend Binding

This frontend node is bound to the workflow action key `ftp-upload` in `src/backend/modules/workflows/service.go`.
Category: `integration`.
