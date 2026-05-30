# Backend Action

- Action key: `image-build`
- Dispatcher: `ExecuteNode` in `src/backend/modules/workflows/service.go`
- Runtime implementation: `executeImageBuildRuntime` in `src/backend/modules/workflows/node_runtime_impl.go`
- Frontend node key: `image-build`
- Category: `action`
- Input ports: `input`
- Output ports: `output`, `error`

The runtime tars the selected build context and calls the Docker API with optional target stage, build args, and cache controls.
