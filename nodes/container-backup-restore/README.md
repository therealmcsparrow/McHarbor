# Container Backup Restore

Restore a completed container backup run into the container

## What This Node Does

Replays a completed backup run through the same pipeline the Restore dialog uses. **Restore Items** is a multiselect over the archive entries the selected run offers — `config`, `filesystem`, `mount:/data`, … (the four fixed units are offered until a run is picked); leave it empty to restore everything the archive offers. **Secret Key** is only needed for runs encrypted with a user-held key and never appears in logs or the payload.

The payload is the restore result: `{ runId, restored: [...], restorable: [...] }`. Pair it with **Container Action** (stop before, start after) for a full procedure.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: Primary success path.
- Output `error`: Failure path used when the node returns an error.

## Configuration Guide

- `Environment` (environment-select, required)
- `Container (filter)` (container-select)
- `Backup Run` (backup-run-select, required)
- `Restore Items` (text)
- `Secret Key` (text, secret)

## Backend Binding

`executeContainerBackupRestore` in `storage_backup_nodes.go`, backed by `ContainerBackupRunner.Restore` → `container_backups.Service.Restore`.
