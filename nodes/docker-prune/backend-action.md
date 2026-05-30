# Backend Action

- Action key: `docker-prune`
- Dispatcher: `ExecuteNode` in `src/backend/modules/workflows/service.go`
- Frontend node key: `docker-prune`
- Category: `action`
- Input ports: `input`
- Output ports: `output`, `error`

The frontend node folder and backend action key are expected to stay in sync. If the backend implementation changes, update this folder's documentation and translations together with the frontend node definition.
