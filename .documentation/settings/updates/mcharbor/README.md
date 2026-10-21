# McHarbor

**Settings → Updates → McHarbor** · `?tab=updates-mcharbor`

Check for new McHarbor releases and install them.

## Permissions

| Key | What it unlocks |
|---|---|
| `updates.view` | See the section |
| `updates.manage` | Change settings and trigger an update |

A section you lack the key for renders **disabled with a tooltip naming it**, rather than
opening onto an empty panel.

## What it governs

- When McHarbor looks for a new release, and where it announces one.
- Updating now, whether updates install automatically, and **the window in which an automatic
  install may happen**.

## What counts as a new release

The check reads the latest release on GitHub and compares it with the running McHarbor by
**version, then build**. A newer version is always offered. A release with the same version is
offered when its build stamp (`yyyyMMddHHmm`) is newer than the running one, so a rebuild of a
version reaches you without a version bump.

The release's build comes from a `BUILD` file attached to it. A release without that file, and
an install whose own build is unstamped (`dev`), are compared on version alone. If the file
cannot be downloaded, the check keeps the build it last saw for that version, rather than
announcing the release again.

The banner, the notification and the automatic install name a rebuild as
`<version>+<build>` (`2026.10.01+202610220141`). Each rebuild is announced once and installed
once, and dismissing the banner dismisses that build only.

## Updates never land in the middle of work

The release check and the automatic install run on schedules of their own (**When updates
run**). An automatic install (and the automatic agent sweep) starts only inside the install
window. If a container backup or restore, an instance export or verification, or an instance
restore (maintenance mode) is still running when the window opens, the install waits and says
why ("Waiting for a container backup or restore to finish"), trying again every minute until the
window closes. A window that never goes quiet is skipped, and the section says so; the update
waits for the next window.

## Before turning automatic installs on

Self-update starts a container, so McHarbor needs a Docker socket it can write to.

**A native install does not update itself.** **Update now** is not offered there; the section
says instead to upgrade with the package manager (`apt` or `dnf`) or by running `install.sh`
from a newer tarball, and the automatic-install setting says it has no effect. See
[Native Linux install → Upgrading](../../../getting-started/installation/native-linux.md#5-upgrading).

More importantly, an automatic install is an unattended upgrade of the thing you would use
to diagnose a failed upgrade. Two habits make it safe: **back up the data volume on a
schedule**, because rollback across a schema change means restoring it, and **let agent
updates follow**, so agents do not fall behind a McHarbor that upgraded itself overnight.

For an install that matters, the safer shape is checker on, notifications to a channel
someone reads, automatic install off.

## In this folder

| Page | What it covers |
|---|---|
| [Reference](./reference.md) | Every setting, its default, and what changing it costs |

## Related

- [Getting started → Upgrading](../../../getting-started/upgrading/README.md): the whole path, including plugins and rollback
- [Agents](../agents/README.md): keeping agents on the matching version
- [System → About](../../system/about/README.md): the same controls beside version and dependencies
