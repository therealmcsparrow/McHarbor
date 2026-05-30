# Backend Action

- Action key: `ping`
- Dispatcher: `ExecuteNode` in `src/backend/modules/workflows/service.go`
- Frontend node key: `ping`
- Category: `integration`
- Input ports: `input`
- Output ports: `output`, `error`

The frontend node folder and backend action key are expected to stay in sync. If the backend implementation changes, update this folder's documentation and translations together with the frontend node definition.
