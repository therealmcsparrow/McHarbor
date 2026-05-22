# Backend Action

- Action key: `filter`
- Dispatcher: `ExecuteNode` in `src/backend/modules/workflows/service.go`
- Frontend node key: `filter`
- Category: `logic`
- Input ports: `input`
- Output ports: `pass`, `block`

The frontend node folder and backend action key are expected to stay in sync. If the backend implementation changes, update this folder's documentation and translations together with the frontend node definition.
