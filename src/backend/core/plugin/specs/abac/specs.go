// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package abac is the optional contract through which a plugin tells the host which
// resources it owns, so attribute-based policies can target them. It is a new package
// so no already-built plugin changes its ABI; the host finds it by type assertion.
package abac

import "context"

// Attribute documents one value a policy expression can read on a resource of this
// type, for the policy editor's reference panel.
type Attribute struct {
	Path        string `json:"path"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

// Route binds a resource type to the plugin routes that act on one resource: Prefix
// is the chi pattern as registered (e.g. "/api/vms/{id}") and IDParam names the URL
// parameter holding the resource id. Routes below the prefix inherit the binding.
type Route struct {
	Prefix  string `json:"prefix"`
	IDParam string `json:"idParam"`
}

// ResourceType declares one kind of resource. Type is unique across the instance and
// lowercase_snake; Category is the permission category it pairs with (e.g. "vm").
type ResourceType struct {
	Type       string      `json:"type"`
	Label      string      `json:"label"`
	Category   string      `json:"category"`
	Routes     []Route     `json:"routes"`
	Attributes []Attribute `json:"attributes"`
}

// Resource is what a policy sees as `resource`. Attrs carries type-specific values
// (vcpus, memory, os …) and is exposed as resource.attrs.
type Resource struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Owner       string            `json:"owner"`
	State       string            `json:"state"`
	Environment string            `json:"environment"`
	Labels      map[string]string `json:"labels"`
	Attrs       map[string]any    `json:"attrs"`
}

// Provider is asserted on the plugin's exported Plugin value. ResolveABACResource
// returns (nil, nil) for an unknown id so the policy sees no resource rather than an error.
type Provider interface {
	ABACResourceTypes() []ResourceType
	ResolveABACResource(ctx context.Context, resourceType, envID, id string) (*Resource, error)
}
