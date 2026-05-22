# Backend Action

- Action key: `ssh-exec`
- Dispatcher: `ExecuteNode` in `src/backend/modules/workflows/service.go`
- Runtime implementation: `executeSSHExecRuntime` in `src/backend/modules/workflows/node_runtime_impl.go`
- Frontend node key: `ssh-exec`
- Category: `integration`
- Input ports: `input`
- Output ports: `output`, `error`

The runtime supports password auth, key auth, explicit port selection, and SSH URLs with embedded credentials.
