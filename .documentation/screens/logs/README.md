# Logs

`/logs`

Container log output, live or historical, for one container at a time.

## Permissions

| Key | What it unlocks |
|---|---|
| `logs.view` | Read container log output |

Everywhere except navigation, a control you lack the key for renders **disabled with a
tooltip naming it**, rather than disappearing. Navigation is the exception: a sidebar
entry you cannot use is hidden, because a dead-end page is not informative.

## What it shows

- A container selector: until you pick one the page says "Select a container to view logs" rather than showing an empty console.
- The log buffer, newest-first or oldest-first, with a free-text filter over the visible lines.
- A live indicator when the stream is attached.
- Severity colouring: each line is tinted from the level word it contains, with `error`/`fatal`/`panic` in the danger tone, `warn`/`warning` in the warning tone, `info` in the info tone, `trace`/`debug` muted, and anything with no level word in the normal text colour. It is a read of the line text, not a field Docker supplies, so a log format that never names its level stays uncoloured.

## What you are actually reading

Docker’s own log driver output for the container, stdout and stderr demultiplexed. Two
consequences follow:

- **A container with a non-default log driver may return nothing.** If the daemon ships
  logs straight to syslog or a remote collector, there is nothing local to read, and the
  page is empty with no error.
- **Rotation is Docker’s.** McHarbor does not store container logs; what you can scroll
  back to is what the log driver still holds.

## In this folder

| Page | What it covers |
|---|---|
| [Actions](./actions.md) | Everything you can do here, what gates it, and what it really does |
| [Fields](./fields.md) | Every column, filter and form field the screen exposes |
| [Troubleshooting](./troubleshooting.md) | Symptom, cause and fix |

## Related

- [Containers](../containers/README.md): the per-container Logs tab
- [Lifecycle log](../lifecycle/README.md): what happened *to* a container rather than what it printed
