# Backend Action

- Action key: `container-backup-restore`
- Dispatcher: `ExecuteNode` in `src/backend/modules/workflows/service.go` → `executeContainerBackupRestore` in `storage_backup_nodes.go`, backed by `ContainerBackupRunner.Restore` → `container_backups.Service.Restore`.
- Selector: none
- Legacy aliases: none
- Frontend node key: `container-backup-restore`
- Category: `action`
- Input ports: `input`
- Output ports: `output`, `error`
