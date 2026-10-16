# Container Backup Manage

List backup plans or runs, or get, cancel and delete a backup run

## What This Node Does

One node for the bookkeeping side of container backups. The `action` field selects the behaviour; only the fields that apply are shown.

- `list_plans` / `list_runs` return `{ items, count }` for the environment, optionally narrowed to one container. Runs can be filtered by status and capped with **Limit** (newest first, as the API returns them).
- `get_run` returns the run record (status, progress, destinations, archive size).
- `cancel_run` stops an in-flight backup or restore.
- `delete_run` removes the run and its archive from every destination.

Use `list_runs` + **Loop** + `delete_run` for retention policies the plan itself does not express, or `get_run` after **Run Backup Plan** to wait on a long run.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: Primary success path.
- Output `error`: Failure path used when the node returns an error.

## Configuration Guide

- `Environment` (environment-select, required)
- `Action` (select, required)
- `Container (filter)` (container-select) — shown when `{"action":["list_plans","list_runs"]}`
- `Status` (select) — shown when `{"action":"list_runs"}`
- `Limit` (number) — shown when `{"action":"list_runs"}`
- `Backup Run` (backup-run-select) — shown when `{"action":["get_run","cancel_run","delete_run"]}`

## Backend Binding

`executeContainerBackupManage` in `storage_backup_nodes.go`, backed by `ContainerBackupRunner.ListPlans` / `ListRuns` / `Run` / `CancelRun` / `DeleteRun` → `container_backups.Service`.
