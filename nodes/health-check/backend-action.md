# Backend Action

- Action key: `health-check`
- Dispatcher: `ExecuteNode` in `src/backend/modules/workflows/service.go`
- Frontend node key: `health-check`
- Category: `action`
- Input ports: `input`
- Output ports: `healthy`, `unhealthy`

The frontend node folder and backend action key are expected to stay in sync. If the backend implementation changes, update this folder's documentation and translations together with the frontend node definition.
