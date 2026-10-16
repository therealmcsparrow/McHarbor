# Storage Object

List, upload, download or delete objects on a storage location through its adapter

## What This Node Does

Talks to a configured storage location through the storage adapter contract (`Test`, `Upload`, `Download`, `Delete`, `List`). Local destinations are served from their base path; every other location type is dispatched to the plugin that registered the adapter, so the same node works for SFTP, Samba, S3-compatible stores, OneDrive, Google Drive and any future storage plugin. No credential ever enters the workflow message.

- `list` returns `{ items: [{ key, size, lastModified }], count }` under the prefix (capped at 1000 entries).
- `upload` writes a message property (strings verbatim, objects as JSON, optionally base64-decoded) or inline text to the object key.
- `download` reads the object (64 MB cap) into the output property as text, parsed JSON or base64.
- `delete` removes the key; a missing key is not an error.

Adapters that did not declare `SupportsListing` / `SupportsDelete` route to the `error` port for those actions.

## Port Behavior

- Input `input`: Receives the incoming msg on this port.
- Output `output`: Primary success path.
- Output `error`: Failure path used when the node returns an error.

## Configuration Guide

- `Storage Location` (storage-location-select, required)
- `Action` (select, required)
- `Prefix` (text) — shown when `{"action":"list"}`
- `Object Key` (text) — shown when `{"action":["upload","download","delete"]}`
- `Source` (select) — shown when `{"action":"upload"}`
- `Source Property` (expression) — shown when `{"action":"upload","source_mode":"property"}`
- `Source Encoding` (select) — shown when `{"action":"upload","source_mode":"property"}`
- `Content` (textarea) — shown when `{"action":"upload","source_mode":"text"}`
- `Output Encoding` (select) — shown when `{"action":"download"}`
- `Output Property` (expression) — shown when `{"action":"download"}`

## Backend Binding

`executeStorageObject` in `storage_backup_nodes.go`, backed by `StorageObjectOperator` (`internal/bootstrap/adapters.go` → `storage_locations.Service` object methods in `objects.go`).
