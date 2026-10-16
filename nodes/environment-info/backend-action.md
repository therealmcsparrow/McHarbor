# Backend Action

- Action key: `environment-info`
- Dispatcher: `ExecuteNode` in `src/backend/modules/workflows/service.go` → `executeEnvironmentInfo` in `merged_nodes.go`
- Selector: `scope`
- Legacy aliases: `docker-info`, `host-info`, `environment-status` (see `legacy_aliases.go`)
- Frontend node key: `environment-info`
- Category: `action`
- Input ports: `input`
- Output ports: `output`, `error`

The router reads `scope` and calls the pre-existing executor for that value. The alias table in `legacy_aliases.go` must match `LEGACY_NODE_TRANSFORMS` in `src/frontend/modules/workflows/utils/migrateWorkflow.ts`.
