// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

// PermComplianceAttest records a sign-off on a compliance snapshot or an access
// review. Kept apart from compliance.manage so starting a check and accepting its
// result are two authorities, which is the separation an auditor looks for.
const PermComplianceAttest Permission = "compliance.attest"

func init() {
	RegisterPermissions(PermissionEntry{
		Key:         PermComplianceAttest,
		Category:    "compliance",
		Action:      "attest",
		Description: "Record a sign-off on a compliance-pack run or an access review. Attestations cannot be edited or deleted",
		Scope:       string(ScopeGlobal),
	})
}
