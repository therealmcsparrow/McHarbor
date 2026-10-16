// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package contract declares the plugin lifecycle, manifest and registry shapes shared by the host and every plugin.
package contract

import "github.com/therealmcsparrow/mcharbor/core/plugin/types"

// AccessRegistry is the §7.2 surface — plugins add/remove their own
// permissions and ABAC rules through it. Direct edits to rbac_*
// tables are forbidden and caught at install time.
type AccessRegistry interface {
	AddPermission(p types.PermissionDecl) error
	RemovePermission(key string) error
	InstallABACRule(rule types.ABACRuleDecl) error
	RemoveABACRule(id string) error
	// SCIM schema registration (§7.5).
	RegisterSCIMSchema(schema types.SCIMSchema) error
	UnregisterSCIMSchema(uri string) error
}

// SCIMHandler is the optional callback a plugin may return from
// Init() to handle SCIM operations on its own resource types.
// Dispatched by modules/scim/dispatcher.go.
type SCIMHandler interface {
	Get(resourceType, id string) (map[string]any, error)
	List(resourceType string, filter string, startIndex, count int) ([]map[string]any, int, error)
	Create(resourceType string, attrs map[string]any) (map[string]any, error)
	Patch(resourceType, id string, patch map[string]any) (map[string]any, error)
	Delete(resourceType, id string) error
}
