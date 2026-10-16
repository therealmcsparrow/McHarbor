// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

// PermDashboardWidgetsInstall installs and removes downloaded widget bundles. It
// is global and separate from widgets.manage because it lets UI code from a
// catalog run in every operator's browser, which is a trust decision, not a layout one.
const PermDashboardWidgetsInstall Permission = "dashboard_widgets.install"

func init() {
	RegisterPermissions(
		PermissionEntry{
			Key: PermDashboardWidgetsInstall, Category: "dashboard_widgets", Action: "install",
			Description: "Install and remove signed widget bundles from the widget catalog. " +
				"An installed bundle runs in every dashboard viewer's browser",
			Scope: string(ScopeGlobal),
		},
	)
}
