# Backend Action

- Action key: `container-backup-manage`
- Dispatcher: `ExecuteNode` in `src/backend/modules/workflows/service.go` → `executeContainerBackupManage` in `storage_backup_nodes.go`, backed by `ContainerBackupRunner.ListPlans` / `ListRuns` / `Run` / `CancelRun` / `DeleteRun` → `container_backups.Service`.
- Selector: `action`
- Legacy aliases: none
- Frontend node key: `container-backup-manage`
- Category: `action`
- Input ports: `input`
- Output ports: `output`, `error`
