# Backend Action

- Action key: `stack-manage`
- Dispatcher: `ExecuteNode` in `src/backend/modules/workflows/service.go` → `executeStackManage` in `merged_nodes.go`
- Selector: `action`
- Legacy aliases: `stack-deploy`, `compose-up`, `compose-down`, `stack-remove` (see `legacy_aliases.go`)
- Frontend node key: `stack-manage`
- Category: `action`
- Input ports: `input`
- Output ports: `output`, `error`

The router reads `action` and calls the pre-existing executor for that value. The alias table in `legacy_aliases.go` must match `LEGACY_NODE_TRANSFORMS` in `src/frontend/modules/workflows/utils/migrateWorkflow.ts`.
