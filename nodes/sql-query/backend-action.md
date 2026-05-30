# Backend Action

- Action key: `sql-query`
- Dispatcher: `ExecuteNode` in `src/backend/modules/workflows/service.go`
- Frontend node key: `sql-query`
- Category: `utility`
- Input ports: `input`
- Output ports: `output`, `error`

The frontend node folder and backend action key are expected to stay in sync. If the backend implementation changes, update this folder's documentation and translations together with the frontend node definition.
The runtime enforces read-only access to workflow tables and caps returned rows to prevent unbounded scans from workflow-authored queries.
