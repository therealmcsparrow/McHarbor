# Backend Action

- Action key: `manual-trigger`
- Dispatcher: `ExecuteNode` in `src/backend/modules/workflows/service.go`
- Frontend node key: `manual-trigger`
- Category: `trigger`
- Input ports: none
- Output ports: `output`

The frontend node folder and backend action key are expected to stay in sync. If the backend implementation changes, update this folder's documentation and translations together with the frontend node definition.
