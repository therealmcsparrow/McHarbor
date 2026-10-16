// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
)

// ResourceAttribute documents one value a policy expression can read.
type ResourceAttribute struct {
	Path        string `json:"path"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

// ResourceRoute binds a resource type to the routes that act on one resource of it.
type ResourceRoute struct {
	Prefix  string `json:"prefix"`
	IDParam string `json:"idParam"`
}

// ResourceTypeInfo is one entry of the policy editor's resource-type list. Source is
// "host" or the slug of the plugin that declared it.
type ResourceTypeInfo struct {
	Type       string              `json:"type"`
	Label      string              `json:"label"`
	Category   string              `json:"category"`
	Source     string              `json:"source"`
	Routes     []ResourceRoute     `json:"routes"`
	Attributes []ResourceAttribute `json:"attributes"`
	Resolvable bool                `json:"resolvable"`
}

// ResourceResolver loads the attributes of one resource for policy evaluation.
type ResourceResolver func(ctx context.Context, envID, id string) (*ResourceAttributes, error)

type registeredType struct {
	info    ResourceTypeInfo
	resolve ResourceResolver
}

var (
	resourceTypesMu sync.RWMutex
	resourceTypes   = map[string]registeredType{}
)

// commonAttributes are available on every resource type, resolved or not.
var commonAttributes = []ResourceAttribute{
	{Path: "resource.type", Type: "string", Description: "The resource type"},
	{Path: "resource.id", Type: "string", Description: "The resource id from the route"},
}

// resolvedAttributes are available once a resolver loaded the resource.
var resolvedAttributes = []ResourceAttribute{
	{Path: "resource.name", Type: "string", Description: "Display name"},
	{Path: "resource.owner", Type: "string", Description: "Owning user id, when the resource records one"},
	{Path: "resource.state", Type: "string", Description: "Current state, when the resource has one"},
	{Path: "resource.environment", Type: "string", Description: "Environment id the resource lives in"},
	{Path: "resource.labels", Type: "map", Description: "Labels; read with hasLabel(resource.labels, key)"},
}

// RegisterResourceType adds or replaces a resource type. A nil resolver still makes the
// type selectable and route-matched; only resource.type and resource.id are then known.
func RegisterResourceType(info ResourceTypeInfo, resolve ResourceResolver) {
	if info.Type == "" {
		return
	}
	if info.Source == "" {
		info.Source = "host"
	}
	info.Resolvable = resolve != nil
	attrs := append([]ResourceAttribute{}, commonAttributes...)
	if resolve != nil {
		attrs = append(attrs, resolvedAttributes...)
	}
	info.Attributes = append(attrs, info.Attributes...)
	resourceTypesMu.Lock()
	defer resourceTypesMu.Unlock()
	resourceTypes[info.Type] = registeredType{info: info, resolve: resolve}
}

// UnregisterResourceTypes drops every type a plugin declared, when it is unloaded.
func UnregisterResourceTypes(source string) {
	resourceTypesMu.Lock()
	defer resourceTypesMu.Unlock()
	for k, v := range resourceTypes {
		if v.info.Source == source {
			delete(resourceTypes, k)
		}
	}
}

// ResourceTypes lists every registered type, host types first, then by label.
func ResourceTypes() []ResourceTypeInfo {
	resourceTypesMu.RLock()
	defer resourceTypesMu.RUnlock()
	out := make([]ResourceTypeInfo, 0, len(resourceTypes))
	for _, v := range resourceTypes {
		out = append(out, v.info)
	}
	sort.Slice(out, func(i, j int) bool {
		hi, hj := out[i].Source == "host", out[j].Source == "host"
		if hi != hj {
			return hi
		}
		return out[i].Label < out[j].Label
	})
	return out
}

func resolverFor(resourceType string) ResourceResolver {
	resourceTypesMu.RLock()
	defer resourceTypesMu.RUnlock()
	return resourceTypes[resourceType].resolve
}

// ResourceFromRoute names the resource a request acts on from the matched route
// pattern, so a policy bound to a type applies at the route gate and not only where
// service code happens to pass the type along.
func ResourceFromRoute(r *http.Request) (string, string) {
	rctx := chi.RouteContext(r.Context())
	if rctx == nil {
		return "", ""
	}
	pattern := rctx.RoutePattern()
	if pattern == "" {
		return "", ""
	}
	resourceTypesMu.RLock()
	defer resourceTypesMu.RUnlock()
	best, bestLen, bestParam := "", 0, ""
	for typ, v := range resourceTypes {
		for _, rt := range v.info.Routes {
			if len(rt.Prefix) > bestLen && (pattern == rt.Prefix || strings.HasPrefix(pattern, rt.Prefix+"/")) {
				best, bestLen, bestParam = typ, len(rt.Prefix), rt.IDParam
			}
		}
	}
	if best == "" {
		return "", ""
	}
	return best, chi.URLParam(r, bestParam)
}

// registryProvider adapts the registry to the AttributeBundle's provider lookup, so
// every registered type resolves through the same path as the original container one.
func (b *AttributeBundle) resolveRegistered(ctx context.Context, req AuthzRequest) *ResourceAttributes {
	resolve := resolverFor(req.ResourceType)
	if resolve == nil {
		return nil
	}
	attrs, err := resolve(ctx, req.EnvironmentID, req.ResourceID)
	if err != nil {
		if b.logger != nil {
			b.logger.Warn("abac: resource resolver failed", "resource_type", req.ResourceType, "resource_id", req.ResourceID, "error", err)
		}
		return nil
	}
	return attrs
}

// ResourceFromPattern names the resource for a route known by its declared pattern
// (plugin routes are dispatched from their own table, not matched by chi), reading
// the id from the request path segment that sits where the pattern has the parameter.
func ResourceFromPattern(pattern, path string) (string, string) {
	resourceTypesMu.RLock()
	best, bestLen, bestParam := "", 0, ""
	for typ, v := range resourceTypes {
		for _, rt := range v.info.Routes {
			if len(rt.Prefix) > bestLen && (pattern == rt.Prefix || strings.HasPrefix(pattern, rt.Prefix+"/")) {
				best, bestLen, bestParam = typ, len(rt.Prefix), rt.IDParam
			}
		}
	}
	resourceTypesMu.RUnlock()
	if best == "" {
		return "", ""
	}
	pat := strings.Split(strings.Trim(pattern, "/"), "/")
	got := strings.Split(strings.Trim(path, "/"), "/")
	for i, seg := range pat {
		if seg == "{"+bestParam+"}" && i < len(got) {
			return best, got[i]
		}
	}
	return best, ""
}
