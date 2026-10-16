# Backend Action

- Action key: `send-communication`
- Dispatcher: `ExecuteNode` in `src/backend/modules/workflows/service.go`
- Executor: `executeSendCommunication` → `executeSendConfiguredCommunication`
- Legacy aliases: `send-slack`, `send-discord`, `send-teams`, `send-telegram`, `send-whatsapp`, `send-signal`, `send-ntfy`, `send-gotify`, each claimed by its comm plugin as a channel-type `workflow-node-alias` (see `channel_caps.go`)
- Frontend node key: `send-communication`
- Category: `integration`
- Input ports: `input`
- Output ports: `output`, `error`

Behaviour:

- The node has no `channel_type` field. An empty `channel_id` resolves to the global default channel.
- Nodes rewritten from a legacy `send-<provider>` key carry `channel_type: <provider>`; with an empty `channel_id` that resolves to the default channel of that provider.
- `recipient`, `title` and `message` are rendered with `renderMsgTemplate` (`{{path}}` against msg) before sending.
- A non-empty `recipient` is passed as `ChannelRequest.Recipient`. The notify dispatcher writes it into the adapter's declared overridable destination key and refuses the send when the adapter declares none.

The frontend node folder and backend action key are expected to stay in sync. If the backend implementation changes, update this folder's documentation and translations together with the frontend node definition. The alias table in `legacy_aliases.go` must match `LEGACY_NODE_TRANSFORMS` in `src/frontend/modules/workflows/utils/migrateWorkflow.ts`.
