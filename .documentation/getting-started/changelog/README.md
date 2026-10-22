# Changelog

What changed in each McHarbor release, and what an upgrade will ask of you.

The authoritative file is [`CHANGELOG.md`](../../../CHANGELOG.md) in the repository root;
it carries each release as a summary of what an operator gets, loses or must act on. This
page is the operator's view of it: the current release with its breaking changes first, then
a table of every release back to 1.1.0.

The documentation set has a changelog of its own,
[`.documentation/CHANGELOG.md`](../../CHANGELOG.md), because documentation pages carry no
date stamps. That one records what changed in the *docs*, and against which McHarbor
version each page was verified.

## Versioning

McHarbor uses calendar versions, `yyyy.MM.<sequential>`: the year and month the version was
opened, then a counter that starts at `01` each month. The counter carries no compatibility
meaning: `2026.10.02` follows `2026.10.01` whether the change was a typo or a rewrite, so
read the release notes rather than the number. The current version is the single line in
the repository's `VERSION` file, the same string the About page shows. A separate `BUILD`
stamp (`yyyyMMddHHmm`) records when the artifact was built.

Releases up to 2.0.8 used `MAJOR.MINOR.PATCH`; 2026.10.01 is the first calendar version.

**The remote agent shares McHarbor's version** and is released with it. Some features still
name an older agent floor (interactive terminal exec needs agent v1.1.0 or newer, offline
buffering arrived with agent 2.4.0, compute needs 3.0.0). Those floors keep their old numbers,
and every agent from 2026.10.01 on clears them. [Settings → Updates → Agents](../../settings/updates/agents/README.md)
pushes the matching binary to every connected agent.

## Before you upgrade

Read the release notes for **every version you are skipping**, not just the one you are
moving to. A breaking change in 2.0.0 still applies if you jump from 1.6.6 to 2026.10.01.
[Upgrading](../upgrading/README.md) covers the mechanics, the rollback path, and why
backing up the data volume is step 1.

## Current release

### 2026.10.02 (2026-10-22)

A patch over 2026.10.01. Nothing breaks and no existing setting changes. McHarbor now detects a
full host disk, and shows a disk it cannot read as unknown rather than healthy.

**What an upgrade asks of you:** three default disk alerts are added on first start. They are
ordinary rules: edit, disable or delete them under Settings → Monitoring → Alerts. Update each
agent to the 2026.10.02 build to get the startup cleanup of its orphaned temporary archives. An
agent that cannot report host stats shows **No disk data**, and the alert names the cause as
`agent_too_old`.

### 2026.10.01 (2026-09-21)

A major release, compared with 2.0.8. The core became a slim host shipping Docker management,
dashboards, workflows, backups and governance, while Kubernetes, Swarm, reporting, storage
providers, communication channels, identity providers, scanners, log forwarders, snapshot
providers and AI providers moved into signed plugins.

**Breaking changes (read these before upgrading):**

| Change | What it means for an existing install |
|---|---|
| **Capabilities moved into plugins** | Kubernetes, Docker Swarm, reporting, every external storage provider, every communication channel, every external identity provider, the vulnerability scanners, log forwarders, snapshot providers and AI providers are no longer in the core. Install the matching plugin from the Plugin Store to keep each one. |
| **Removed** | Podman environments, blueprints (use the App Store and stack templates), the topbar help button and its help registry, the built-in storage providers and email-server settings, and seven routes nothing called. |
| **HTTPS by default** | A local certificate authority signs the server certificate on first boot, and the main listener serves TLS on `8705`. TLS 1.3 is the default floor; `TLS_MIN_VERSION=1.2` allows older clients. Install the root once per client to lose the browser warning; see [Installing the root CA](../../screens/security/install-root-ca.md). |
| **Every mutating route needs a permission key** | Routes that needed only a session now need the matching key, and API keys carry an explicit permission list. |
| **McHarbor's own containers and host need `host_self.manage`** | Exec, terminals, file writes, lifecycle actions, recreate and remove on McHarbor's containers; creating containers or stacks on McHarbor's environment with host-level access (host binds, the Docker socket, privileged options); and saving a workflow that acts on McHarbor's containers. See [Self-protection](../../screens/containers/self-protection/README.md). |
| **Credentials are masked unless revealed** | Container inspect, image inspect and history, stack compose text and env vars, and VM cloud-init return `••••••••` for credential-looking values. Revealing needs the `manage` key and is audited. Image export needs `images.manage`. |
| **Everyone is signed out once** | Sessions are now stored hashed. API keys are unaffected. |
| **Stack routes act on the stack's own environment** | Whatever `?env=` says. |
| **Compose runs no longer inherit McHarbor's environment** | They get an allow-list plus the stack's own variables. |
| **The Files tab protects McHarbor's data** | McHarbor's data directory, secrets and `/proc` need `system.manage`. |
| **Forwarding headers count only from `MCHARBOR_TRUSTED_PROXY_CIDRS`** | Behind a reverse proxy, set it. |
| **Exporting the audit log needs `audit.export`** | Roles that exported before need the key added. |
| **A fresh install needs a one-time setup token** | Read it from the container log before creating the first account. |
| **Agents install and update only from McHarbor** | Never from GitHub or a registry directly. |
| **An unrecognised `MCHARBOR_DB_DRIVER` stops the start** | Instead of falling back to SQLite. |
| **The container image is Debian-based** | `debian:bookworm-slim`; Alpine is not supported as a host. |

**Headline additions:** the plugin platform and Plugin Store with 68 first-party plugins and
self-repair after an upgrade; virtual machines and system containers through three compute
plugins; plugin commands in the CLI; host-sourced environment metrics; scheduled self-update
with an install window; two-factor authentication; a tamper-evident audit log; encryption at
rest for every stored credential; and a flatter interface.

The full 2026.10.01 entry, including Fixed and Security, is in
[`CHANGELOG.md`](../../../CHANGELOG.md).

## Release history

| Version | Date | What landed |
|---|---|---|
| **2026.10.02** | 2026-10-22 | Disk-full detection: host disk alerts and three default rules; a grey "No disk data" state where McHarbor cannot read the disk; startup cleanup of orphaned temporary archives. Patch; no breaking changes. |
| **2026.10.01** | 2026-09-21 | Plugin platform and Plugin Store; compute (VMs and system containers); RBAC on every route; tamper-evident audit log; encryption at rest; HTTPS by default; calendar versioning. **Breaking**; see above. |
| 2.0.8 | 2026-07-26 | First-run wizard, in-app help, keyboard shortcuts, skeleton loaders, empty states, bulk operations, saved searches, What's New overlay, dashboard templates. |
| 2.0.7 | 2026-07-25 | Container update no longer resets a custom network to `bridge`; container move keeps the original image reference; self-update settings actually persist. `react-router` 8.3.0 clears a CSRF advisory. |
| 2.0.6 | 2026-07-24 | Signal CLI and Asra app definitions; notification-channel hint pointed at the wrong Settings tab. |
| 2.0.5 | 2026-07-20 | Backup-key installation no longer 500s when container auto-discovery fails. |
| 2.0.4 | 2026-07-17 | Raw i18n key rendered in two test dialogs; 73 missing locale keys restored. |
| 2.0.3 | 2026-07-17 | Manual `projectPath` for backup-key install outside Compose. |
| 2.0.2 | 2026-07-17 | **Update now** on the About page; `system.manage` added to the Admin role. |
| 2.0.1 | 2026-07-17 | Lifecycle Log page; security-scan history page. |
| **2.0.0** | 2026-07-08 | Active-active multi-node deployment behind a load balancer; PostgreSQL backend with all 58 migrations made dialect-portable. MySQL/MariaDB followed on 2026-07-10. |
| 1.6.6 | 2026-07-02 | Backups overview page; per-destination backup retry upload. |
| 1.6.5 | 2026-06-30 | Backup retry with live byte progress; storage-type icons per destination. |
| 1.6.4 | 2026-06-27 | Edit a saved backup schedule; container-name resolution for stale ids on the agent. |
| 1.6.3 | 2026-06-26 | **Clear now** for audit and activity log retention. |
| 1.6.2 | 2026-06-22 | Clipboard copy on plain-HTTP LAN addresses no longer silently failed. |
| 1.6.1 | 2026-06-21 | System menu removed from the sidebar. |
| **1.6.0** | 2026-06-16 | Host page with live CPU, memory, load, filesystem and Docker disk breakdown; `host.view` / `host.manage` permissions. |
| 1.5.5 | 2026-06-16 | Used/unused visibility on networks; remove an orphaned network with its container. |
| 1.5.4 | 2026-06-13 | Agent configuration migrated to the `.opencode/` layout. |
| 1.5.3 | 2026-06-12 | App Store Compose generation for env-based list items (Drupal, WordPress, Ghost, Matomo and others). |
| 1.5.2 | 2026-06-12 | Agent-side container backups and full-archive restores for agent environments. |
| 1.5.1 | 2026-06-10 | Version reporting for the local instance and every connected agent. |
| **1.5.0** | 2026-06-07 | Reusable external storage locations (FTP, FTPS, SFTP, Samba, S3, Google Drive, OneDrive); encrypted container backups with plans, schedules and run history. |
| 1.4.1 | 2026-06-04 | Editable Linux device mappings; multi-file and folder upload in the file browser. |
| **1.4.0** | 2026-06-03 | Agent-side Docker Compose execution, so managed stacks and App Store installs run on remote agent environments. |
| 1.3.7 | 2026-06-02 | Token-safe diagnostics in the agent direct-transfer test. |
| 1.3.6 | 2026-06-02 | Agent-to-agent direct-transfer connectivity test. |
| 1.3.5 | 2026-06-01 | Optional direct agent-to-agent snapshot image transfer for moves between remote environments. |
| 1.3.4 | 2026-06-01 | Editable target volume mounts on a container move; agent install commands pinned to the current version. |
| 1.3.3 | 2026-06-01 | Target-agent floor raised to 1.3.3 for image-data moves. |
| 1.3.2 | 2026-05-31 | Longer operation window so remote image and volume moves finish on slower hosts. |
| 1.3.1 | 2026-05-31 | Container moves between Docker environments, with a preview dialog and editable target network settings. |
| 1.2.4 | 2026-05-30 | Frontend and backend dependency sweep. |
| 1.2.3 | 2026-05-30 | Container renaming; workflow nodes for Docker event, health and registry-tag triggers. |
| 1.2.2 | 2026-05-29 | Protected-resource detection for the running McHarbor container. |
| 1.2.1 | 2026-05-27 | System menu with overview metrics, services, processes and dependencies. |
| **1.2.0** | 2026-05-24 | Environments card overview with a persistent table/card switch. |
| 1.1.1 to 1.1.15 | 2026-05-23 to 05-24 | The self-update series: detection, restart, watchdog and cgroup v2 handling for McHarbor updating its own container, plus a confirmation dialog for **Reinstall All**. |
| **1.1.0** | 2026-05-23 | Generic OIDC authentication. |

## A note on the source file

The root `CHANGELOG.md` grew by appending, and the section below the 1.1.0 entry repeats
several releases: `2.0.0`, `2.0.2`, `1.6.5` and `1.6.6` each appear more than once, and an
`[Unreleased]` block sits among them. The first occurrence of each version, reading from
the top, is the current one; that is what this page is built from. The duplicates are
historical noise, not a second set of changes.

## Related

- [Upgrading](../upgrading/README.md): the mechanics, plugins across an upgrade, rollback, factory reset
- [Settings → Updates → McHarbor](../../settings/updates/mcharbor/README.md): the release checker and **Update now**
- [Settings → Updates → Agents](../../settings/updates/agents/README.md): pushing the matching agent binary
- [Settings → System → About](../../settings/system/about/README.md): the running version
- [`CHANGELOG.md`](../../../CHANGELOG.md): the authoritative, full-detail release notes
- [Documentation changelog](../../CHANGELOG.md): what changed in these pages
