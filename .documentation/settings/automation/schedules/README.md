# Schedules

**Settings → Automation → Schedules** · `?tab=schedules`

Recurring jobs McHarbor runs on its own.

## Permissions

| Key | What it unlocks |
|---|---|
| `schedules.view` | See scheduled tasks |
| `schedules.manage` | Create and change them |
| `schedules.delete` | Delete one |

A section you lack the key for renders **disabled with a tooltip naming it**, rather than
opening onto an empty panel.

## What it governs

- A cron expression, an action, and a target container.
- Whether each schedule is active.

## Creating and editing one

**Add Schedule** in the section head, and **Edit schedule** on a row, open the same dialog. There
is one dialog rather than two, so a field added to a schedule is never added to only half of it.

| Field | Create | Edit |
|---|---|---|
| **Name**, **Description** | yes | yes |
| **Cron Expression**: the schedule itself | yes | yes |
| **Action**: Start, Stop, Restart or Exec | yes | yes |
| **Target**: a container name or id | yes | yes |
| **Active** | — | yes |

**Active appears only when editing.** A schedule starts active, and the create route does not
accept the flag, so offering the switch on a new one would be a control that cannot be sent.

**Cron Expression** is the readable schedule builder, not a bare text box: *every 1 day starting
at 03:00* in its readable mode, the expression itself in its advanced mode, and the next few fire
times under both. It opens in whichever mode can express the schedule you are editing, and either
way the value stored is the cron expression.

Both creating and editing need `schedules.manage`; without it the buttons render **disabled with a
tooltip naming the key**, rather than disappearing.

Editing is why a schedule with a wrong cron, action or target no longer has to be deleted and
made again. Recreating one loses its execution history, which is the record of whether it has
been working.

## Cron and the time zone

Cron expressions are evaluated in the time zone set in
[General](../../general/general/README.md), not in UTC and not in the browser’s zone.

This is the single most common cause of a schedule that runs "an hour off" or at the wrong
time entirely. Set the time zone once, before writing schedules, and the expressions mean
what they look like they mean.

## Exec runs unattended

The **Exec** action runs a command inside the target container on the schedule. It is the
escape hatch for maintenance a container does not do for itself (a database dump, a cache
trim, a log rotation the image never wired up), and it runs as whatever user the image runs
as. Treat it with the same care as a cron entry on the host.

## What this is not

Backups are configured on the backup plan, image update checks in
[Updates → Containers](../../updates/containers/README.md), and anything with branching or
several steps belongs in a [workflow](../../../screens/workflows/README.md), which has run
history and error handling that a cron line does not.

## In this folder

| Page | What it covers |
|---|---|
| [Reference](./reference.md) | Every setting, its default, and what changing it costs |

## Related

- [General](../../general/general/README.md): the time zone every cron expression is read in
- [Workflows](../../../screens/workflows/README.md): scheduled automation with logic
- [Webhooks](../webhooks/README.md): the event-driven counterpart
