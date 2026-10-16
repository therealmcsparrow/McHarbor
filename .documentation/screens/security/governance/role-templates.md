# Role templates

**Security → Management → Roles → Create role → Start from a preset**

Six templates for common teams, added by migration `093_governance.sql` beside the four that
already existed (Backup Manager, Read-Only Auditor, Container Operator, Notification
Recipient). Applying a preset fills the role's permission set; you can edit it before saving.
None of them contains `*`.

Every template includes `account.self.manage`, `dashboard.view`, `widgets.view` and
`in_app_notifications.view`, so the holder can use their own account and the dashboard.

| Template | Intent | Notable grants | Notably withheld |
|---|---|---|---|
| **Viewer** | Read workloads, logs and dashboards | every workload `*.view`, `logs.view`, `events.view`, `metrics.view`, `cluster.view` | secrets, settings, identity, every mutation |
| **Operator** | Run day-to-day workloads | Viewer + `containers/stacks/images/volumes/networks.manage`, `terminal.access`, backups and restore, `updates.manage`, `incidents.manage`, `workflows.run`, `secrets.apply` | every `*.delete`, settings, identity |
| **Developer** | Build and ship | Viewer + workload `*.manage`, `git.*`, `gitops.view/manage`, `workflows.manage/run`, `custom_nodes.*`, `secrets.view/manage`, `api_keys.manage` (own keys), `ai_assistant.view/use` | deletes, settings, identity, `secrets.reveal` |
| **Auditor** | Review governance | `audit.view`, `audit.export`, `approvals.view`, users, groups, roles, `permissions.view`, `rbac.policies.view`, `sessions.view`, `api_keys.view`, identity and SCIM views, `settings.view` | every mutation, secret values |
| **Security Admin** | Run identity and access | users, groups, roles, `permissions.assign`, `rbac.policies.manage`, identity providers, SCIM, sessions, API keys, `approvals.view/manage/decide`, scanning, audit | workload control |
| **Platform Admin** | Run the platform and its workloads | every catalog key except the governance set | `users.manage`, `account.manage`, `account.password.reset`, `groups.manage`, `roles.manage`, `permissions.assign`, `rbac.policies.manage`, `scim.manage`, `identity_providers.manage`, `sessions.manage`, `approvals.manage`, `approvals.decide`, `secrets.reveal`, `audit.export`, `instance_backup.restore_audit`, `license.manage` |

Platform Admin and Security Admin are meant to be held by different people: one runs the
install, the other decides who may, and approves the other's destructive actions.

The exact key lists are in the migration; `GET /api/roles/presets` returns them as stored.
Keys added to the catalog later are not added to these templates.
