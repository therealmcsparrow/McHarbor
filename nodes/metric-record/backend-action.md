# Backend Action

- Action key: `metric-record`
- Dispatcher: `ExecuteNode` in `src/backend/modules/workflows/service.go`
- Persistence: `workflow_metrics` table created by `src/backend/core/db/migrations/036_workflow_metrics.sql`
- Frontend node key: `metric-record`
- Category: `utility`
- Input ports: `input`
- Output ports: `output`, `error`

The node reads a configured msg property, persists a metric sample row, and annotates the outgoing msg with `_metric`.
