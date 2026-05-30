# Backend Action

- Action key: `ftp-upload`
- Dispatcher: `ExecuteNode` in `src/backend/modules/workflows/service.go`
- Runtime implementation: `executeFTPUploadRuntime` in `src/backend/modules/workflows/node_runtime_impl.go`
- Frontend node key: `ftp-upload`
- Category: `integration`
- Input ports: `input`
- Output ports: `output`, `error`

The runtime can upload either a local file or a temporary file generated from a workflow msg property.
