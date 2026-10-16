// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

// Permission represents a granular access control permission.
type Permission string

// PermWildcard grants access to everything.
const PermWildcard Permission = "*"

// --- Containers ---
const (
	PermContainersView   Permission = "containers.view"
	PermContainersManage Permission = "containers.manage"
	PermContainersDelete Permission = "containers.delete"
	// PermContainersDebugShell places a static busybox inside a running container
	// so a distroless image can be debugged; strictly stronger than manage.
	PermContainersDebugShell Permission = "containers.debug_shell"
)

// --- Images ---
const (
	PermImagesView   Permission = "images.view"
	PermImagesManage Permission = "images.manage"
	PermImagesDelete Permission = "images.delete"
)

// --- Volumes ---
const (
	PermVolumesView   Permission = "volumes.view"
	PermVolumesManage Permission = "volumes.manage"
	PermVolumesDelete Permission = "volumes.delete"
)

// --- Networks ---
const (
	PermNetworksView   Permission = "networks.view"
	PermNetworksManage Permission = "networks.manage"
	PermNetworksDelete Permission = "networks.delete"
)

// --- Stacks ---
const (
	PermStacksView   Permission = "stacks.view"
	PermStacksManage Permission = "stacks.manage"
	PermStacksDelete Permission = "stacks.delete"
)

// --- Secrets ---
//
// Six keys rather than the usual three. `reveal` gates something
// categorically different from reading metadata — folding it into `view`
// would turn the list page into a credential dump — and `manage_global` is
// the only way to express "may manage secrets here, but not everywhere",
// given that scope lives on the row rather than on the permission.
//
// `apply` is separate from `manage` because propagating a rotation restarts
// containers and redeploys stacks. Rotating a value and taking someone's
// production stack down for thirty seconds are different authorities, and one
// key covering both would grant the second silently.
const (
	PermSecretsView         Permission = "secrets.view"
	PermSecretsManage       Permission = "secrets.manage"
	PermSecretsManageGlobal Permission = "secrets.manage_global"
	PermSecretsDelete       Permission = "secrets.delete"
	PermSecretsReveal       Permission = "secrets.reveal"
	PermSecretsApply        Permission = "secrets.apply"
)

// --- Environments ---
const (
	PermEnvironmentsView   Permission = "environments.view"
	PermEnvironmentsManage Permission = "environments.manage"
)

// --- Users ---
const (
	PermUsersView   Permission = "users.view"
	PermUsersManage Permission = "users.manage"
)

// --- Settings ---
const (
	PermSettingsView   Permission = "settings.view"
	PermSettingsManage Permission = "settings.manage"
)

// --- Communications ---
const (
	PermCommunicationsView   Permission = "communications.view"
	PermCommunicationsManage Permission = "communications.manage"
)

// --- Store: Apps ---
const (
	PermStoreAppsView   Permission = "store_apps.view"
	PermStoreAppsManage Permission = "store_apps.manage"
)

// --- Store: Nodes ---
const (
	PermStoreNodesView   Permission = "store_nodes.view"
	PermStoreNodesManage Permission = "store_nodes.manage"
)

// --- Store: Widgets ---
const (
	PermStoreWidgetsView   Permission = "store_widgets.view"
	PermStoreWidgetsManage Permission = "store_widgets.manage"
)

// PermTerminalAccess gates opening an interactive terminal session in a container.
const PermTerminalAccess Permission = "terminal.access"

// PermLogsView gates reading container and system logs.
const PermLogsView Permission = "logs.view"

// --- Host OS updates ---
//
// Separate from settings.* because this governs one environment's host, not the
// McHarbor installation: a role that may patch a lab box should not thereby
// gain the global settings surface.
const (
	PermHostUpdatesView   Permission = "host_updates.view"
	PermHostUpdatesManage Permission = "host_updates.manage"
	// PermHostUpdatesReboot schedules a reboot of the host, which stops every
	// container and guest on it; apart from manage because patching does not.
	PermHostUpdatesReboot Permission = "host_updates.reboot"
)

// --- Host systemd services ---
//
// Separate from host_updates.* because stopping or reconfiguring a unit can
// take a host off the network, which patching packages on a schedule cannot.
const (
	// PermHostServicesView lists a native agent host's systemd services and
	// reads their unit files and drop-ins.
	PermHostServicesView Permission = "host_services.view"
	// PermHostServicesManage starts, stops, restarts, enables and disables
	// units, writes and reverts drop-in overrides, and runs daemon-reload.
	PermHostServicesManage Permission = "host_services.manage"
)

// --- API Keys ---
const (
	PermAPIKeysView   Permission = "api_keys.view"
	PermAPIKeysManage Permission = "api_keys.manage"
)

// --- Groups ---
const (
	PermGroupsView   Permission = "groups.view"
	PermGroupsManage Permission = "groups.manage"
)

// --- Roles ---
const (
	PermRolesView   Permission = "roles.view"
	PermRolesManage Permission = "roles.manage"
)

// --- Scans ---
const (
	PermScansView   Permission = "scans.view"
	PermScansManage Permission = "scans.manage"
)

// --- Host ---
const (
	PermHostView   Permission = "host.view"
	PermHostManage Permission = "host.manage"
)

// PermSystemManage covers self-management actions on the McHarbor
// application itself (restart, stop, future install hooks). The
// Admin system role holds it via the wildcard; custom roles must
// be granted it explicitly.
const PermSystemManage Permission = "system.manage"

// PermSystemDemo switches the instance into and out of demo mode. It is apart
// from PermSystemManage because entering demo restarts onto a different database.
const PermSystemDemo Permission = "system.demo"

// PermMetricsScrape gates the Prometheus exposition at /api/metrics. It is its
// own key so a scrape-only API key can read fleet shape without host.view.
const PermMetricsScrape Permission = "metrics.scrape"

// --- Database ---
// PermDatabaseView reads which SQL backend McHarbor is running on and the
// state of any migration between backends. PermDatabaseManage changes that
// backend and runs the data migration, which touches every row in the
// product — it is deliberately a key of its own rather than a fold into
// system.manage, so "may restart the container" and "may repoint the
// database" stay separate grants.
const (
	PermDatabaseView   Permission = "database.view"
	PermDatabaseManage Permission = "database.manage"
)

// --- Account (self / admin lifecycle) ---
// PermAccountSelfManage lets a user update their own profile, email,
// password, and MFA — the common authenticated-self surface.
// PermAccountManage is the admin-level counterpart that covers
// creating, disabling, enabling, and deleting other users' accounts.
const (
	PermAccountSelfManage    Permission = "account.self.manage"
	PermAccountManage        Permission = "account.manage"
	PermAccountPasswordReset Permission = "account.password.reset"
)

// --- Permissions meta ---
const (
	PermPermissionsView   Permission = "permissions.view"
	PermPermissionsAssign Permission = "permissions.assign"
)

// --- Sessions ---
const (
	PermSessionsView   Permission = "sessions.view"
	PermSessionsManage Permission = "sessions.manage"
)

// --- Audit ---
const (
	PermAuditView   Permission = "audit.view"
	PermAuditExport Permission = "audit.export"
)

// --- Instance backup ---
//
// An instance export is a copy of every secret in the system, so it is gated
// apart from settings.manage, and restoring audit history apart from any other restore.
const (
	PermInstanceBackupView         Permission = "instance_backup.view"
	PermInstanceBackupManage       Permission = "instance_backup.manage"
	PermInstanceBackupRestore      Permission = "instance_backup.restore"
	PermInstanceBackupRestoreAudit Permission = "instance_backup.restore_audit"
)

// --- Notifications ---
const (
	PermInAppNotificationsView Permission = "in_app_notifications.view"
)

// --- Workflows ---
const (
	PermWorkflowsView     Permission = "workflows.view"
	PermWorkflowsManage   Permission = "workflows.manage"
	PermWorkflowsDelete   Permission = "workflows.delete"
	PermWorkflowsRun      Permission = "workflows.run"
	PermCustomNodesView   Permission = "custom_nodes.view"
	PermCustomNodesManage Permission = "custom_nodes.manage"
	PermSchedulesView     Permission = "schedules.view"
	PermSchedulesManage   Permission = "schedules.manage"
	PermSchedulesDelete   Permission = "schedules.delete"
)

// --- Webhooks / alerts / autoheal ---
const (
	PermWebhooksView   Permission = "webhooks.view"
	PermWebhooksManage Permission = "webhooks.manage"
	PermWebhooksDelete Permission = "webhooks.delete"
	PermAlertsView     Permission = "alerts.view"
	PermAlertsManage   Permission = "alerts.manage"
	PermAlertsDelete   Permission = "alerts.delete"
	PermAutohealView   Permission = "autoheal.view"
	PermAutohealManage Permission = "autoheal.manage"
)

// --- Backups ---
const (
	PermBackupsView   Permission = "backups.view"
	PermBackupsManage Permission = "backups.manage"
	PermBackupsDelete Permission = "backups.delete"
	// Downloading hands the caller a complete copy of the container's data,
	// which view / manage / delete do not cover — hence its own key.
	PermBackupsDownload Permission = "backups.download"
	// Browsing an archive lists every file path it holds, which is worth
	// granting separately from the power to write those files back.
	PermRestoreView   Permission = "restore.view"
	PermRestoreRun    Permission = "restore.run"
	PermBackupLogView Permission = "backup_log.view"
)

// --- Git / GitOps ---
const (
	PermGitView       Permission = "git.view"
	PermGitManage     Permission = "git.manage"
	PermGitopsView    Permission = "gitops.view"
	PermGitopsManage  Permission = "gitops.manage"
	PermGitopsDelete  Permission = "gitops.delete"
	PermGitopsApprove Permission = "gitops.approve"
)

// --- Appstore / registry / plugins ---
const (
	PermRegistryView   Permission = "registry.view"
	PermRegistryManage Permission = "registry.manage"
	PermPluginsView    Permission = "plugins.view"
	PermPluginsManage  Permission = "plugins.manage"
	PermPluginsDelete  Permission = "plugins.delete"
)

// --- Storage locations ---
const (
	PermStorageLocationsView   Permission = "storage_locations.view"
	PermStorageLocationsManage Permission = "storage_locations.manage"
	PermStorageLocationsDelete Permission = "storage_locations.delete"
)

// --- Reconciler / updates ---
const (
	PermReconcilerView   Permission = "reconciler.view"
	PermReconcilerManage Permission = "reconciler.manage"
	// PermReconcilerEnforce lets McHarbor change containers without a person
	// approving each change, so it is granted apart from managing intent.
	PermReconcilerEnforce Permission = "reconciler.enforce"
	// PermIncidentsView shows incidents, their evidence and any narrative.
	PermIncidentsView Permission = "incidents.view"
	// PermIncidentsManage asks a model to narrate, changes an environment's
	// narration opt-in, and resolves or dismisses; it can send evidence off-host.
	PermIncidentsManage Permission = "incidents.manage"
	// PermClusterView shows the HA topology, node roles, readiness and the migration plan.
	PermClusterView Permission = "cluster.view"
	// PermClusterManage fails over to another node and drains or resumes nodes, which
	// moves every user's traffic, so it is separate from settings.manage.
	PermClusterManage Permission = "cluster.manage"
	PermUpdatesView   Permission = "updates.view"
	PermUpdatesManage Permission = "updates.manage"
)

// --- Compliance ---
const (
	// PermComplianceView reads the compliance posture and downloads the report; it is
	// separate from settings.view so an auditor can hold it without seeing every setting.
	PermComplianceView Permission = "compliance.view"
)

// --- Support ---
const (
	// PermSupportView runs diagnostics, reads the state inventory and previews what a
	// support bundle would contain, all read-only.
	PermSupportView Permission = "support.view"
	// PermSupportManage generates, downloads and deletes support bundles, which carry
	// redacted configuration and logs off the host.
	PermSupportManage Permission = "support.manage"
)

// --- MFA / identity ---
const (
	PermIdentityProvidersView   Permission = "identity_providers.view"
	PermIdentityProvidersManage Permission = "identity_providers.manage"
)

// --- AI assistant ---
const (
	PermAIAssistantView Permission = "ai_assistant.view"
	PermAIAssistantUse  Permission = "ai_assistant.use"
	PermAIAssistantAct  Permission = "ai_assistant.act"

	PermAIAssistantManage Permission = "ai_assistant.manage"
)

// --- Observability / dashboards ---
const (
	PermEventsView    Permission = "events.view"
	PermActivityView  Permission = "activity.view"
	PermMetricsView   Permission = "metrics.view"
	PermDashboardView Permission = "dashboard.view"
	PermWidgetsView   Permission = "widgets.view"
	PermWidgetsManage Permission = "widgets.manage"
)

// --- License ---
//
// Instance-wide by nature: there is one licence per install, and the
// payload behind license.view carries the customer's billing detail, so
// it is not part of the viewer / editor / operator presets. role_admin is
// ["*"] and picks both keys up automatically.
const (
	PermLicenseView   Permission = "license.view"
	PermLicenseManage Permission = "license.manage"
)

// --- RBAC / SCIM (v3) ---
const (
	PermRbacPoliciesView   Permission = "rbac.policies.view"
	PermRbacPoliciesManage Permission = "rbac.policies.manage"
	PermScimView           Permission = "scim.view"
	PermScimManage         Permission = "scim.manage"
)

// --- Approvals ---
//
// Three keys because reading the queue, changing which actions need a second
// person, and being that second person are separate authorities: an auditor
// reads, a security admin writes rules, and approvers decide.
const (
	PermApprovalsView   Permission = "approvals.view"
	PermApprovalsManage Permission = "approvals.manage"
	PermApprovalsDecide Permission = "approvals.decide"
)

// AllPermissions is every permission key, for the role editor and the contract
// test that asserts the catalog and this list agree.
//
// It is a function rather than a var so a key registered through
// RegisterPermissions is included without editing this list. The historical
// entries stay spelled out below.
func AllPermissions() []Permission {
	out := make([]Permission, 0, len(allPermissionsBase)+8)
	out = append(out, allPermissionsBase...)
	for _, e := range registeredPermissions() {
		out = append(out, e.Key)
	}
	return out
}

var allPermissionsBase = []Permission{
	PermContainersView, PermContainersManage, PermContainersDelete, PermContainersDebugShell,
	PermImagesView, PermImagesManage, PermImagesDelete,
	PermVolumesView, PermVolumesManage, PermVolumesDelete,
	PermNetworksView, PermNetworksManage, PermNetworksDelete,
	PermStacksView, PermStacksManage, PermStacksDelete,
	PermSecretsView, PermSecretsManage, PermSecretsManageGlobal,
	PermSecretsDelete, PermSecretsReveal, PermSecretsApply,
	PermEnvironmentsView, PermEnvironmentsManage,
	PermUsersView, PermUsersManage,
	PermSettingsView, PermSettingsManage,
	PermCommunicationsView, PermCommunicationsManage,
	PermStoreAppsView, PermStoreAppsManage,
	PermStoreNodesView, PermStoreNodesManage,
	PermStoreWidgetsView, PermStoreWidgetsManage,
	PermTerminalAccess,
	PermLogsView,
	PermHostUpdatesView,
	PermHostUpdatesManage,
	PermHostUpdatesReboot,
	PermHostServicesView,
	PermHostServicesManage,
	PermAPIKeysView, PermAPIKeysManage,
	PermGroupsView, PermGroupsManage,
	PermRolesView, PermRolesManage,
	PermScansView, PermScansManage,
	PermHostView, PermHostManage,
	PermSystemManage,
	PermSystemDemo,
	PermMetricsScrape,
	PermDatabaseView, PermDatabaseManage,
	PermAccountSelfManage, PermAccountManage,
	PermAccountPasswordReset,
	PermPermissionsView, PermPermissionsAssign,
	PermSessionsView, PermSessionsManage,
	PermAuditView, PermAuditExport,
	PermInstanceBackupView, PermInstanceBackupManage,
	PermInstanceBackupRestore, PermInstanceBackupRestoreAudit,
	PermInAppNotificationsView,
	PermWorkflowsView, PermWorkflowsManage, PermWorkflowsDelete, PermWorkflowsRun,
	PermCustomNodesView, PermCustomNodesManage,
	PermSchedulesView, PermSchedulesManage, PermSchedulesDelete,
	PermWebhooksView, PermWebhooksManage, PermWebhooksDelete,
	PermAlertsView, PermAlertsManage, PermAlertsDelete,
	PermAutohealView, PermAutohealManage,
	PermBackupsView, PermBackupsManage, PermBackupsDelete, PermBackupsDownload,
	PermRestoreView, PermRestoreRun,
	PermBackupLogView,
	PermGitView, PermGitManage,
	PermGitopsView, PermGitopsManage, PermGitopsDelete, PermGitopsApprove,

	PermRegistryView, PermRegistryManage,
	PermPluginsView, PermPluginsManage, PermPluginsDelete,
	PermStorageLocationsView, PermStorageLocationsManage, PermStorageLocationsDelete,
	PermReconcilerView, PermReconcilerManage, PermReconcilerEnforce,
	PermIncidentsView, PermIncidentsManage,
	PermClusterView, PermClusterManage, PermComplianceView,
	PermSupportView, PermSupportManage,
	PermUpdatesView, PermUpdatesManage,

	PermIdentityProvidersView, PermIdentityProvidersManage,
	PermAIAssistantView, PermAIAssistantUse, PermAIAssistantAct, PermAIAssistantManage,
	PermEventsView, PermActivityView, PermMetricsView,
	PermDashboardView, PermWidgetsView, PermWidgetsManage,
	PermRbacPoliciesView, PermRbacPoliciesManage,
	PermScimView, PermScimManage,
	PermLicenseView, PermLicenseManage,
	PermApprovalsView, PermApprovalsManage, PermApprovalsDecide,
}
