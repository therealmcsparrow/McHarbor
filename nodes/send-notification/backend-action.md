# Backend Action

- Action key: `send-notification`
- Dispatcher: `ExecuteNode` in `src/backend/modules/workflows/service.go`
- Frontend node key: `send-notification`
- Category: `integration`
- Input ports: `input`
- Output ports: `output`, `error`

The frontend node folder and backend action key are expected to stay in sync. If the backend implementation changes, update this folder's documentation and translations together with the frontend node definition.

This action now targets the built-in McHarbor in-app notification system only. External transports are handled by their dedicated workflow nodes.
