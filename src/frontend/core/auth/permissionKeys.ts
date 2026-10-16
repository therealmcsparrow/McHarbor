// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * Frontend mirror of the canonical permission keys defined in
 * src/backend/core/rbac/permissions.go. Use these constants in
 * JSX / hooks instead of bare string literals so:
 *
 *   1. Renames or additions in one place propagate everywhere.
 *   2. The IDE catches typos at the call site.
 *   3. `usePermission(PERM_USERS_MANAGE)` is greppable from the
 *      backend constant in seconds.
 *
 * Keep this list in lockstep with AllPermissions in
 * permissions.go. If you add a new key there, add the same key
 * here in the same commit. There is no codegen yet — the file is
 * short enough to maintain by hand.
 */

export const PERM = {
  // Containers
  CONTAINERS_VIEW: 'containers.view',
  CONTAINERS_MANAGE: 'containers.manage',
  CONTAINERS_DELETE: 'containers.delete',
  CONTAINERS_DEBUG_SHELL: 'containers.debug_shell',

  // Images
  IMAGES_VIEW: 'images.view',
  IMAGES_MANAGE: 'images.manage',
  IMAGES_DELETE: 'images.delete',

  // Volumes
  VOLUMES_VIEW: 'volumes.view',
  VOLUMES_MANAGE: 'volumes.manage',
  VOLUMES_DELETE: 'volumes.delete',

  // Networks
  NETWORKS_VIEW: 'networks.view',
  NETWORKS_MANAGE: 'networks.manage',
  NETWORKS_DELETE: 'networks.delete',

  // Stacks
  STACKS_VIEW: 'stacks.view',
  STACKS_MANAGE: 'stacks.manage',
  STACKS_DELETE: 'stacks.delete',

  // Secrets vault. SECRETS_REVEAL is in no role preset by design — it is
  // obtained through an approved, time-bounded elevation, so a control gated
  // on it renders disabled-with-a-tooltip for almost every user, beside a
  // "request access" affordance.
  SECRETS_VIEW: 'secrets.view',
  SECRETS_MANAGE: 'secrets.manage',
  SECRETS_MANAGE_GLOBAL: 'secrets.manage_global',
  SECRETS_DELETE: 'secrets.delete',
  SECRETS_REVEAL: 'secrets.reveal',
  SECRETS_APPLY: 'secrets.apply',

  // Environments
  ENVIRONMENTS_VIEW: 'environments.view',
  ENVIRONMENTS_MANAGE: 'environments.manage',

  // Users
  USERS_VIEW: 'users.view',
  USERS_MANAGE: 'users.manage',

  // Settings
  SETTINGS_VIEW: 'settings.view',
  SETTINGS_MANAGE: 'settings.manage',

  // Communications
  COMMUNICATIONS_VIEW: 'communications.view',
  COMMUNICATIONS_MANAGE: 'communications.manage',

  // Store
  STORE_APPS_VIEW: 'store_apps.view',
  STORE_APPS_MANAGE: 'store_apps.manage',
  STORE_NODES_VIEW: 'store_nodes.view',
  STORE_NODES_MANAGE: 'store_nodes.manage',
  STORE_WIDGETS_VIEW: 'store_widgets.view',
  STORE_WIDGETS_MANAGE: 'store_widgets.manage',

  // Terminal / Logs
  TERMINAL_ACCESS: 'terminal.access',
  LOGS_VIEW: 'logs.view',
  HOST_UPDATES_VIEW: 'host_updates.view',
  HOST_UPDATES_MANAGE: 'host_updates.manage',
  HOST_UPDATES_REBOOT: 'host_updates.reboot',
  HOST_SERVICES_VIEW: 'host_services.view',
  HOST_SERVICES_MANAGE: 'host_services.manage',
  HOST_SELF_VIEW: 'host_self.view',
  HOST_SELF_MANAGE: 'host_self.manage',
  HOST_SELF_REBOOT: 'host_self.reboot',

  // API keys
  API_KEYS_VIEW: 'api_keys.view',
  API_KEYS_MANAGE: 'api_keys.manage',

  // Groups / Roles / Permissions
  GROUPS_VIEW: 'groups.view',
  GROUPS_MANAGE: 'groups.manage',
  ROLES_VIEW: 'roles.view',
  ROLES_MANAGE: 'roles.manage',
  PERMISSIONS_VIEW: 'permissions.view',
  PERMISSIONS_ASSIGN: 'permissions.assign',

  // Scans
  SCANS_VIEW: 'scans.view',
  SCANS_MANAGE: 'scans.manage',

  // Host
  HOST_VIEW: 'host.view',
  HOST_MANAGE: 'host.manage',

  // System
  SYSTEM_MANAGE: 'system.manage',
  SYSTEM_DEMO: 'system.demo',
  METRICS_SCRAPE: 'metrics.scrape',
  // Database backend selection and the cross-backend data migration.
  // Deliberately separate from SYSTEM_MANAGE: restarting the container and
  // repointing the database are different grants.
  DATABASE_VIEW: 'database.view',
  DATABASE_MANAGE: 'database.manage',

  // Account lifecycle
  ACCOUNT_SELF_MANAGE: 'account.self.manage',
  ACCOUNT_MANAGE: 'account.manage',
  ACCOUNT_PASSWORD_RESET: 'account.password.reset',

  // Sessions / Audit
  SESSIONS_VIEW: 'sessions.view',
  SESSIONS_MANAGE: 'sessions.manage',
  AUDIT_VIEW: 'audit.view',
  AUDIT_EXPORT: 'audit.export',
  INSTANCE_BACKUP_VIEW: 'instance_backup.view',
  INSTANCE_BACKUP_MANAGE: 'instance_backup.manage',
  INSTANCE_BACKUP_RESTORE: 'instance_backup.restore',
  INSTANCE_BACKUP_RESTORE_AUDIT: 'instance_backup.restore_audit',

  // Notifications
  IN_APP_NOTIFICATIONS_VIEW: 'in_app_notifications.view',

  // Workflows
  WORKFLOWS_VIEW: 'workflows.view',
  WORKFLOWS_MANAGE: 'workflows.manage',
  WORKFLOWS_DELETE: 'workflows.delete',
  WORKFLOWS_RUN: 'workflows.run',
  CUSTOM_NODES_VIEW: 'custom_nodes.view',
  CUSTOM_NODES_MANAGE: 'custom_nodes.manage',
  SCHEDULES_VIEW: 'schedules.view',
  SCHEDULES_MANAGE: 'schedules.manage',
  SCHEDULES_DELETE: 'schedules.delete',

  // Webhooks / Alerts / Autoheal
  WEBHOOKS_VIEW: 'webhooks.view',
  WEBHOOKS_MANAGE: 'webhooks.manage',
  WEBHOOKS_DELETE: 'webhooks.delete',
  ALERTS_VIEW: 'alerts.view',
  ALERTS_MANAGE: 'alerts.manage',
  ALERTS_DELETE: 'alerts.delete',
  AUTOHEAL_VIEW: 'autoheal.view',
  AUTOHEAL_MANAGE: 'autoheal.manage',

  // Backups
  BACKUPS_VIEW: 'backups.view',
  BACKUPS_MANAGE: 'backups.manage',
  BACKUPS_DELETE: 'backups.delete',
  BACKUPS_DOWNLOAD: 'backups.download',
  RESTORE_VIEW: 'restore.view',
  RESTORE_RUN: 'restore.run',
  BACKUP_LOG_VIEW: 'backup_log.view',

  // Git / GitOps
  GIT_VIEW: 'git.view',
  GIT_MANAGE: 'git.manage',
  GITOPS_VIEW: 'gitops.view',
  GITOPS_MANAGE: 'gitops.manage',
  GITOPS_DELETE: 'gitops.delete',
  GITOPS_APPROVE: 'gitops.approve',

  // Appstore / Registry / Plugins

  REGISTRY_VIEW: 'registry.view',
  REGISTRY_MANAGE: 'registry.manage',
  PLUGINS_VIEW: 'plugins.view',
  PLUGINS_MANAGE: 'plugins.manage',
  PLUGINS_DELETE: 'plugins.delete',

  // Storage locations
  STORAGE_LOCATIONS_VIEW: 'storage_locations.view',
  STORAGE_LOCATIONS_MANAGE: 'storage_locations.manage',
  STORAGE_LOCATIONS_DELETE: 'storage_locations.delete',

  // Reconciler / Updates
  RECONCILER_VIEW: 'reconciler.view',
  RECONCILER_MANAGE: 'reconciler.manage',
  RECONCILER_ENFORCE: 'reconciler.enforce',
  INCIDENTS_VIEW: 'incidents.view',
  INCIDENTS_MANAGE: 'incidents.manage',
  CLUSTER_VIEW: 'cluster.view',
  CLUSTER_MANAGE: 'cluster.manage',
  COMPLIANCE_VIEW: 'compliance.view',
  COMPLIANCE_MANAGE: 'compliance.manage',
  COMPLIANCE_ATTEST: 'compliance.attest',
  SUPPORT_VIEW: 'support.view',
  SUPPORT_MANAGE: 'support.manage',
  UPDATES_VIEW: 'updates.view',
  UPDATES_MANAGE: 'updates.manage',

  // MFA / Identity providers

  IDENTITY_PROVIDERS_VIEW: 'identity_providers.view',
  IDENTITY_PROVIDERS_MANAGE: 'identity_providers.manage',

  // AI assistant
  AI_ASSISTANT_VIEW: 'ai_assistant.view',
  AI_ASSISTANT_USE: 'ai_assistant.use',
  AI_ASSISTANT_ACT: 'ai_assistant.act',

  AI_ASSISTANT_MANAGE: 'ai_assistant.manage',

  // Observability
  EVENTS_VIEW: 'events.view',
  ACTIVITY_VIEW: 'activity.view',
  METRICS_VIEW: 'metrics.view',
  DASHBOARD_VIEW: 'dashboard.view',
  WIDGETS_VIEW: 'widgets.view',
  WIDGETS_MANAGE: 'widgets.manage',
  DASHBOARD_WIDGETS_INSTALL: 'dashboard_widgets.install',

  // RBAC v3 / SCIM
  RBAC_POLICIES_VIEW: 'rbac.policies.view',
  RBAC_POLICIES_MANAGE: 'rbac.policies.manage',
  SCIM_VIEW: 'scim.view',
  SCIM_MANAGE: 'scim.manage',

  // License
  LICENSE_VIEW: 'license.view',
  LICENSE_MANAGE: 'license.manage',

  APPROVALS_VIEW: 'approvals.view',
  APPROVALS_MANAGE: 'approvals.manage',
  APPROVALS_DECIDE: 'approvals.decide',

  // Runbooks
  RUNBOOKS_VIEW: 'runbooks.view',
  RUNBOOKS_MANAGE: 'runbooks.manage',
  RUNBOOKS_RUN: 'runbooks.run',
  RUNBOOKS_AUTOMATE: 'runbooks.automate',
} as const;

export type PermissionKey = (typeof PERM)[keyof typeof PERM];

/**
 * ALL_PERMISSION_KEYS — the canonical list, kept in sync with
 * AllPermissions in src/backend/core/rbac/permissions.go. Use
 * this when iterating every key (e.g. role-editor matrix).
 */
export const ALL_PERMISSION_KEYS: readonly PermissionKey[] = Object.values(PERM);
