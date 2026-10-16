// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package approvals

import (
	"sort"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// Risk category keys a rule can name.
const (
	CategoryContainerRemove   = "container.remove"
	CategoryContainerKill     = "container.kill"
	CategoryVolumeRemove      = "volume.remove"
	CategoryVolumePrune       = "volume.prune"
	CategoryImageRemove       = "image.remove"
	CategoryImagePrune        = "image.prune"
	CategoryStackDown         = "stack.down"
	CategoryStackRemove       = "stack.remove"
	CategoryEnvironmentDelete = "environment.delete"
	CategoryPluginUninstall   = "plugin.uninstall"
	CategoryUserDelete        = "user.delete"
	CategoryRoleDelete        = "role.delete"
)

// guardedRoute is one destructive route the gate can hold. Perms mirror the
// route's own gate so a caller who would be refused is refused, not queued.
type guardedRoute struct {
	Category string
	Method   string
	// Pattern is the path below /api with {param} segments.
	Pattern string
	Perms   []rbac.Permission
	// StackParam names the path parameter a stack-scoped gate resolves.
	StackParam string
	// ResourceParam names the path parameter shown as the affected resource.
	ResourceParam string
	// ResourceBodyField names a JSON body field shown when the path has none.
	ResourceBodyField string
	// SkipWhenDryRun lets a {"dryRun": true} preview through unheld.
	SkipWhenDryRun bool
}

// guardedRoutes is the catalogue the gate matches. It lists routes rather than
// wrapping each module's handler so that no module has to know approvals exist.
var guardedRoutes = []guardedRoute{
	{Category: CategoryContainerRemove, Method: "DELETE", Pattern: "/containers/{id}", Perms: []rbac.Permission{rbac.PermContainersDelete}, ResourceParam: "id"},
	{Category: CategoryContainerRemove, Method: "POST", Pattern: "/containers/{id}/remove", Perms: []rbac.Permission{rbac.PermContainersDelete}, ResourceParam: "id"},
	{Category: CategoryContainerKill, Method: "POST", Pattern: "/containers/{id}/kill", Perms: []rbac.Permission{rbac.PermContainersManage}, ResourceParam: "id"},
	{Category: CategoryVolumeRemove, Method: "DELETE", Pattern: "/volumes/{name}", Perms: []rbac.Permission{rbac.PermVolumesDelete}, ResourceParam: "name"},
	{Category: CategoryVolumePrune, Method: "POST", Pattern: "/volumes/prune", Perms: []rbac.Permission{rbac.PermVolumesManage}},
	{Category: CategoryImageRemove, Method: "DELETE", Pattern: "/images/{id}", Perms: []rbac.Permission{rbac.PermImagesDelete}, ResourceParam: "id"},
	{Category: CategoryImagePrune, Method: "POST", Pattern: "/images/prune", Perms: []rbac.Permission{rbac.PermImagesManage}},
	{Category: CategoryStackDown, Method: "POST", Pattern: "/stacks/{name}/down", Perms: []rbac.Permission{rbac.PermStacksManage}, StackParam: "name", ResourceParam: "name"},
	{Category: CategoryStackRemove, Method: "DELETE", Pattern: "/stacks/{name}", Perms: []rbac.Permission{rbac.PermStacksDelete}, StackParam: "name", ResourceParam: "name"},
	{Category: CategoryEnvironmentDelete, Method: "DELETE", Pattern: "/environments/{id}", Perms: []rbac.Permission{rbac.PermEnvironmentsManage}, ResourceParam: "id"},
	{Category: CategoryPluginUninstall, Method: "POST", Pattern: "/appstore/plugins/uninstall", Perms: []rbac.Permission{rbac.PermPluginsManage, rbac.PermPluginsDelete}, ResourceBodyField: "slug"},
	{Category: CategoryUserDelete, Method: "DELETE", Pattern: "/users/{id}", Perms: []rbac.Permission{rbac.PermUsersManage}, ResourceParam: "id"},
	{Category: CategoryUserDelete, Method: "POST", Pattern: "/users/bulk/delete", Perms: []rbac.Permission{rbac.PermUsersManage}, SkipWhenDryRun: true},
	{Category: CategoryRoleDelete, Method: "DELETE", Pattern: "/roles/{id}", Perms: []rbac.Permission{rbac.PermRolesManage}, ResourceParam: "id"},
}

// matchRoute finds the guarded route for a method and a path below /api and
// returns the path parameters it bound. Trailing slashes are ignored because chi
// serves "/x/" and "/x" from the same handler here.
func matchRoute(method, path string) (*guardedRoute, map[string]string) {
	segs := splitPath(path)
	for i := range guardedRoutes {
		g := &guardedRoutes[i]
		if g.Method != method {
			continue
		}
		if params, ok := matchSegments(splitPath(g.Pattern), segs); ok {
			return g, params
		}
	}
	return nil, nil
}

func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

func matchSegments(pattern, segs []string) (map[string]string, bool) {
	if len(pattern) != len(segs) {
		return nil, false
	}
	params := map[string]string{}
	for i, p := range pattern {
		if strings.HasPrefix(p, "{") && strings.HasSuffix(p, "}") {
			if segs[i] == "" {
				return nil, false
			}
			params[strings.Trim(p, "{}")] = segs[i]
			continue
		}
		if p != segs[i] {
			return nil, false
		}
	}
	return params, true
}

// KnownCategory reports whether key names a category the gate can enforce.
func KnownCategory(key string) bool {
	for _, g := range guardedRoutes {
		if g.Category == key {
			return true
		}
	}
	return false
}

// Categories lists every risk category with the routes it holds, for the builder.
func Categories() []CategoryInfo {
	byKey := map[string]*CategoryInfo{}
	var order []string
	for _, g := range guardedRoutes {
		info, ok := byKey[g.Category]
		if !ok {
			info = &CategoryInfo{Key: g.Category}
			byKey[g.Category] = info
			order = append(order, g.Category)
		}
		info.Routes = append(info.Routes, RouteInfo{Method: g.Method, Pattern: "/api" + g.Pattern})
		for _, p := range g.Perms {
			info.Permissions = appendUnique(info.Permissions, string(p))
		}
	}
	sort.Strings(order)
	out := make([]CategoryInfo, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	return out
}

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}
