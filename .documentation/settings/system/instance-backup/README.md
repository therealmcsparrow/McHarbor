# Instance backup

**Settings → System → Instance backup** · `?tab=instance-backup`

One encrypted file with everything that makes this instance itself, and the way back from
losing the host it runs on.

> This is **not** a container backup. Container data is backed up under
> [Backup](../../../backup/README.md). An operator needs both: the workloads survive a lost host
> through container backups, the governance survives it only through an instance export.

## What an export holds

| Part | Contents |
|---|---|
| Database | Every table except the ones meaningless elsewhere (below): roles, scopes, ABAC policies, users, environments and their agent tokens, workflows, schedules, alert rules, the secrets vault, catalogs, dashboards, table views, identity providers, plugin settings and plugin data tables |
| Key material | The data-encryption key the encrypted columns depend on, the audit signing secret, and McHarbor's certificate authority (root and key). The serving certificate only when it was uploaded; a generated one is re-issued from the restored CA |
| Plugins | Slug, version and enabled flag of each installed plugin, **not** the binaries, which are bound to the build that produced them. A restore reinstalls each from the catalog |
| Files | `stacks/`, `custom-nodes/`, `widgets/`, `widgets-state.json`, `workflow-nodes/`, `files/` and the restore ledger |

Never exported: sessions (a restored session is a restored login), step-up windows, login
attempts, MFA and SSO in-flight state, password-reset tokens, device sign-in codes, agent
install tokens and `schema_migrations`. Metrics history (`host_metrics`, `container_metrics`,
their roll-ups, container events, agent logs and similar) is left out unless **Include metrics
history** is switched on: it regenerates, and it is usually most of an export's size.

Container data, Docker itself, the host, and the remote agents' binaries are out of scope. A
restore brings back the agent **records** and tokens; the agents reconnect on their own because
the CA they pin came back with them.

## Permissions

| Key | What it unlocks |
|---|---|
| `instance_backup.view` | The export history, schedules, the last restore's status and the restore ledger |
| `instance_backup.manage` | Creating, downloading, verifying and deleting exports; managing schedules |
| `instance_backup.restore` | Uploading an export, checking it, and restoring it, without touching audit history |
| `instance_backup.restore_audit` | Restoring audit history, and only through an **approved elevation** at the time of the restore |

Neither restore key is in any role preset. Downloading an export also needs a step-up
re-authentication (password or authenticator code), because the file is every secret in the
instance at once.

## In this folder

- [Export](./export.md): creating an export, the key, the file format, downloading.
- [Restore](./restore.md): the three modes, the plan, audit history and its anchor, restoring
  a dead instance from the command line, and what happens when a restore is interrupted.
- [Schedules and verification](./schedules-and-verification.md): unattended exports, storage
  destinations, retention, restore rehearsals, the widget and the alerts.

## Related

- [System](../README.md)
- [CLI](../../../cli/README.md): `mcharbor restore --local`, `mcharbor audit verify`
