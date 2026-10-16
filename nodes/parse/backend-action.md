# Backend Action

- Action key: `parse`
- Dispatcher: `ExecuteNode` in `src/backend/modules/workflows/service.go` → `executeParse` in `merged_nodes.go`
- Selector: `format`
- Legacy aliases: `parse-json`, `parse-yaml`, `parse-xml`, `parse-csv`, `parse-html` (see `legacy_aliases.go`)
- Frontend node key: `parse`
- Category: `utility`
- Input ports: `input`
- Output ports: `output`, `error`

The router reads `format` and calls the pre-existing executor for that value. The alias table in `legacy_aliases.go` must match `LEGACY_NODE_TRANSFORMS` in `src/frontend/modules/workflows/utils/migrateWorkflow.ts`.
