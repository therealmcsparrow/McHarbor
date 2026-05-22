// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

// Permission represents a granular access control permission.
type Permission string

// Wildcard grants access to everything.
const PermWildcard Permission = "*"

// --- Containers ---
const (
	PermContainersView   Permission = "containers.view"
	PermContainersManage Permission = "containers.manage"
	PermContainersDelete Permission = "containers.delete"
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

// --- Email Servers ---
const (
	PermEmailServersView   Permission = "email_servers.view"
	PermEmailServersManage Permission = "email_servers.manage"
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

// --- Terminal ---
const PermTerminalAccess Permission = "terminal.access"

// --- Logs ---
const PermLogsView Permission = "logs.view"

// --- API Keys ---
const PermAPIKeysManage Permission = "api_keys.manage"

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

// --- Kubernetes: Pods ---
const (
	PermPodsView   Permission = "pods.view"
	PermPodsManage Permission = "pods.manage"
	PermPodsDelete Permission = "pods.delete"
)

// --- Kubernetes: Deployments ---
const (
	PermDeploymentsView   Permission = "deployments.view"
	PermDeploymentsManage Permission = "deployments.manage"
	PermDeploymentsDelete Permission = "deployments.delete"
)

// --- Kubernetes: Services ---
const (
	PermK8sServicesView   Permission = "k8s_services.view"
	PermK8sServicesManage Permission = "k8s_services.manage"
	PermK8sServicesDelete Permission = "k8s_services.delete"
)

// --- Kubernetes: Namespaces ---
const (
	PermNamespacesView Permission = "namespaces.view"
)

// --- Scans ---
const (
	PermScansView   Permission = "scans.view"
	PermScansManage Permission = "scans.manage"
)

// AllPermissions is the complete list of permissions for UI display.
var AllPermissions = []Permission{
	PermContainersView, PermContainersManage, PermContainersDelete,
	PermImagesView, PermImagesManage, PermImagesDelete,
	PermVolumesView, PermVolumesManage, PermVolumesDelete,
	PermNetworksView, PermNetworksManage, PermNetworksDelete,
	PermStacksView, PermStacksManage, PermStacksDelete,
	PermEnvironmentsView, PermEnvironmentsManage,
	PermUsersView, PermUsersManage,
	PermSettingsView, PermSettingsManage,
	PermEmailServersView, PermEmailServersManage,
	PermCommunicationsView, PermCommunicationsManage,
	PermStoreAppsView, PermStoreAppsManage,
	PermStoreNodesView, PermStoreNodesManage,
	PermStoreWidgetsView, PermStoreWidgetsManage,
	PermTerminalAccess,
	PermLogsView,
	PermAPIKeysManage,
	PermGroupsView, PermGroupsManage,
	PermRolesView, PermRolesManage,
	PermPodsView, PermPodsManage, PermPodsDelete,
	PermDeploymentsView, PermDeploymentsManage, PermDeploymentsDelete,
	PermK8sServicesView, PermK8sServicesManage, PermK8sServicesDelete,
	PermNamespacesView,
	PermScansView, PermScansManage,
}
