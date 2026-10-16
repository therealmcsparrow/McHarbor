// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

// PermComplianceManage starts a compliance-pack check run and configures a pack.
// Separate from compliance.view because a run reaches out to a live Docker daemon,
// which reading a computed posture never does.
const PermComplianceManage Permission = "compliance.manage"

// Registered from this file rather than appended to CatalogSeed and
// AllPermissions, so the const, the catalog entry and the aggregate list cannot
// drift apart. Scope is global to match compliance.view: a pack is an
// instance-wide posture even when one of its checks runs per environment.
func init() {
	RegisterPermissions(PermissionEntry{
		Key:         PermComplianceManage,
		Category:    "compliance",
		Action:      "manage",
		Description: "Start compliance-pack check runs and configure packs",
		Scope:       "global",
	})
}
