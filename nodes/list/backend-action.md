# Backend Action

- Action key: `list`
- Dispatcher: `ExecuteNode` in `src/backend/modules/workflows/service.go` → `executeList` in `merged_nodes.go`
- Selector: `resource`
- Legacy aliases: `container-list`, `image-list`, `network-list`, `volume-list`, `stack-list` (see `legacy_aliases.go`)
- Frontend node key: `list`
- Category: `action`
- Input ports: `input`
- Output ports: `output`, `error`

The router reads `resource` and calls the pre-existing executor for that value. The alias table in `legacy_aliases.go` must match `LEGACY_NODE_TRANSFORMS` in `src/frontend/modules/workflows/utils/migrateWorkflow.ts`.
