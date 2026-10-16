# Activity

`/activity`

The real-time container event feed, straight from the Docker daemon.

## Permissions

| Key | What it unlocks |
|---|---|
| `activity.view` | Read the container event feed |

Everywhere except navigation, a control you lack the key for renders **disabled with a
tooltip naming it**, rather than disappearing. Navigation is the exception: a sidebar
entry you cannot use is hidden, because a dead-end page is not informative.

## What it shows

- One row per Docker event, newest first, arriving live.
- The attribute map Docker attached to each event: labels, exit code, signal and anything else the daemon included.

## Activity, lifecycle, audit: which is which

| Screen | Answers | Source |
|---|---|---|
| **Activity** | What did Docker just do? | The daemon’s event stream, largely unprocessed |
| [Lifecycle log](../lifecycle/README.md) | What has happened to my containers, images, volumes, networks and stacks over time? | The same events, interpreted, with severity and subject types |
| [Audit](../audit/README.md) | Who asked McHarbor to do what? | McHarbor’s own tamper-evident trail |

A container that died at 03:00 appears in Activity and in the Lifecycle log. A person who
clicked **Stop** appears in Audit. If nothing in Audit corresponds to a stop, the container
stopped on its own, which is usually the answer you were looking for.

## In this folder

| Page | What it covers |
|---|---|
| [Actions](./actions.md) | Everything you can do here, what gates it, and what it really does |
| [Fields](./fields.md) | Every column, filter and form field the screen exposes |
| [Troubleshooting](./troubleshooting.md) | Symptom, cause and fix |

## Related

- [Lifecycle log](../lifecycle/README.md): the interpreted, filterable version
- [Audit](../audit/README.md): who asked for it
- [Settings → Monitoring → Data retention](../../settings/monitoring/data-retention/README.md): how long events are kept
