# Lifecycle Log

`/lifecycle`

Docker, image, volume, network and stack lifecycle events across **all** environments,
interpreted and filterable, plus the objects the add-ons own: **virtual machines** from
Virtual Machines and **storage pools** from Compute Foundation.

The add-on subjects come from the plugins themselves. A plugin that declares a
`lifecycle-subject` capability can record events for that subject type, and the subject filter
offers it. With the plugin uninstalled, its subject type leaves the filter, and McHarbor refuses
new events of that type. Rows it already wrote stay in the log.

## Permissions

| Key | What it unlocks |
|---|---|
| `events.view` | Read lifecycle events across every environment |

Everywhere except navigation, a control you lack the key for renders **disabled with a
tooltip naming it**, rather than disappearing. Navigation is the exception: a sidebar
entry you cannot use is hidden, because a dead-end page is not informative.

## What it shows

- One row per event: subject, what happened in plain words, severity, and when.
- A total count and pagination: "1,284 events".
- An expandable detail carrying the event id, the subject, the event and action, and the source.

The **source** names what produced the row: `docker` for the event collector, or a plugin
slug such as `addon-kvm` for an add-on's own objects. A virtual machine appears here on a
genuine transition only. The reconciler re-asserts a running guest's state once a minute,
and those re-assertions are not events.

## The vocabulary

Actions are rendered as plain past tense rather than Docker’s verbs: started, stopped,
died, killed, paused, resumed, restarted, created, removed, pulled, loaded, imported,
tagged, untagged, deleted, mounted, unmounted, connected, disconnected, **killed (OOM)**,
failed, errored.

States are equally plain: running, stopped, paused, created, restarting, removed, exited,
available, in use, dangling, tagged, active, up, partially up, errored.

`killed (OOM)` is the one worth scanning for. It means the kernel killed the container for
exceeding its memory limit, and it shows up here well before anyone thinks to look at the
container’s memory settings.

## In this folder

| Page | What it covers |
|---|---|
| [Actions](./actions.md) | Everything you can do here, what gates it, and what it really does |
| [Fields](./fields.md) | Every column, filter and form field the screen exposes |
| [Troubleshooting](./troubleshooting.md) | Symptom, cause and fix |

## Related

- [Activity](../activity/README.md): the live, per-environment, raw feed
- [Audit](../audit/README.md): who asked for it
- [Settings → Monitoring → Data retention](../../settings/monitoring/data-retention/README.md): how long rows are kept
