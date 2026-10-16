# Backend Action

- Action key: `storage-object`
- Dispatcher: `ExecuteNode` in `src/backend/modules/workflows/service.go` → `executeStorageObject` in `storage_backup_nodes.go`, backed by `StorageObjectOperator` (`internal/bootstrap/adapters.go` → `storage_locations.Service` object methods in `objects.go`).
- Selector: `action`
- Legacy aliases: none
- Frontend node key: `storage-object`
- Category: `action`
- Input ports: `input`
- Output ports: `output`, `error`
