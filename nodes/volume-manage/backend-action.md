# Backend Action

- Action key: `volume-manage`
- Dispatcher: `ExecuteNode` in `src/backend/modules/workflows/service.go` → `executeVolumeManage` in `merged_nodes.go`
- Selector: `action`
- Legacy aliases: `volume-create`, `volume-remove`, `volume-backup`, `volume-restore` (see `legacy_aliases.go`)
- Frontend node key: `volume-manage`
- Category: `action`
- Input ports: `input`
- Output ports: `output`, `error`

The router reads `action` and calls the pre-existing executor for that value. The alias table in `legacy_aliases.go` must match `LEGACY_NODE_TRANSFORMS` in `src/frontend/modules/workflows/utils/migrateWorkflow.ts`.
