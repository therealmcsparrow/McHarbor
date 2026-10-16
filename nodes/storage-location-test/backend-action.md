# Backend Action

- Action key: `storage-location-test`
- Dispatcher: `ExecuteNode` in `src/backend/modules/workflows/service.go` → `executeStorageLocationTest` in `storage_backup_nodes.go`, backed by `StorageObjectOperator.Test` / `Usage` → `storage_locations.Service.TestStorageLocation` / `Usage`.
- Selector: none
- Legacy aliases: none
- Frontend node key: `storage-location-test`
- Category: `action`
- Input ports: `input`
- Output ports: `output`, `error`
