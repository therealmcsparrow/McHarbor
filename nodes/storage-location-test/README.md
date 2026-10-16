# Storage Location Test

Run the connection test for a storage location and optionally read its capacity

## What This Node Does

Runs the same multi-step probe as the "Test connection" button in Settings → Storage (credentials, write, change, delete for local; the plugin's own `TestSteps` for plugin-owned types). The payload carries `overallStatus`, `duration` and the per-step list. With **Include Usage** the adapter's optional quota is added as `usage` (`supported: false` when the plugin does not report capacity).

Routes to `error` when the overall status is `fail`, or when any step is worse than `pass` and **Fail On Warning** is on — handy as a pre-flight check before a backup run.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: Primary success path.
- Output `error`: Failure path used when the node returns an error.

## Configuration Guide

- `Storage Location` (storage-location-select, required)
- `Include Usage` (toggle)
- `Fail On Warning` (toggle)

## Backend Binding

`executeStorageLocationTest` in `storage_backup_nodes.go`, backed by `StorageObjectOperator.Test` / `Usage` → `storage_locations.Service.TestStorageLocation` / `Usage`.
