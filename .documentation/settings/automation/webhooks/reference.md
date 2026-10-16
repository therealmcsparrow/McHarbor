# Webhooks: Reference

| Setting | Default | What it does |
|---|---|---|
| **Name** | — | Your label |
| **URL** | — | Where the POST goes |
| **Events** | — | Comma-separated event names, such as `container.start,container.stop` |
| **Status** | Active | Active or Inactive |

## When something does not behave

| Symptom | Usual cause | Fix |
|---|---|---|
| The receiver never fires | The event name does not match a canonical action | Check the names in the audit action browser |
| Test works, real events do not | The subscribed events are not the ones happening | Narrow or correct the event list |
| The receiver is overwhelmed | Too broad a subscription on a busy host | Subscribe narrowly, or move the work into a workflow |
| A missed event is never retried | There is no retry queue here | Use an alert rule or a workflow for anything that must not be missed |

## Related

- [Webhooks](./README.md): what the section is for
- [Settings](../../README.md): the other groups
