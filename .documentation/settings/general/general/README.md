# General

**Settings → General → General** · `?tab=general`

Language, time zone, date and time formats, and how often data refreshes.

## Permissions

| Key | What it unlocks |
|---|---|
| _readable in Settings_ | See the section |
| `settings.manage` | Change an app-wide value |

A section you lack the key for renders **disabled with a tooltip naming it**, rather than
opening onto an empty panel.

## What it governs

- The application name shown in the interface, useful when several McHarbor installs are open at once.
- Time and date format: 24-hour (`13:30`) or 12-hour (`1:30 PM`), `DD/MM/YYYY` or `MM/DD/YYYY`.
- Your language, and the default language the login page uses.
- How often lists and detail views re-fetch from the server.
- The time zone every cron expression in the product is read in.

## Global and personal

Several settings here carry a **Global** or **Personal** marker. Global is the app-wide
default; Personal is your own override. The marker tells you which one you are editing, so
a language change does not surprise everyone else.

## The time zone is the one that is not cosmetic

It decides what `0 3 * * *` means for every schedule, backup plan and update policy in the
product, and **changing it restarts McHarbor**: "McHarbor will save *Europe/Amsterdam* and
restart so the new time zone takes effect. You will be disconnected briefly."

Set it during first run, before any schedule exists, and you never have to reason about it
again. `TZ` in the container environment sets the container’s own zone; this setting is
what McHarbor uses for its own time maths.

**On a native install** McHarbor follows the host's time zone, and the picker and its save
button are disabled: "A native install uses the host's timezone, currently *zone*. Change it on
the host with sudo timedatectl set-timezone &lt;zone&gt;, then restart McHarbor." A save sent anyway
is refused (`409`, `err.invalid_body` with reason `native_timezone`) before anything is stored,
because there is no container to recreate with `TZ`.

## In this folder

| Page | What it covers |
|---|---|
| [Reference](./reference.md) | Every setting, its default, and what changing it costs |

## Related

- [Themes](../themes/README.md): appearance defaults and per-variable overrides
- [Notifications](../notifications/README.md): toast behaviour
- [Settings → Automation → Schedules](../../automation/schedules/README.md): what the time zone governs
- [Getting started → Configuration](../../../getting-started/configuration/README.md): `TZ` and the other environment variables
