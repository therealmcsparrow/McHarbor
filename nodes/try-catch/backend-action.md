# Backend Action

- Action key: `try-catch`
- Dispatcher: `ExecuteNode` in `src/backend/modules/workflows/service.go`
- Runtime helpers: `src/backend/modules/workflows/try_catch.go` and `src/backend/modules/workflows/executor.go`
- Frontend node key: `try-catch`
- Category: `logic`
- Input ports: `input`
- Output ports: `output`, `catch`

The node pushes a catch frame onto the workflow msg.
The shared executor consumes that frame when a guarded downstream node fails and reroutes the msg through the `catch` output.
