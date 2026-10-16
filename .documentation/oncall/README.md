# On-call

The on-call view is a small, installable version of McHarbor for a phone. It does the five
things an operator does before deciding whether to get out of bed, and nothing else:

| Tab or screen | What it answers | Permission |
|---|---|---|
| **Alerts** | What fired, where, and what it said | `alerts.view` for alert history, `in_app_notifications.view` for notices |
| **Fleet** | Is this one thing, or is everything on fire | `environments.view`, `containers.view` |
| **Container** (from an alert or the fleet) | Its state, exit code, restarts and health checks; restart it | `containers.view`; `containers.manage` to restart |
| **Logs** (from a container) | Is it real or flapping: the last 300 lines, following live, with search | `logs.view` |
| **Approvals** | Approve or deny a pending temporary elevation | `users.manage` |
| **Runbook** (from an alert) | The steps to follow, one at a time, and the record of who followed them | `runbooks.view`; `runbooks.run` to record a step, plus the step's own key for an action |
| **Device** | Push notifications for this device, your other devices, and the layout | none; acts on your own account only |

Only the runbook screen added permission keys of its own: `runbooks.view`, `runbooks.manage`
and `runbooks.run`, described in [Runbooks](./runbooks/README.md). Every other screen calls the
same routes as the desktop interface, behind the same checks, and a control you may not use
renders disabled with the missing key named. Managing your own push devices is an action on your own account, covered
the same way as the rest of your profile; regenerating the instance push keys needs
`settings.manage`.

## Pages

| Page | Covers |
|---|---|
| [Installing on a phone](./install.md) | The manifest and icons, Android and iOS install, the certificate a phone needs, updates |
| [Push notifications](./push.md) | Turning them on, what is sent and through whom, the device list, delivery failures, the instance keys |
| [Offline and stale data](./offline.md) | What is cached, how staleness is shown, what is never cached, sessions that expire in the background |
| [Runbooks](./runbooks/README.md) | The runbook engine: step kinds, how a step is authorised against you, following one on a phone, what is offline, authoring, auditing |

API routes are in [Push and install](../api/endpoints/push.md).

## Reaching it

- **On a phone**, McHarbor switches to the on-call layout by itself: any screen narrower than
  640 px opens `/oncall` instead of the desktop interface.
- **On any screen**, open `/oncall` directly. The installed app always starts there.
- **Choosing a layout.** *Device → Layout* offers *Automatic (by screen width)*, *Always the
  on-call layout* and *Always the desktop layout*. The choice is stored in this browser only.
  *Open the desktop interface* switches this browser to the desktop layout and opens the
  dashboard. This is the way back for the case where only the desktop carries the control you need.

## Deep links

Every screen has a stable URL, and push notifications open them:

| URL | Screen |
|---|---|
| `/oncall/alerts` | The alert inbox |
| `/oncall/fleet` | The fleet glance |
| `/oncall/containers/<environment id>/<container id>` | One container |
| `/oncall/containers/<environment id>/<container id>/logs` | Its log tail |
| `/oncall/approvals` | Pending elevation requests |
| `/oncall/device` | This device |

An alert whose source is a container links to that container; every other alert links to the
inbox. A new elevation request links to *Approvals*.

A container alert that belongs to an [incident](../incidents/README.md) also shows the
incident's narrative (its summary and confidence) with a link to the incident, when you
hold `incidents.view` and the environment has narration turned on.

## What is deliberately missing

Stop, remove, recreate, a terminal, workflows, GitOps, settings and role editing stay on the
desktop. Nothing is queued while offline: a restart pressed without a connection is refused
on the spot rather than replayed later, because believing something restarted when it did not
is worse than being told the network is down.
