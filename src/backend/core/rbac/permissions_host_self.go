// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

// The keys for McHarbor's own host. They are separate from host_services.* and
// host_updates.* so that an "all environments" grant of those per-environment keys
// no longer reaches the machine McHarbor itself runs on.

// PermHostSelfView reads McHarbor's own host: its services, OS updates and reboot state.
const PermHostSelfView Permission = "host_self.view"

// PermHostSelfManage changes McHarbor's own host: service actions, OS updates, and exec,
// lifecycle or config changes on McHarbor's own containers (core/selfguard).
const PermHostSelfManage Permission = "host_self.manage"

// PermHostSelfReboot schedules or cancels a reboot of McHarbor's own host.
const PermHostSelfReboot Permission = "host_self.reboot"

func init() {
	RegisterPermissions(
		PermissionEntry{
			Key: PermHostSelfView, Category: "host_self", Action: "view",
			Description: "View the services, OS updates and reboot state of the host McHarbor runs on",
			Scope:       string(ScopeGlobal),
		},
		PermissionEntry{
			Key: PermHostSelfManage, Category: "host_self", Action: "manage",
			Description: "Run service actions and install OS updates on the host McHarbor runs on, and act on McHarbor's own containers",
			Scope:       string(ScopeGlobal),
		},
		PermissionEntry{
			Key: PermHostSelfReboot, Category: "host_self", Action: "reboot",
			Description: "Schedule or cancel a reboot of the host McHarbor runs on",
			Scope:       string(ScopeGlobal),
		},
	)
}
