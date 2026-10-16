# Backend Action

- Action key: `itsm-status-trigger`
- Fired by: `itsm.Service.ApplyRemote` through the hook installed with `SetStatusTrigger` in `src/backend/main.go`, which calls `TriggerService.FirePluginTrigger`
- Filter: `workflows.ITSMStatusMatches` (`channel_id`, `states`)
- Manual run: `ExecuteNode` returns an empty payload with topic `itsm-status-trigger`
- Frontend node key: `itsm-status-trigger`
- Category: `trigger`
- Input ports: none
- Output ports: `output`

Runs start only on the workflow-trigger leader, like every other trigger.
