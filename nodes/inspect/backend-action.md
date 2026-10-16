# Backend Action

- Action key: `inspect`
- Dispatcher: `ExecuteNode` in `src/backend/modules/workflows/service.go` → `executeInspect` in `merged_nodes.go`
- Selector: `resource`
- Legacy aliases: `container-inspect`, `image-inspect`, `network-inspect`, `volume-inspect`, `stack-status` (see `legacy_aliases.go`)
- Frontend node key: `inspect`
- Category: `action`
- Input ports: `input`
- Output ports: `output`, `error`

The router reads `resource` and calls the pre-existing executor for that value. The alias table in `legacy_aliases.go` must match `LEGACY_NODE_TRANSFORMS` in `src/frontend/modules/workflows/utils/migrateWorkflow.ts`.
