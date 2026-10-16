# Backend Action

- Action key: `container-trigger`
- Dispatcher: `ExecuteNode` in `src/backend/modules/workflows/service.go` → `executeContainerTrigger` in `merged_nodes.go`
- Selector: `watch`
- Legacy aliases: `container-status-trigger`, `container-health-trigger` (see `legacy_aliases.go`)
- Frontend node key: `container-trigger`
- Category: `trigger`
- Input ports: none
- Output ports: `output`

The router reads `watch` and calls the pre-existing executor for that value. The alias table in `legacy_aliases.go` must match `LEGACY_NODE_TRANSFORMS` in `src/frontend/modules/workflows/utils/migrateWorkflow.ts`.
