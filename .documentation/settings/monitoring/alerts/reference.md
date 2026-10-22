# Alerts: Reference

| Setting | Default | What it does |
|---|---|---|
| **Rule name** | — | Your label, such as "High CPU usage" |
| **Severity** | — | Critical, Warning or Info. Carried through to delivery |
| **Alert type** | — | CPU usage, Memory usage, Disk usage, Container down, Image update |
| **Condition** | — | The signal being watched. Plugin-contributed conditions are labelled "Provided by the *X* plugin" |
| **Operator** and **Threshold** | — | `> 80`, and so on |
| **Evaluation window** | — | How much history is averaged before the comparison |
| **Sustained for** / **Down for** | Immediately | How long the condition must hold before firing |
| **Target** | `*` | A glob, such as `frontend-*` or `database`. Empty matches every target |
| **Environments** | all | "None selected means all of them" |
| **Destinations** | — | In-app notifications, and any configured channel. At least one required |
| **Minimum severity** | Any severity | Per destination |
| **Cooldown** | none | "Minimum time between two notifications from this rule" |
| **Notify on recovery** | Off | "Send a follow-up when the condition clears again" |
| **Message template** | default | Leave empty for the default message; see [Message templates](#message-templates) |
| **Status** | Enabled | Enabled is "active and can trigger"; Disabled is "stored but currently muted" |

Recovery notifications are worth turning on for anything that pages a person. An alert with
no matching recovery leaves someone checking by hand whether the thing they were woken for
is still broken.

**Send test** delivers through the rule’s real destinations, so it confirms the whole path
rather than the rule alone.

## Message templates

A template is plain text with placeholders in double braces. The **Template reference** beside
the field lists them (click one to insert it at the cursor), and the **Preview** under it
renders the draft with sample values, as **Firing** or **Resolved**, through the same renderer
the alert engine uses. A placeholder the renderer does not know is sent as written, and the
preview names it: "Not recognised, sent as written".

| Placeholder | Value |
|---|---|
| `{{alert}}` | Name of the rule |
| `{{severity}}` | `info`, `warning` or `critical` |
| `{{state}}` | `firing` or `resolved` |
| `{{condition}}` | Name of the condition |
| `{{rule}}` | The condition as configured, such as "Docker disk usage > 85%" |
| `{{operator}}`, `{{threshold}}` | Comparison operator, and the threshold with its unit |
| `{{value}}` | The measured value, formatted |
| `{{subject}}`, `{{subjectId}}` | What the alert is about (a container, host, plan) and its stable id |
| `{{environment}}`, `{{environmentId}}` | Environment name and id |
| `{{duration}}` | How long it fired; filled in on recovery |
| `{{alertId}}` | Id of the rule |
| `{{time}}`, `{{date}}`, `{{timestamp}}` | When it was sent: `HH:MM`, `YYYY-MM-DD`, RFC 3339, all UTC |

**Condition values** (the **Condition** tab) are `{{ctx.<key>}}` placeholders the chosen
condition fills in: `{{ctx.used_bytes}}` for Docker disk usage, `{{ctx.remote_digest}}` for an
image update, `{{ctx.ip}}` for failed logins, `{{ctx.error}}` for a failed instance backup.
Plugin conditions declare no list, so the tab shows the keys McHarbor has seen them produce
since the last restart, marked "Seen on this condition's recent evaluations".

Routes, both behind `alerts.view`: `GET /api/alerts/template-reference?source=&condition=` and
`POST /api/alerts/template-preview`, which stores and sends nothing.

## Host disk conditions

| Condition | What it does |
|---|---|
| **Host filesystem usage** (`host_fs_percent`) | Percentage of the filesystem under the host, read without asking Docker. Operators `>`, `>=`, `<` and `<=`; default `>=` 85%. A host whose disk cannot be read is skipped, never counted as 0%. |
| **Host disk data missing** (`host_fs_stale`) | Active while McHarbor has no usable disk reading, counted from the last good one. After a restart it counts from the newest stored capacity sample. Silent for an environment McHarbor cannot read the disk of at all, such as a TCP or SSH connection that is not an agent. |
| **Docker disk usage** (`docker_disk`) | When the Docker driver reports no capacity, the percentage uses the host filesystem's total. The `{{ctx.capacity_source}}` placeholder says which: `driver` or `host_fs`. |
| **Days until full** (`capacity_days`) | A filesystem already at its red line reports 0, labelled "past the red line". Below the line, a subject with too little history reports nothing. |

The environment card's disk bar uses the lowest enabled **Host filesystem usage** rule that covers
the environment for its warning and critical colours. With none, the colours are 85% and 95%.

The `host_fs_stale` rule puts its reason in `{{ctx.reason}}`: `agent_offline`, `agent_too_old` or
`read_failed`. It also carries `{{ctx.last_seen}}` when there was a good reading.

## When something does not behave

| Symptom | Usual cause | Fix |
|---|---|---|
| A rule never fires | The sustain duration is longer than the incident | Shorten **Sustained for**, or widen the evaluation window |
| An alert fires but nobody hears | Delivery failed, or no external destination | History records delivery separately from firing |
| Alerts arrive in a flood | No cooldown | Set one; it is the minimum time between two notifications from the rule |
| A destination vanished | The channel was deleted | "N destinations no longer exist and are ignored"; repoint the rule |
| Nothing happens during an incident | The rule is Disabled | Muting is per rule and survives, which is why it beats deleting |
| **Host disk data missing** fires | McHarbor has no usable disk reading; `{{ctx.reason}}` says why | `agent_offline`: bring the agent back online. `agent_too_old`: update the agent. `read_failed`: check the host's disk |
| A disk bar is grey, reading "No disk data" | The same cause as above | Follow the fix above |
| A disk bar reads "Not monitored" | McHarbor cannot read that environment's disk, so no disk alert fires for it | Connect it through an agent to monitor its disk |

## Related

- [Alerts](./README.md): what the section is for
- [Settings](../../README.md): the other groups
