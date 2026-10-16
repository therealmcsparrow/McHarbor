# Triggers

How a run starts. Nine trigger node types, plus running by hand.

A workflow with no runnable entry node fails immediately with `no runnable entry nodes`, so
every workflow needs at least one.

## Active and inactive

The **Active** toggle arms **automatic triggers only**: "Workflow is inactive — auto-triggers
are disabled. Click to activate."

An inactive workflow still runs from the editor, and still runs from the API with
`workflows.run`. This is the safe way to build something that would otherwise fire while you
are still wiring it.

## The nine

| Trigger | Key | Fires when |
|---|---|---|
| Manual | `manual-trigger` | You press Run |
| Schedule | `schedule-trigger` | A cron expression matches |
| Webhook | `webhook-trigger` | An HTTP request arrives at its path |
| Container | `container-trigger` | A container changes state or health |
| Docker event | `docker-event-trigger` | Any Docker event matching the filter |
| Metric | `metric-trigger` | Container metrics cross a condition |
| Registry tag | `registry-tag-trigger` | A watched image tag's digest changes |
| File watch | `file-watch-trigger` | A watched path changes |
| Link in | `link-in` | Another workflow's **Link Out** stored a message |

Each one's fields are in [Trigger nodes](../nodes/triggers/README.md).

## Schedule

Cron is matched minute by minute against the current time in the trigger's own `timezone`
field, defaulting to `UTC`. The message carries `trigger`, `cron`, `timezone`, `timestamp`,
`datetime` and `unix`, with `topic` set to `schedule`.

Note the difference from the rest of the product: a **workflow** schedule trigger carries its
own time zone, while [Settings → Automation → Schedules](../../settings/automation/schedules/README.md)
and update policies read the app-wide time zone. If a workflow fires an hour off, this field
is the first place to look.

## Webhook

An incoming request to:

```
POST https://mcharbor.example.com/api/workflows/webhooks/<path>
```

is matched against the active workflows' webhook triggers by **path and method**. No session
is involved: this route is deliberately outside the protected group, because the caller is a
forge, a monitoring system or a script.

What guards it instead:

- **The workflow has to be active.** An inactive workflow's webhook 404s.
- **The path has to match**, after normalisation.
- **The secret, when set.** A trigger with a secret rejects a request that does not present a
  matching one with `401`, compared in constant time.
- An unparseable payload is `400`.

The message carries the body as `payload.body`, plus `req` with the method and headers, the
headers again at the top level, `topic: "webhook"` and `_triggerTimestamp`.

To answer the caller with something other than the default, put a **Webhook Response** or
**HTTP Response** node in the flow: whatever it captures becomes the HTTP response the caller
receives.

A webhook trigger with no secret is an unauthenticated entry point into whatever the workflow
does. Set one.

## Docker and container triggers

The trigger service watches the Docker event stream per environment and dispatches matching
events into workflows. **Container Trigger** narrows to state and health changes;
**Docker Event Trigger** takes type, action, name and label filters over everything the daemon
emits.

On an agent environment these arrive by whichever event mode is configured in
[Settings → System → Agent](../../settings/system/agent/README.md) (polling by default).

## Metric, registry tag and file watch

- **Metric** evaluates container metrics against a condition on the service's own schedule.
- **Registry tag** watches an image tag and fires when its digest changes. This is the digest-level
  check, so a silently rebuilt tag is caught.
- **File watch** fires when a watched path changes.

## Link in and link out

**Link Out** stores the current message under a name. **Link In** in another workflow consumes
the latest message from that link, and McHarbor fires the consuming workflow when the message
is stored.

This is the supported way to compose workflows: one workflow ends by publishing, another
begins by consuming, and neither has to know about the other beyond the link name.

## Plugin triggers

Plugins can bind their own events to workflow triggers, which is how a plugin-contributed
event reaches the canvas without the host knowing anything about it.

**VM Trigger** (`vm-trigger`) is one of them. It ships with the Virtual Machines plugin
(`addon-kvm`), appears in the palette only while that plugin is installed, and needs `vm.view`.
It fires when a guest changes state, filtered by environment, virtual machine, states and
severity. Workflows saved before it moved into the plugin keep their `vm-trigger` node and work
unchanged. While the plugin is uninstalled such a node stays on the canvas and does not fire. See
[Virtual Machines](../../virtual-machines/README.md#vm-trigger).

In an active/active cluster a plugin can raise its event on any node, while runs start only on
the node leading the workflow trigger. A fire elsewhere is forwarded to that node, which starts
each matched workflow once; with no leader reachable it is retried for up to a minute. See
[Active/active → Plugin workflow triggers](../../settings/system/high-availability/active-active.md#plugin-workflow-triggers).

## Who can run a workflow

| Action | Permission |
|---|---|
| Run from the editor or the API | `workflows.run` |
| Create or change a workflow | `workflows.manage` |
| Delete | `workflows.delete` |

Automatic triggers run as the system rather than as a person. A manual run carries the id of
the account that pressed Run, so plugin nodes that must act as a user act as that one.

## Related

- [Trigger nodes](../nodes/triggers/README.md): every field on each trigger
- [Execution model](../execution/README.md): what happens after a trigger fires
- [Runs and debugging](../runs/README.md): confirming a trigger actually fired
