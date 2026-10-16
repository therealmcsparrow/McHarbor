# Backend Action

- Action key: `object`
- Dispatcher: `ExecuteNode` in `src/backend/modules/workflows/service.go` → `executeObject` in `merged_nodes.go`
- Selector: `operation`
- Legacy aliases: `pick`, `omit`, `map` (see `legacy_aliases.go`)
- Frontend node key: `object`
- Category: `utility`
- Input ports: `input`
- Output ports: `output`, `error`

The router reads `operation` and calls the pre-existing executor for that value. The alias table in `legacy_aliases.go` must match `LEGACY_NODE_TRANSFORMS` in `src/frontend/modules/workflows/utils/migrateWorkflow.ts`.
