# Backend Action

- Action key: `file`
- Dispatcher: `ExecuteNode` in `src/backend/modules/workflows/service.go` → `executeFile` in `merged_nodes.go`
- Selector: `operation`
- Legacy aliases: `read-file`, `write-file` (see `legacy_aliases.go`)
- Frontend node key: `file`
- Category: `utility`
- Input ports: `input`
- Output ports: `output`, `error`

The router reads `operation` and calls the pre-existing executor for that value. The alias table in `legacy_aliases.go` must match `LEGACY_NODE_TRANSFORMS` in `src/frontend/modules/workflows/utils/migrateWorkflow.ts`.
