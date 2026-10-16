// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

// The runbook keys, declared through the append seam in catalog_extra.go so the
// const, the catalog entry and the AllPermissions membership cannot disagree.

// PermRunbooksView reads a runbook and its run history.
const PermRunbooksView Permission = "runbooks.view"

// PermRunbooksManage authors and edits runbooks, including which action a step
// may take, which is why it is global rather than per-environment.
const PermRunbooksManage Permission = "runbooks.manage"

// PermRunbooksRun starts a run and records steps. It is permission to follow a
// runbook, never permission to do what a step says — see the catalog entry.
const PermRunbooksRun Permission = "runbooks.run"

// PermRunbooksAutomate assigns a runbook to a service principal for unattended
// runs. It is global because it decides which principal acts, not what it may do.
const PermRunbooksAutomate Permission = "runbooks.automate"

func init() {
	RegisterPermissions(
		PermissionEntry{
			Key: PermRunbooksView, Category: "runbooks", Action: "view",
			Description: "View runbooks and the history of runs against them",
			Scope:       string(ScopeEnvironment),
		},
		PermissionEntry{
			Key: PermRunbooksManage, Category: "runbooks", Action: "manage",
			Description: "Author and edit runbooks, including which action each step may take",
			Scope:       string(ScopeGlobal),
		},
		PermissionEntry{
			Key: PermRunbooksRun, Category: "runbooks", Action: "run",
			Description: "Follow a runbook and record its steps. This does NOT grant what a step " +
				"does: an action step is re-checked against the operator's own permission for that " +
				"action every time it is pressed, so restarting a container from a runbook still " +
				"requires containers.manage",
			Scope: string(ScopeEnvironment),
		},
		PermissionEntry{
			Key: PermRunbooksAutomate, Category: "runbooks", Action: "automate",
			Description: "Configure a runbook to run unattended, on its alert or a schedule, as a " +
				"service principal. Every step is still checked against that principal's own roles; " +
				"this key decides which principal acts, not what it may do",
			Scope: string(ScopeGlobal),
		},
	)
}
