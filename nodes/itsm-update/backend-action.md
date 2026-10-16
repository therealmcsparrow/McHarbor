# Backend Action

- Action key: `itsm-update`
- Dispatcher: `ExecuteNode` in `src/backend/modules/workflows/service.go`
- Executor: `executeITSM` in `src/backend/modules/workflows/itsm_nodes.go`
- Frontend node key: `itsm-update`
- Category: `integration`
- Input ports: `input`
- Output ports: `output`, `error`

Behaviour:

- Emits an `incident.Event` with state `update` through the emitter installed by `workflows.SetITSMEmitter` (the `modules/itsm` service), which queues it in `incident_outbox`.
- `dedup_key`, `title` and `message` are rendered with `renderMsgTemplate`. An empty key falls back to `wf:{{_msgid}}`, which ties the ticket to one run.
- `HandleUpdate` and `HandleImport` refuse a canvas containing `itsm-open`, `itsm-update` or `itsm-resolve` unless the caller holds `alerts.manage` (`mayUseITSMNodes`).
