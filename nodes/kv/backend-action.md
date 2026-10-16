# Backend Action

- Action key: `kv`
- Dispatcher: `ExecuteNode` in `src/backend/modules/workflows/service.go` → `executeKv` in `merged_nodes.go`
- Selector: `operation`
- Legacy aliases: `kv-get`, `kv-set`, `kv-delete` (see `legacy_aliases.go`)
- Frontend node key: `kv`
- Category: `utility`
- Input ports: `input`
- Output ports: `output`

The router reads `operation` and calls the pre-existing executor for that value. The alias table in `legacy_aliases.go` must match `LEGACY_NODE_TRANSFORMS` in `src/frontend/modules/workflows/utils/migrateWorkflow.ts`.
