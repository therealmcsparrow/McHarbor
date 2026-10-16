# Backend Action

- Action key: `encode`
- Dispatcher: `ExecuteNode` in `src/backend/modules/workflows/service.go` → `executeEncode` in `merged_nodes.go`
- Selector: `algorithm`
- Legacy aliases: `base64`, `hash` (see `legacy_aliases.go`)
- Frontend node key: `encode`
- Category: `utility`
- Input ports: `input`
- Output ports: `output`, `error`

The router reads `algorithm` and calls the pre-existing executor for that value. The alias table in `legacy_aliases.go` must match `LEGACY_NODE_TRANSFORMS` in `src/frontend/modules/workflows/utils/migrateWorkflow.ts`.
