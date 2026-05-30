# Backend Action

- Action key: `stack-deploy`
- Dispatcher: `ExecuteNode` in `src/backend/modules/workflows/service.go`
- Runtime implementation: `executeStackDeployRuntime` in `src/backend/modules/workflows/node_runtime_impl.go`
- Frontend node key: `stack-deploy`
- Category: `action`
- Input ports: `input`
- Output ports: `output`, `error`

Compose bytes are resolved from config or the workflow msg, then deployed through `docker compose up -d`.
