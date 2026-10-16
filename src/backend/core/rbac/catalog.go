// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"database/sql"
	"fmt"
)

// PermissionEntry describes one row in the permission_catalog table.
// Server is the source of truth; the constants in permissions.go are
// the fast-path symbols referenced in code, but every entry here is
// mirrored into the database by CatalogSeed.
type PermissionEntry struct {
	Key         Permission
	Category    string
	Action      string
	Description string
	Scope       string // "global" | "environment" | "stack" | "owner" | "account"
}

// CatalogSeed returns the canonical list of all known permissions.
// This is called once at startup by the rbac module's seed function
// and is used by the UI /auth/permissions/catalog endpoint.
//
// The list is grouped by category. Adding a permission requires:
//  1. Adding a const in permissions.go.
//  2. Adding an entry here.
//  3. Updating AllPermissions slice in permissions.go.
//
// A new area should use rbac.RegisterPermissions from its own file instead,
// which takes all three in one call so they cannot disagree — see
// catalog_extra.go for why that matters.
func CatalogSeed() []PermissionEntry {
	base := []PermissionEntry{
		// Containers
		{PermContainersView, "containers", "view", "View container details, logs, stats", "environment"},
		{PermContainersManage, "containers", "manage", "Start, stop, restart, exec into containers", "owner"},
		{PermContainersDelete, "containers", "delete", "Remove or kill containers", "environment"},
		{PermContainersDebugShell, "containers", "debug_shell", "Upload a temporary static busybox into a shell-less container and open a shell with it", "environment"},

		// Images
		{PermImagesView, "images", "view", "Inspect images and layers", "environment"},
		{PermImagesManage, "images", "manage", "Pull, push, tag, build images", "owner"},
		{PermImagesDelete, "images", "delete", "Remove images from registry cache", "environment"},

		// Volumes
		{PermVolumesView, "volumes", "view", "List and inspect volumes", "environment"},
		{PermVolumesManage, "volumes", "manage", "Create volumes and mounts", "owner"},
		{PermVolumesDelete, "volumes", "delete", "Remove volumes", "environment"},

		// Networks
		{PermNetworksView, "networks", "view", "List and inspect networks", "environment"},
		{PermNetworksManage, "networks", "manage", "Create, connect, disconnect networks", "owner"},
		{PermNetworksDelete, "networks", "delete", "Remove networks", "environment"},

		// Stacks
		{PermStacksView, "stacks", "view", "List and inspect Compose stacks", "environment"},
		{PermStacksManage, "stacks", "manage", "Create, edit, deploy stacks", "stack"},
		{PermStacksDelete, "stacks", "delete", "Remove stacks", "environment"},

		// Secrets
		{PermSecretsView, "secrets", "view", "List secret names and metadata (never values)", "environment"},
		{PermSecretsManage, "secrets", "manage", "Create and rotate environment and stack secrets", "environment"},
		{PermSecretsManageGlobal, "secrets", "manage_global", "Create and rotate secrets visible to every environment", "global"},
		{PermSecretsDelete, "secrets", "delete", "Remove secrets", "environment"},
		{PermSecretsReveal, "secrets", "reveal", "Read a stored secret value in plaintext", "environment"},
		{PermSecretsApply, "secrets", "apply", "Recreate containers and redeploy stacks so they pick up a rotated secret", "environment"},

		// Environments
		{PermEnvironmentsView, "environments", "view", "List and inspect environments", "global"},
		{PermEnvironmentsManage, "environments", "manage", "Add, edit, remove environments", "global"},

		// Users
		{PermUsersView, "users", "view", "List and inspect users", "global"},
		{PermUsersManage, "users", "manage", "Update user profiles and roles", "account"},

		// Settings
		{PermSettingsView, "settings", "view", "View system settings", "global"},
		{PermSettingsManage, "settings", "manage", "Update system settings", "global"},

		// Communications
		{PermCommunicationsView, "communications", "view", "View notification channels", "global"},
		{PermCommunicationsManage, "communications", "manage", "Manage notification channels", "global"},

		// Store: Apps
		{PermStoreAppsView, "store_apps", "view", "Browse the app catalog", "global"},
		{PermStoreAppsManage, "store_apps", "manage", "Install, upgrade, uninstall apps", "global"},

		// Store: Nodes
		{PermStoreNodesView, "store_nodes", "view", "Browse workflow node catalog", "global"},
		{PermStoreNodesManage, "store_nodes", "manage", "Install workflow nodes", "global"},

		// Store: Widgets
		{PermStoreWidgetsView, "store_widgets", "view", "Browse widget catalog", "global"},
		{PermStoreWidgetsManage, "store_widgets", "manage", "Install widgets", "global"},

		// Terminal
		{PermTerminalAccess, "terminal", "access", "Open interactive terminal sessions", "environment"},

		// Logs
		{PermLogsView, "logs", "view", "View container and workload logs", "environment"},

		// Host OS updates
		{PermHostUpdatesView, "host_updates", "view", "View host OS details and pending package updates", "environment"},
		{PermHostUpdatesManage, "host_updates", "manage", "Schedule host OS update checks and install updates", "environment"},
		{PermHostUpdatesReboot, "host_updates", "reboot", "Schedule or cancel a reboot of the host, which stops its containers and guests first", "environment"},

		// Host systemd services
		{PermHostServicesView, "host_services", "view", "View a native agent host's systemd services, unit files and overrides", "environment"},
		{PermHostServicesManage, "host_services", "manage", "Start, stop, restart, enable and disable host services, edit and revert their overrides, and reload systemd", "environment"},

		// API Keys
		{PermAPIKeysView, "api_keys", "view", "List API keys and their grants", "account"},
		{PermAPIKeysManage, "api_keys", "manage", "Create, edit, revoke API keys", "account"},

		// Groups
		{PermGroupsView, "groups", "view", "List groups and memberships", "global"},
		{PermGroupsManage, "groups", "manage", "Create, edit, remove groups", "global"},

		// Roles
		{PermRolesView, "roles", "view", "List roles and permissions", "global"},
		{PermRolesManage, "roles", "manage", "Create, edit, remove roles", "global"},

		// Scans
		{PermScansView, "scans", "view", "View vulnerability scan results", "environment"},
		{PermScansManage, "scans", "manage", "Trigger scans, configure scanners", "environment"},

		// Host
		{PermHostView, "host", "view", "View host metrics and Docker daemon", "environment"},
		{PermHostManage, "host", "manage", "Prune, restart daemon, manage host", "environment"},

		// System
		{PermSystemManage, "system", "manage", "Self-management actions on McHarbor", "global"},
		{PermSystemDemo, "system", "demo", "Enter, exit and reset demo mode", "global"},
		{PermMetricsScrape, "metrics", "scrape", "Scrape McHarbor's own Prometheus metrics", "global"},

		// Database
		{PermDatabaseView, "database", "view", "View the database backend and migration status", "global"},
		{PermDatabaseManage, "database", "manage", "Change the database backend and migrate the data", "global"},

		// ─── Foundational (meta) ───
		{PermPermissionsView, "permissions", "view", "Read the permission catalog and effective permissions", "global"},
		{PermPermissionsAssign, "permissions", "assign", "Grant or revoke user-level permission overrides", "global"},
		{PermSessionsView, "sessions", "view", "List active sessions", "account"},
		{PermSessionsManage, "sessions", "manage", "Revoke any user's sessions", "account"},
		{PermAccountSelfManage, "account", "self.manage", "Update own profile, email, password, MFA", "account"},
		{PermAccountManage, "account", "manage", "Create, disable, enable, delete accounts", "account"},
		{PermAccountPasswordReset, "account", "password.reset", "Issue admin-set password or email reset link", "account"},

		// ─── Audit ───
		{PermAuditView, "audit", "view", "Read the audit log", "global"},
		{PermAuditExport, "audit", "export", "Export audit entries as CSV/PDF", "global"},

		// ─── Instance backup ───
		{PermInstanceBackupView, "instance_backup", "view", "See instance export history, schedules and manifests", "global"},
		{PermInstanceBackupManage, "instance_backup", "manage", "Create, download and delete instance exports and their schedules", "global"},
		{PermInstanceBackupRestore, "instance_backup", "restore", "Restore an instance export, leaving audit history untouched", "global"},
		{PermInstanceBackupRestoreAudit, "instance_backup", "restore_audit", "Restore audit history from an instance export", "global"},

		// ─── Notifications ───
		{PermInAppNotificationsView, "in_app_notifications", "view", "View in-app notification center", "account"},

		// ─── Workflows ───
		{PermWorkflowsView, "workflows", "view", "List and inspect workflows", "global"},
		{PermWorkflowsManage, "workflows", "manage", "Create, edit workflows", "global"},
		{PermWorkflowsDelete, "workflows", "delete", "Delete workflows", "global"},
		{PermWorkflowsRun, "workflows", "run", "Trigger a workflow manually", "global"},
		{PermCustomNodesView, "custom_nodes", "view", "List custom JavaScript nodes", "global"},
		{PermCustomNodesManage, "custom_nodes", "manage", "Upload / edit custom JS nodes", "global"},
		{PermSchedulesView, "schedules", "view", "List scheduled jobs", "global"},
		{PermSchedulesManage, "schedules", "manage", "Create / edit schedules", "global"},
		{PermSchedulesDelete, "schedules", "delete", "Delete schedules", "global"},

		// ─── Webhooks / alerts / autoheal ───
		{PermWebhooksView, "webhooks", "view", "List webhook endpoints", "global"},
		{PermWebhooksManage, "webhooks", "manage", "Create / edit webhooks", "global"},
		{PermWebhooksDelete, "webhooks", "delete", "Delete webhooks", "global"},
		{PermAlertsView, "alerts", "view", "List alert rules", "global"},
		{PermAlertsManage, "alerts", "manage", "Create / edit alert rules", "global"},
		{PermAlertsDelete, "alerts", "delete", "Delete alert rules", "global"},
		{PermAutohealView, "autoheal", "view", "List autoheal configurations", "environment"},
		{PermAutohealManage, "autoheal", "manage", "Configure autoheal", "environment"},

		// ─── Backups ───
		{PermBackupsView, "backups", "view", "View backup plans, runs, and the backups page", "environment"},
		{PermBackupsManage, "backups", "manage", "Create / edit backup plans and run backups", "owner"},
		{PermBackupsDelete, "backups", "delete", "Delete backup plans and runs", "environment"},
		{PermBackupsDownload, "backups", "download", "Download a backup archive", "environment"},

		// ─── Restore ───
		{PermRestoreView, "restore", "view", "Browse the contents of a backup archive", "environment"},
		{PermRestoreRun, "restore", "run", "Restore a backup into a container", "environment"},
		{PermBackupLogView, "backup_log", "view", "View backup log entries", "environment"},

		// ─── Git / GitOps ───
		{PermGitView, "git", "view", "List git repos and credentials", "global"},
		{PermGitManage, "git", "manage", "Add / edit git repos", "global"},
		{PermGitopsView, "gitops", "view", "List GitOps pipelines and previews", "global"},
		{PermGitopsManage, "gitops", "manage", "Create / edit GitOps pipelines", "global"},
		{PermGitopsDelete, "gitops", "delete", "Delete GitOps pipelines", "global"},
		{PermGitopsApprove, "gitops", "approve", "Approve or reject promotion gates", "global"},

		// ─── Registry / plugins ───

		{PermRegistryView, "registry", "view", "List container registries", "global"},
		{PermRegistryManage, "registry", "manage", "Add / edit container registries", "global"},
		{PermPluginsView, "plugins", "view", "List installed plugins", "global"},
		{PermPluginsManage, "plugins", "manage", "Install / configure plugins", "global"},
		{PermPluginsDelete, "plugins", "delete", "Uninstall plugins", "global"},

		// ─── Storage ───
		{PermStorageLocationsView, "storage_locations", "view", "List storage locations", "global"},
		{PermStorageLocationsManage, "storage_locations", "manage", "Add / edit storage locations", "global"},
		{PermStorageLocationsDelete, "storage_locations", "delete", "Delete storage locations", "global"},

		// ─── Reconciler / updates ───
		{PermReconcilerView, "reconciler", "view", "View reconciler state", "environment"},
		{PermReconcilerManage, "reconciler", "manage", "Trigger reconciliation", "environment"},
		{PermReconcilerEnforce, "reconciler", "enforce", "Let the reconciler apply fixes without approval", "environment"},
		{PermIncidentsView, "incidents", "view", "View incidents, their evidence and narratives", "environment"},
		{PermIncidentsManage, "incidents", "manage", "Narrate incidents, set narration opt-in, resolve and dismiss", "environment"},
		{PermClusterView, "cluster", "view", "View high-availability topology, node roles and readiness", "global"},
		{PermClusterManage, "cluster", "manage", "Fail over to another node and drain or resume nodes", "global"},
		{PermComplianceView, "compliance", "view", "View the compliance posture and download compliance reports", "global"},
		{PermSupportView, "support", "view", "Run diagnostics, view the state inventory and preview support bundles", "global"},
		{PermSupportManage, "support", "manage", "Generate, download and delete redacted support bundles", "global"},
		{PermUpdatesView, "updates", "view", "View available updates", "environment"},
		{PermUpdatesManage, "updates", "manage", "Trigger an update", "environment"},

		// ─── MFA / identity ───

		{PermIdentityProvidersView, "identity_providers", "view", "List identity providers", "global"},
		{PermIdentityProvidersManage, "identity_providers", "manage", "Add / edit identity providers", "global"},

		// ─── AI assistant ───
		{PermAIAssistantView, "ai_assistant", "view", "View AI assistant", "global"},
		{PermAIAssistantUse, "ai_assistant", "use", "Send prompts to AI assistant", "global"},
		{PermAIAssistantAct, "ai_assistant", "act", "Consent to and execute assistant-proposed actions", "global"},
		{PermAIAssistantManage, "ai_assistant", "manage", "Configure AI assistant provider, models", "global"},

		// ─── Events / activity / metrics / dashboards ───
		{PermEventsView, "events", "view", "View Docker events", "environment"},
		{PermActivityView, "activity", "view", "View activity feed", "global"},
		{PermMetricsView, "metrics", "view", "View metrics and charts", "environment"},
		{PermDashboardView, "dashboard", "view", "View dashboard widgets", "global"},
		{PermWidgetsView, "widgets", "view", "List widget definitions", "global"},
		{PermWidgetsManage, "widgets", "manage", "Install / configure widgets", "global"},

		// ─── ABAC policies (v3) ───
		{PermRbacPoliciesView, "rbac", "policies.view", "List ABAC policies and their history", "global"},
		{PermRbacPoliciesManage, "rbac", "policies.manage", "Create, edit, delete, and enable ABAC policies", "global"},

		// ─── SCIM (v3) ───
		{PermScimView, "scim", "view", "List SCIM endpoints and sync history", "global"},
		{PermScimManage, "scim", "manage", "Add, edit, delete, and trigger SCIM endpoints", "global"},

		// License
		{PermLicenseView, "license", "view", "View the licence, its entitlement, and recorded usage", "global"},
		{PermLicenseManage, "license", "manage", "Request, refresh, and release the licence", "global"},

		// Approvals
		{PermApprovalsView, "approvals", "view", "View every approval request and its decisions", "global"},
		{PermApprovalsManage, "approvals", "manage", "Create, edit, and delete approval rules for destructive actions", "global"},
		{PermApprovalsDecide, "approvals", "decide", "Approve or deny another user's approval request", "global"},
	}
	// Entries an area declared beside its own constants, through
	// RegisterPermissions. Appended rather than merged so the list above stays
	// the historical record of what shipped when.
	return append(base, registeredPermissions()...)
}

// UpsertCatalog inserts (or updates) every entry from CatalogSeed
// into the permission_catalog table. Idempotent — safe to call on
// every startup.
func UpsertCatalog(db DBExecutor) error {
	// bounded by the compiled-in permission catalog
	for _, e := range CatalogSeed() {
		if _, err := db.Exec(`
			INSERT INTO permission_catalog (key, category, action, description, scope, is_system, added_at)
			VALUES (?, ?, ?, ?, ?, 1, CURRENT_TIMESTAMP)
			ON CONFLICT (key) DO UPDATE SET
				category = EXCLUDED.category,
				action = EXCLUDED.action,
				description = EXCLUDED.description,
				scope = EXCLUDED.scope
		`, string(e.Key), e.Category, e.Action, e.Description, e.Scope); err != nil {
			return fmt.Errorf("upserting permission %s: %w", e.Key, err)
		}
	}
	return nil
}

// DBExecutor is the minimal interface UpsertCatalog needs.
// *sql.DB and *sql.Tx both satisfy it.
type DBExecutor interface {
	Exec(query string, args ...any) (sql.Result, error)
}
