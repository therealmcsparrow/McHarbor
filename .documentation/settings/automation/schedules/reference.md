# Schedules: Reference

| Setting | Default | What it does |
|---|---|---|
| **Name** | — | Your label, such as "Nightly restart" |
| **Description** | empty | What the schedule is for, shown in the list |
| **Cron Expression** | — | Standard five-field cron, set through the readable builder or typed directly. `0 3 * * *` is every day at 3 AM |
| **Action** | — | Start, Stop, Restart or Exec |
| **Target** | — | A container name or id |
| **Active** | Active | Only on the edit dialog. A new schedule starts active; an inactive one is kept but never fires |

## When something does not behave

| Symptom | Usual cause | Fix |
|---|---|---|
| A schedule runs at the wrong hour | The configured time zone | Cron is read in the time zone from Settings → General |
| It never fires | It is Inactive, or the target no longer resolves | Check both; a renamed or recreated container leaves the schedule pointing at nothing |
| Exec does nothing visible | The command ran and produced no output | Check the audit trail and the container’s own logs |
| Two jobs fight over the same container | A schedule and a workflow or policy overlap | Pick one owner per container |
| The edit button is disabled | You hold `schedules.view` but not `schedules.manage` | The tooltip names the key; ask for it rather than deleting and recreating |
| No **Active** switch on a new schedule | It is edit-only by design | A schedule starts active; change it after saving |

## Related

- [Schedules](./README.md): what the section is for
- [Settings](../../README.md): the other groups
